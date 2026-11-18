package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	blobCacheDirName      = ".blob-cache"
	blobCacheStorageFile  = "storage.json"
	blobExtractedOkMarker = ".gp-extracted-ok"
	maxArchiveFileBytes   = 32 << 30 // 32 GiB safety cap per archive entry
)

// CachedArtifactMeta records verified metadata for an Artifact materialized in <workDir>/.blob-cache.
type CachedArtifactMeta = driver.CachedArtifactMeta

// BlobCacheStorageInfo records the host filesystem device and CoW reflink capability of <workDir>/.blob-cache.
type BlobCacheStorageInfo struct {
	DeviceID         uint64    `json:"deviceId"`
	ReflinkSupported bool      `json:"reflinkSupported"`
	ProbedAt         time.Time `json:"probedAt"`
}

var (
	blobLocks        sync.Map // map[string]*sync.Mutex
	blobStorageCache sync.Map // map[string]BlobCacheStorageInfo
)

func lockKey(key string) *sync.Mutex {
	mu, _ := blobLocks.LoadOrStore(key, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func blobFilePath(workDir, hexDigest string) string {
	return filepath.Join(workDir, blobCacheDirName, "sha256", hexDigest)
}

func blobExtractedDir(workDir, hexDigest string) string {
	return filepath.Join(workDir, blobCacheDirName, "extracted", hexDigest)
}

func artifactMetaPath(workDir, coord string) string {
	return filepath.Join(workDir, blobCacheDirName, "artifacts", coord+".json")
}

func parseSHA256Hex(digest string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(digest))
	hexPart, ok := strings.CutPrefix(trimmed, "sha256:")
	if !ok || len(hexPart) != 64 {
		return "", fmt.Errorf("invalid sha256 digest %q", digest)
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", fmt.Errorf("invalid sha256 hex %q: %w", digest, err)
	}
	return hexPart, nil
}

// EnsureArtifactCached verifies that coord is Ready in the Artifact Registry, downloads & verifies its
// content-addressed blob into <workDir>/.blob-cache/sha256/<hex>, and extracts archives once.
func EnsureArtifactCached(ctx context.Context, workDir string, cli client.Interface, coord string) (string, *CachedArtifactMeta, error) {
	if strings.TrimSpace(workDir) == "" {
		return "", nil, errors.New("agent workDir is required for blob cache")
	}
	if cli == nil {
		return "", nil, errors.New("registry API client is unavailable")
	}
	art, err := cli.RegistryV1().Artifacts().Get(ctx, coord)
	if err != nil {
		return "", nil, fmt.Errorf("artifact %s unavailable: %w", coord, err)
	}
	switch art.Status.Phase {
	case registryv1.ArtifactPhasePending, registryv1.ArtifactPhaseSyncing:
		return fmt.Sprintf("正在从镜像站拉取 %s", coord), nil, nil
	case registryv1.ArtifactPhaseReady:
	default:
		msg := strings.TrimSpace(art.Status.Message)
		if msg == "" {
			msg = string(art.Status.Phase)
		}
		return "", nil, fmt.Errorf("artifact %s failed in registry: %s", coord, msg)
	}
	meta, err := buildCachedArtifactMeta(coord, art)
	if err != nil {
		return "", nil, err
	}
	if err := ensureBlobDownloadedAndExtracted(ctx, workDir, cli, meta); err != nil {
		return "", nil, err
	}
	if err := writeCachedArtifactMeta(workDir, meta); err != nil {
		return "", nil, err
	}
	return "", meta, nil
}

func buildCachedArtifactMeta(coord string, art *registryv1.Artifact) (*CachedArtifactMeta, error) {
	desc := art.Status.Descriptor
	if desc == nil {
		desc = art.Spec.Descriptor
	}
	if desc == nil || strings.TrimSpace(desc.Digest) == "" {
		return nil, fmt.Errorf("artifact %s is Ready without a blob descriptor", coord)
	}
	hexDigest, err := parseSHA256Hex(desc.Digest)
	if err != nil {
		return nil, fmt.Errorf("artifact %s: %w", coord, err)
	}
	archFmt := firstNonEmpty(desc.ArchiveFormat, art.Spec.ArchiveFormat, inferBlobArchiveFormat(desc.FileName))
	return &CachedArtifactMeta{
		Coordinate:    coord,
		Digest:        "sha256:" + hexDigest,
		HexDigest:     hexDigest,
		SizeBytes:     desc.SizeBytes,
		ArchiveFormat: archFmt,
		FileName:      filepath.Base(strings.TrimSpace(desc.FileName)),
		TargetPath:    strings.TrimSpace(art.Spec.TargetPath),
		ModIDs:        append([]string(nil), art.Status.ModIDs...),
		MapFolders:    append([]string(nil), art.Status.MapFolders...),
		ConfigSchema:  append([]registryv1.AddonConfigField(nil), art.Status.ConfigSchema...),
	}, nil
}

func inferBlobArchiveFormat(fileName string) string {
	low := strings.ToLower(strings.TrimSpace(fileName))
	switch {
	case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
		return registryv1.ArchiveFormatTarGz
	case strings.HasSuffix(low, ".zip"):
		return registryv1.ArchiveFormatZip
	case strings.HasSuffix(low, ".jar"):
		return registryv1.ArchiveFormatJar
	case strings.HasSuffix(low, ".tmod"):
		return registryv1.ArchiveFormatTMod
	default:
		return registryv1.ArchiveFormatRaw
	}
}

func isArchiveFormat(format string) bool {
	return format == registryv1.ArchiveFormatTarGz || format == registryv1.ArchiveFormatZip
}

func isArchiveAlreadyExtracted(extractedDir string) bool {
	marker := filepath.Join(extractedDir, blobExtractedOkMarker)
	info, err := os.Lstat(marker)
	return err == nil && volume.IsImmutableRegularCacheFile(info)
}

func ensureBlobDownloadedAndExtracted(ctx context.Context, workDir string, cli client.Interface, meta *CachedArtifactMeta) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mu := lockKey("blob:" + meta.HexDigest)
	mu.Lock()
	defer mu.Unlock()

	dstBlob := blobFilePath(workDir, meta.HexDigest)
	extractedDir := blobExtractedDir(workDir, meta.HexDigest)
	if isArchiveFormat(meta.ArchiveFormat) && isValidLocalBlob(dstBlob, meta.SizeBytes) && isArchiveAlreadyExtracted(extractedDir) {
		return nil
	}
	if !isVerifiedLocalBlob(dstBlob, meta.HexDigest, meta.SizeBytes) {
		_ = os.RemoveAll(extractedDir)
		_ = os.Remove(dstBlob)
		if err := downloadAndVerifyBlob(ctx, cli, meta.Digest, meta.HexDigest, meta.SizeBytes, dstBlob); err != nil {
			return err
		}
	}
	if isArchiveFormat(meta.ArchiveFormat) {
		return ensureArchiveExtractedContext(ctx, dstBlob, extractedDir, meta.ArchiveFormat)
	}
	return nil
}

