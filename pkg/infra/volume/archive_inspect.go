package volume

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

const (
	maxArchiveEntries             = 100000
	maxArchiveDirectories         = 200
	maxArchiveDirectoryDepth      = 3
	maxArchiveTextFileBytes       = 64 * 1024
	maxArchiveTotalTextBytes      = 512 * 1024
	zipInspectMemoryFastPathBytes = 8 << 20
	zipInspectSpoolMaxBytes       = 512 << 20
)

var zipSpoolSem = make(chan struct{}, 2)

// ArchiveInspectOptions parameterizes archive directory and anchor discovery without app hardcoding.
type ArchiveInspectOptions struct {
	DefaultTarget string
	AnchorFiles   []string
}

// ArchiveInspection describes detected format, candidate root directories, and directory choices.
type ArchiveInspection struct {
	Format            string
	RecommendedSource string
	CandidateSources  []string
	Directories       []string
	Files             []string
	TextFiles         map[string]string
	Entries           int
	FileCount         int
	UncompressedBytes int64
}

type archiveScanResult struct {
	files             []string
	dirs              []string
	textFiles         map[string]string
	entries           int
	uncompressedBytes int64
}

// DetectArchiveFormat identifies a backup stream format from magic bytes rather than filename.
func DetectArchiveFormat(header []byte) (string, error) {
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return corev1.ArchiveFormatTarGz, nil
	}
	if len(header) >= 4 && header[0] == 'P' && header[1] == 'K' &&
		((header[2] == 3 && header[3] == 4) || (header[2] == 5 && header[3] == 6) || (header[2] == 7 && header[3] == 8)) {
		return corev1.ArchiveFormatZip, nil
	}
	return "", errors.New("unsupported backup archive format: expected .tar.gz or .zip")
}

// InspectArchiveStream scans a backup stream in read-only mode to discover layout and candidates.
func InspectArchiveStream(ctx context.Context, src io.Reader, maxBytes int64, opts ArchiveInspectOptions) (*ArchiveInspection, error) {
	if src == nil {
		return nil, errors.New("backup stream is required")
	}
	format, reader, err := sniffArchiveReader(src)
	if err != nil {
		return nil, err
	}
	var scanned archiveScanResult
	switch format {
	case corev1.ArchiveFormatTarGz:
		scanned, err = scanTarGzEntries(ctx, reader, maxBytes)
	case corev1.ArchiveFormatZip:
		scanned, err = scanZipEntries(ctx, reader, maxBytes)
	default:
		return nil, fmt.Errorf("unsupported archive format %q", format)
	}
	if err != nil {
		return nil, err
	}
	res := AnalyzeArchivePaths(scanned.files, scanned.dirs, opts.DefaultTarget, opts.AnchorFiles)
	res.Format = format
	res.Entries = scanned.entries
	res.FileCount = len(scanned.files)
	res.UncompressedBytes = scanned.uncompressedBytes
	res.Files = scanned.files
	res.TextFiles = scanned.textFiles
	return &res, nil
}

func sniffArchiveReader(src io.Reader) (string, io.Reader, error) {
	var buf [4]byte
	n, err := io.ReadFull(src, buf[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", nil, err
	}
	header := buf[:n]
	format, err := DetectArchiveFormat(header)
	if err != nil {
		return "", nil, err
	}
	return format, io.MultiReader(bytes.NewReader(header), src), nil
}

func scanTarGzEntries(ctx context.Context, src io.Reader, maxBytes int64) (archiveScanResult, error) {
	compLimited := &quotaCountingReader{r: fileContextReader{ctx: ctx, r: src}, maxBytes: maxBytes}
	gz, err := gzip.NewReader(compLimited)
	if err != nil {
		return archiveScanResult{}, fmt.Errorf("invalid gzip backup stream: %w", err)
	}
	defer func() { _ = gz.Close() }()
	decompLimited := &quotaCountingReader{r: gz, maxBytes: maxBytes}
	tr := tar.NewReader(decompLimited)
	filesSet, dirsSet := map[string]bool{}, map[string]bool{}
	textFiles := map[string]string{}
	textBudget := maxArchiveTotalTextBytes
	var totalSize int64
	entries := 0
	for ; ; entries++ {
		if err := ctx.Err(); err != nil {
			return archiveScanResult{}, err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, ErrFileQuotaExceeded) {
				return archiveScanResult{}, ErrFileQuotaExceeded
			}
			return archiveScanResult{}, fmt.Errorf("read tar header: %w", err)
		}
		if entries >= maxArchiveEntries {
			return archiveScanResult{}, errors.New("backup contains too many entries")
		}
		totalSize, err = collectTarInspectEntry(tr, hdr, filesSet, dirsSet, textFiles, &textBudget, maxBytes, totalSize)
		if err != nil {
			return archiveScanResult{}, err
		}
	}
	if err := drainBoundedGzipTrailer(decompLimited); err != nil {
		return archiveScanResult{}, err
	}
	return archiveScanResult{
		files:             sortedMapKeys(filesSet),
		dirs:              sortedMapKeys(dirsSet),
		textFiles:         textFiles,
		entries:           entries,
		uncompressedBytes: totalSize,
	}, nil
}