func isVerifiedLocalBlob(path, expectedHex string, expectedSize int64) bool {
	if !isValidLocalBlob(path, expectedSize) {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(expectedHex))
	if want == "" {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == want
}

func isValidLocalBlob(path string, expectedSize int64) bool {
	info, err := os.Lstat(path)
	if err != nil || !volume.IsImmutableRegularCacheFile(info) {
		return false
	}
	return expectedSize <= 0 || info.Size() == expectedSize
}

func readonlyCachePerm(mode os.FileMode) os.FileMode {
	if mode&0o111 != 0 {
		return 0o555
	}
	return 0o444
}

func downloadAndVerifyBlob(ctx context.Context, cli client.Interface, digest, expectedHex string, expectedSize int64, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	rc, err := cli.RegistryV1().Blobs().Download(ctx, digest)
	if err != nil {
		return fmt.Errorf("download blob %s: %w", digest, err)
	}
	defer func() { _ = rc.Close() }()

	tmpFile, err := os.CreateTemp(filepath.Dir(dstPath), ".blob-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmpFile, hasher), rc)
	closeErr := tmpFile.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("stream blob %s: %w", digest, errors.Join(copyErr, closeErr))
	}
	actualHex := hex.EncodeToString(hasher.Sum(nil))
	if actualHex != expectedHex || (expectedSize > 0 && written != expectedSize) {
		_ = os.Remove(tmpName)
		return fmt.Errorf("blob %s integrity check failed (got sha256:%s size=%d, expected size=%d)", digest, actualHex, written, expectedSize)
	}
	if err := os.Chmod(tmpName, 0o444); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dstPath); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func ensureArchiveExtracted(archivePath, dstDir, format string) error {
	return ensureArchiveExtractedContext(context.Background(), archivePath, dstDir, format)
}

func ensureArchiveExtractedContext(ctx context.Context, archivePath, dstDir, format string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if isArchiveAlreadyExtracted(dstDir) {
		return nil
	}
	parent := filepath.Dir(dstDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".extract-*")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	var extractErr error
	switch format {
	case registryv1.ArchiveFormatTarGz:
		extractErr = extractTarGzArchiveContext(ctx, archivePath, staging)
	case registryv1.ArchiveFormatZip:
		extractErr = extractZipArchiveContext(ctx, archivePath, staging)
	default:
		cleanup()
		return nil
	}
	if extractErr != nil {
		cleanup()
		return fmt.Errorf("extract %s archive: %w", format, extractErr)
	}
	if err := os.WriteFile(filepath.Join(staging, blobExtractedOkMarker), []byte("ok\n"), 0o444); err != nil {
		cleanup()
		return err
	}
	_ = os.RemoveAll(dstDir)
	if err := os.Rename(staging, dstDir); err != nil {
		cleanup()
		return err
	}
	return nil
}

func safeExtractTarget(rootDir, entryName string) (string, bool) {
	relPath, err := metav1.NormalizeArchiveEntryPath(entryName, false)
	if err != nil {
		return "", false
	}
	target := filepath.Join(rootDir, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(rootDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return target, true
}

func extractTarGzArchiveContext(ctx context.Context, archivePath, dstDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := extractSingleTarEntry(tr, hdr, dstDir); err != nil {
			return err
		}
	}
}

func extractSingleTarEntry(tr *tar.Reader, hdr *tar.Header, dstDir string) error {
	target, ok := safeExtractTarget(dstDir, hdr.Name)
	if !ok {
		return nil
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		_ = os.Remove(target)
		perm := readonlyCachePerm(os.FileMode(hdr.Mode) & 0o777)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, maxArchiveFileBytes))
		chmodErr := out.Chmod(perm)
		closeErr := out.Close()
		return errors.Join(copyErr, chmodErr, closeErr)
	default:
		return nil
	}
}