func collectTarInspectEntry(tr *tar.Reader, hdr *tar.Header, filesSet, dirsSet map[string]bool, textFiles map[string]string, textBudget *int, maxBytes, totalSize int64) (int64, error) {
	isDir := hdr.Typeflag == tar.TypeDir
	if !isDir && hdr.Typeflag != tar.TypeReg {
		return 0, fmt.Errorf("unsupported tar entry type %d for %q: links and special files are not allowed", hdr.Typeflag, hdr.Name)
	}
	if hdr.Size < 0 || (maxBytes > 0 && (hdr.Size > maxBytes || hdr.Size > maxBytes-totalSize)) || (maxBytes <= 0 && totalSize > math.MaxInt64-hdr.Size) {
		return 0, ErrFileQuotaExceeded
	}
	rel, err := recordArchiveEntry(hdr.Name, isDir, filesSet, dirsSet)
	if err != nil {
		return 0, err
	}
	if !isDir && shouldCaptureInspectText(rel, uint64(hdr.Size), *textBudget) {
		if err := captureInspectTextReader(tr, rel, int64(maxArchiveTextFileBytes), textFiles, textBudget); err != nil {
			return 0, err
		}
	}
	return totalSize + hdr.Size, nil
}

func scanZipEntries(ctx context.Context, src io.Reader, maxBytes int64) (archiveScanResult, error) {
	ra, size, cleanup, err := openZipReaderSource(ctx, src, maxBytes)
	if err != nil {
		return archiveScanResult{}, err
	}
	defer cleanup()
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return archiveScanResult{}, fmt.Errorf("invalid zip backup stream: %w", err)
	}
	if len(zr.File) > maxArchiveEntries {
		return archiveScanResult{}, errors.New("backup contains too many entries")
	}
	filesSet, dirsSet := map[string]bool{}, map[string]bool{}
	textFiles := map[string]string{}
	textBudget := maxArchiveTotalTextBytes
	var totalSize uint64
	for _, zf := range zr.File {
		if err := ctx.Err(); err != nil {
			return archiveScanResult{}, err
		}
		totalSize, err = collectZipInspectEntry(zf, filesSet, dirsSet, textFiles, &textBudget, maxBytes, totalSize)
		if err != nil {
			return archiveScanResult{}, err
		}
	}
	return archiveScanResult{
		files:             sortedMapKeys(filesSet),
		dirs:              sortedMapKeys(dirsSet),
		textFiles:         textFiles,
		entries:           len(zr.File),
		uncompressedBytes: int64(totalSize),
	}, nil
}

func collectZipInspectEntry(zf *zip.File, filesSet, dirsSet map[string]bool, textFiles map[string]string, textBudget *int, maxBytes int64, totalSize uint64) (uint64, error) {
	mode := zf.Mode()
	if mode&os.ModeSymlink != 0 || mode&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice) != 0 {
		return 0, fmt.Errorf("unsupported zip entry mode for %q: links and special files are not allowed", zf.Name)
	}
	isDir := zf.FileInfo().IsDir() || strings.HasSuffix(zf.Name, "/") || strings.HasSuffix(zf.Name, "\\")
	if maxBytes > 0 {
		limit := uint64(maxBytes)
		if zf.UncompressedSize64 > limit || totalSize > limit-zf.UncompressedSize64 {
			return 0, ErrFileQuotaExceeded
		}
	} else if zf.UncompressedSize64 > math.MaxInt64 || totalSize > uint64(math.MaxInt64)-zf.UncompressedSize64 {
		return 0, ErrFileQuotaExceeded
	}
	rel, err := recordArchiveEntry(zf.Name, isDir, filesSet, dirsSet)
	if err != nil {
		return 0, err
	}
	if !isDir && shouldCaptureInspectText(rel, zf.UncompressedSize64, *textBudget) {
		rc, err := zf.Open()
		if err != nil {
			return 0, fmt.Errorf("open zip entry %q: %w", rel, err)
		}
		capErr := captureInspectTextReader(rc, rel, int64(maxArchiveTextFileBytes), textFiles, textBudget)
		_ = rc.Close()
		if capErr != nil {
			return 0, capErr
		}
	}
	return totalSize + zf.UncompressedSize64, nil
}