func extractZipArchiveContext(ctx context.Context, archivePath, dstDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := extractSingleZipEntry(zf, dstDir); err != nil {
			return err
		}
	}
	return nil
}

func extractSingleZipEntry(zf *zip.File, dstDir string) error {
	target, ok := safeExtractTarget(dstDir, zf.Name)
	if !ok {
		return nil
	}
	if zf.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if zf.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_ = os.Remove(target)
	perm := readonlyCachePerm(zf.Mode().Perm())
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(rc, maxArchiveFileBytes))
	chmodErr := out.Chmod(perm)
	closeErr := out.Close()
	return errors.Join(copyErr, chmodErr, closeErr)
}

func writeCachedArtifactMeta(workDir string, meta *CachedArtifactMeta) error {
	return writeCacheJSONAtomic(artifactMetaPath(workDir, meta.Coordinate), meta)
}

func blobStorageInfoPath(workDir string) string {
	return filepath.Join(workDir, blobCacheDirName, blobCacheStorageFile)
}

// EnsureBlobCacheStorageInfo probes and persists the host filesystem device and CoW reflink capability of <workDir>/.blob-cache.
func EnsureBlobCacheStorageInfo(workDir string) BlobCacheStorageInfo {
	cleanWork := filepath.Clean(strings.TrimSpace(workDir))
	if cleanWork == "" || cleanWork == "." {
		return BlobCacheStorageInfo{}
	}
	mu := lockKey("storage:" + cleanWork)
	mu.Lock()
	defer mu.Unlock()

	cacheDir := filepath.Join(cleanWork, blobCacheDirName)
	curDev, hasCurDev := volume.DirDeviceID(cacheDir)
	if v, ok := blobStorageCache.Load(cleanWork); ok {
		info := v.(BlobCacheStorageInfo)
		if !info.ProbedAt.IsZero() && (!hasCurDev || curDev == info.DeviceID) {
			applyBlobStorageUnsupportedMark(info)
			return info
		}
	}
	if diskInfo, ok := readBlobStorageInfo(blobStorageInfoPath(cleanWork)); ok && (!hasCurDev || curDev == diskInfo.DeviceID) {
		applyBlobStorageUnsupportedMark(diskInfo)
		blobStorageCache.Store(cleanWork, diskInfo)
		return diskInfo
	}
	devID, supported, err := volume.ProbeDirReflink(cacheDir)
	if err != nil {
		return BlobCacheStorageInfo{DeviceID: devID}
	}
	info := BlobCacheStorageInfo{
		DeviceID:         devID,
		ReflinkSupported: supported,
		ProbedAt:         time.Now().UTC(),
	}
	applyBlobStorageUnsupportedMark(info)
	if writeErr := writeCacheJSONAtomic(blobStorageInfoPath(cleanWork), info); writeErr == nil {
		blobStorageCache.Store(cleanWork, info)
	}
	return info
}

func applyBlobStorageUnsupportedMark(info BlobCacheStorageInfo) {
	if !info.ProbedAt.IsZero() && !info.ReflinkSupported {
		volume.MarkDeviceReflinkUnsupported(info.DeviceID)
	}
}

func readBlobStorageInfo(path string) (BlobCacheStorageInfo, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BlobCacheStorageInfo{}, false
	}
	var info BlobCacheStorageInfo
	if json.Unmarshal(raw, &info) != nil || info.ProbedAt.IsZero() {
		return BlobCacheStorageInfo{}, false
	}
	return info, true
}

func writeCacheJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".meta-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	_, writeErr := tmpFile.Write(raw)
	chmodErr := tmpFile.Chmod(0o644)
	closeErr := tmpFile.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func readCachedArtifactMeta(workDir, coord string) (*CachedArtifactMeta, error) {
	raw, err := os.ReadFile(artifactMetaPath(workDir, coord))
	if err != nil {
		return nil, fmt.Errorf("cached artifact metadata %s missing: %w", coord, err)
	}
	var meta CachedArtifactMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	if !isValidLocalBlob(blobFilePath(workDir, meta.HexDigest), meta.SizeBytes) {
		return nil, fmt.Errorf("cached blob %s missing for %s", meta.Digest, coord)
	}
	return &meta, nil
}