func shouldCaptureInspectText(rel string, size uint64, remainingBudget int) bool {
	if rel == "" || size == 0 || size > maxArchiveTextFileBytes || int(size) > remainingBudget {
		return false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".ini", ".txt", ".properties", ".xml", ".yaml", ".yml", ".json", ".lua", ".cfg", ".conf", ".toml", ".acf":
		return true
	default:
		return false
	}
}

func captureInspectTextReader(r io.Reader, rel string, maxFileBytes int64, dst map[string]string, budget *int) error {
	raw, err := io.ReadAll(io.LimitReader(r, maxFileBytes+1))
	if err != nil {
		return fmt.Errorf("read text entry %q: %w", rel, err)
	}
	if int64(len(raw)) > maxFileBytes || len(raw) > *budget {
		return nil
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return nil
	}
	dst[rel] = string(raw)
	*budget -= len(raw)
	return nil
}

func openZipReaderSource(ctx context.Context, src io.Reader, maxBytes int64) (io.ReaderAt, int64, func(), error) {
	effectiveMax := int64(zipInspectSpoolMaxBytes)
	if maxBytes > 0 && maxBytes < effectiveMax {
		effectiveMax = maxBytes
	}
	probeLimit := min(int64(zipInspectMemoryFastPathBytes), effectiveMax)
	prefix, err := io.ReadAll(io.LimitReader(fileContextReader{ctx: ctx, r: src}, probeLimit+1))
	if err != nil {
		return nil, 0, nil, err
	}
	if int64(len(prefix)) > effectiveMax {
		return nil, 0, nil, ErrFileQuotaExceeded
	}
	if int64(len(prefix)) <= int64(zipInspectMemoryFastPathBytes) {
		return bytes.NewReader(prefix), int64(len(prefix)), func() {}, nil
	}
	return spoolZipRemainderToTemp(ctx, prefix, src, effectiveMax)
}

func spoolZipRemainderToTemp(ctx context.Context, prefix []byte, src io.Reader, effectiveMax int64) (io.ReaderAt, int64, func(), error) {
	select {
	case zipSpoolSem <- struct{}{}:
	case <-ctx.Done():
		return nil, 0, nil, ctx.Err()
	}
	tmp, err := os.CreateTemp("", "gp-zip-*")
	if err != nil {
		<-zipSpoolSem
		return nil, 0, nil, err
	}
	var releaseOnce sync.Once
	cleanup := func() {
		releaseOnce.Do(func() {
			name := tmp.Name()
			_ = tmp.Close()
			_ = os.Remove(name)
			<-zipSpoolSem
		})
	}
	if _, err := tmp.Write(prefix); err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	remLimit := effectiveMax - int64(len(prefix)) + 1
	written, err := io.Copy(tmp, io.LimitReader(fileContextReader{ctx: ctx, r: src}, remLimit))
	if err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	total := int64(len(prefix)) + written
	if total > effectiveMax {
		cleanup()
		return nil, 0, nil, ErrFileQuotaExceeded
	}
	return tmp, total, cleanup, nil
}

func recordArchiveEntry(rawName string, isDir bool, filesSet, dirsSet map[string]bool) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(rawName), "/\\")
	if trimmed == "" || trimmed == "." {
		if !isDir {
			return "", errors.New("backup root entry must be a directory")
		}
		return "", nil
	}
	rel, err := metav1.NormalizeArchiveEntryPath(trimmed, false)
	if err != nil {
		return "", fmt.Errorf("invalid archive entry path %q", rawName)
	}
	if IsReservedMetadataPath(rel) {
		return "", nil
	}
	for parent := path.Dir(rel); parent != "."; parent = path.Dir(parent) {
		dirsSet[parent] = true
	}
	if isDir {
		if filesSet[rel] {
			return "", fmt.Errorf("duplicate conflicting archive entry %q", rel)
		}
		dirsSet[rel] = true
		return rel, nil
	}
	if filesSet[rel] || dirsSet[rel] {
		return "", fmt.Errorf("duplicate conflicting archive entry %q", rel)
	}
	filesSet[rel] = true
	return rel, nil
}

// ListArchiveImmediateEntries returns sorted immediate child directories (suffixed "/") and files
// directly inside sourcePath without app-specific hardcoding.
func ListArchiveImmediateEntries(files, dirs []string, sourcePath string, limit int) []string {
	if strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	norm, err := metav1.NormalizeArchiveEntryPath(sourcePath, true)
	if err != nil {
		return nil
	}
	prefix := ""
	if norm != "." {
		prefix = norm + "/"
	}
	dirChildren, fileChildren := map[string]bool{}, map[string]bool{}
	collectImmediateChildNames(dirs, files, prefix, dirChildren, fileChildren)
	out := make([]string, 0, len(dirChildren)+len(fileChildren))
	for _, d := range sortedMapKeys(dirChildren) {
		out = append(out, d+"/")
	}
	out = append(out, sortedMapKeys(fileChildren)...)
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func collectImmediateChildNames(dirs, files []string, prefix string, dirChildren, fileChildren map[string]bool) {
	for _, d := range dirs {
		if d == "" || d == "." {
			continue
		}
		if prefix == "" {
			if !strings.Contains(d, "/") {
				dirChildren[d] = true
			}
			continue
		}
		if rest, ok := strings.CutPrefix(d, prefix); ok && rest != "" && !strings.Contains(rest, "/") {
			dirChildren[rest] = true
		}
	}
	for _, f := range files {
		rest := f
		if prefix != "" {
			var ok bool
			rest, ok = strings.CutPrefix(f, prefix)
			if !ok || rest == "" {
				continue
			}
		}
		if first, _, hasSlash := strings.Cut(rest, "/"); hasSlash {
			dirChildren[first] = true
		} else {
			fileChildren[rest] = true
		}
	}
}

// AnalyzeArchivePaths resolves candidate source roots and selectable directories from normalized archive entries.
func AnalyzeArchivePaths(filePaths, dirPaths []string, defaultTarget string, anchorFiles []string) ArchiveInspection {
	normTarget := normalizeOptionalRelPath(defaultTarget)
	candidates := findAnchorCandidates(filePaths, dirPaths, anchorFiles)
	recommended := resolveRecommendedSource(filePaths, dirPaths, normTarget, anchorFiles, &candidates)
	dirs := buildSelectableDirectories(dirPaths, candidates)
	return ArchiveInspection{
		RecommendedSource: recommended,
		CandidateSources:  candidates,
		Directories:       dirs,
		FileCount:         len(filePaths),
		Files:             filePaths,
	}
}

func normalizeOptionalRelPath(raw string) string {
	norm, err := metav1.NormalizeRelativeFilePath(strings.TrimSpace(raw), true)
	if err != nil || norm == "" {
		return "."
	}
	return norm
}

func findAnchorCandidates(filePaths, dirPaths, anchorFiles []string) []string {
	anchors := make([]string, 0, len(anchorFiles))
	for _, a := range anchorFiles {
		if norm, err := metav1.NormalizeArchiveEntryPath(a, false); err == nil {
			anchors = append(anchors, norm)
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	seen := map[string]bool{}
	collectAnchorMatches(filePaths, anchors, seen)
	collectAnchorMatches(dirPaths, anchors, seen)
	return sortedMapKeys(seen)
}

func collectAnchorMatches(entries, anchors []string, seen map[string]bool) {
	for _, entry := range entries {
		for _, anchor := range anchors {
			if entry == anchor {
				seen["."] = true
			} else if strings.HasSuffix(entry, "/"+anchor) {
				seen[strings.TrimSuffix(entry, "/"+anchor)] = true
			}
		}
	}
}

func resolveRecommendedSource(filePaths, dirPaths []string, target string, anchorFiles []string, candidates *[]string) string {
	if target != "." && archiveContainsPrefix(filePaths, dirPaths, target) {
		if !containsString(*candidates, target) {
			*candidates = append([]string{target}, *candidates...)
		}
		return target
	}
	if len(*candidates) == 1 {
		return (*candidates)[0]
	}
	if len(*candidates) == 0 && len(anchorFiles) == 0 && target == "." {
		*candidates = []string{"."}
		return "."
	}
	return ""
}

func archiveContainsPrefix(filePaths, dirPaths []string, prefix string) bool {
	sub := prefix + "/"
	for _, d := range dirPaths {
		if d == prefix || strings.HasPrefix(d, sub) {
			return true
		}
	}
	for _, f := range filePaths {
		if f == prefix || strings.HasPrefix(f, sub) {
			return true
		}
	}
	return false
}

func buildSelectableDirectories(dirPaths, candidates []string) []string {
	set := map[string]bool{".": true}
	for _, c := range candidates {
		set[c] = true
	}
	sorted := append([]string(nil), dirPaths...)
	sort.Strings(sorted)
	count := 0
	for _, d := range sorted {
		if d == "" || d == "." || set[d] {
			continue
		}
		if strings.Count(d, "/")+1 > maxArchiveDirectoryDepth {
			continue
		}
		set[d] = true
		count++
		if count >= maxArchiveDirectories {
			break
		}
	}
	out := make([]string, 0, len(set))
	out = append(out, ".")
	delete(set, ".")
	rest := sortedMapKeys(set)
	return append(out, rest...)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sortedMapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