// CollectNodeBlobCacheInventory lists verified cached artifact coordinates and blob digests on disk.
func CollectNodeBlobCacheInventory(workDir string) ([]string, []string) {
	if strings.TrimSpace(workDir) == "" {
		return nil, nil
	}
	blobSet := make(map[string]bool)
	var cachedBlobs []string
	if entries, err := os.ReadDir(filepath.Join(workDir, blobCacheDirName, "sha256")); err == nil {
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			blobPath := filepath.Join(workDir, blobCacheDirName, "sha256", e.Name())
			if !e.IsDir() && len(name) == 64 && isValidLocalBlob(blobPath, -1) {
				d := "sha256:" + name
				blobSet[d] = true
				cachedBlobs = append(cachedBlobs, d)
			}
		}
	}
	sort.Strings(cachedBlobs)

	var cachedArtifacts []string
	if entries, err := os.ReadDir(filepath.Join(workDir, blobCacheDirName, "artifacts")); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			coord := strings.TrimSuffix(e.Name(), ".json")
			if meta, err := readCachedArtifactMeta(workDir, coord); err == nil && blobSet[meta.Digest] {
				cachedArtifacts = append(cachedArtifacts, meta.Coordinate)
			}
		}
	}
	sort.Strings(cachedArtifacts)
	return cachedArtifacts, cachedBlobs
}

// PruneDisallowedNodeBlobCache removes cached artifact metadata, blobs, and extracted directories whose
// repository is outside allowedRepos and not referenced by activeCoords, returning true if anything was removed.
func PruneDisallowedNodeBlobCache(workDir string, allowedRepos, activeCoords map[string]struct{}) bool {
	if strings.TrimSpace(workDir) == "" || len(allowedRepos) == 0 {
		return false
	}
	cacheRoot := filepath.Join(workDir, blobCacheDirName)
	referencedHex, pruned := collectKeptBlobHexes(filepath.Join(cacheRoot, "artifacts"), allowedRepos, activeCoords)
	if pruneUnreferencedDigestFiles(filepath.Join(cacheRoot, "sha256"), referencedHex) {
		pruned = true
	}
	if pruneUnreferencedExtractedDirs(filepath.Join(cacheRoot, "extracted"), referencedHex) {
		pruned = true
	}
	return pruned
}

func collectKeptBlobHexes(artifactsDir string, allowedRepos, activeCoords map[string]struct{}) (map[string]struct{}, bool) {
	keptHex := make(map[string]struct{})
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		return keptHex, false
	}
	pruned := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		coord := strings.TrimSuffix(e.Name(), ".json")
		repo, _, _ := strings.Cut(coord, ".")
		_, allowed := allowedRepos[repo]
		_, active := activeCoords[coord]
		metaPath := filepath.Join(artifactsDir, e.Name())
		if !allowed && !active {
			if os.Remove(metaPath) == nil {
				pruned = true
			}
			continue
		}
		if raw, rErr := os.ReadFile(metaPath); rErr == nil {
			var meta CachedArtifactMeta
			if json.Unmarshal(raw, &meta) == nil && meta.HexDigest != "" {
				keptHex[strings.ToLower(meta.HexDigest)] = struct{}{}
			}
		}
	}
	return keptHex, pruned
}

func pruneUnreferencedDigestFiles(shaDir string, keptHex map[string]struct{}) bool {
	entries, err := os.ReadDir(shaDir)
	if err != nil {
		return false
	}
	pruned := false
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := keptHex[strings.ToLower(name)]; ok {
			continue
		}
		p := filepath.Join(shaDir, name)
		_ = os.Chmod(p, 0o644)
		if os.Remove(p) == nil {
			pruned = true
		}
	}
	return pruned
}

func pruneUnreferencedExtractedDirs(extractedDir string, keptHex map[string]struct{}) bool {
	entries, err := os.ReadDir(extractedDir)
	if err != nil {
		return false
	}
	pruned := false
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := keptHex[strings.ToLower(name)]; ok {
			continue
		}
		p := filepath.Join(extractedDir, name)
		if removeCacheTree(p) == nil {
			pruned = true
		}
	}
	return pruned
}

func removeCacheTree(root string) error {
	if err := os.RemoveAll(root); err == nil {
		return nil
	}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o755)
		} else {
			_ = os.Chmod(p, 0o644)
		}
		return nil
	})
	return os.RemoveAll(root)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
