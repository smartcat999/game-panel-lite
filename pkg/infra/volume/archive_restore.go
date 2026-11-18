package volume

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// RestoreOptions parameterizes archive staging, directory remapping, and promotion.
type RestoreOptions struct {
	SourceSubpath string
	TargetSubpath string
	ReplaceTarget bool
	OnProgress    func(extractedBytes int64, entries int)
}

// StagedRestore holds a validated, completely extracted archive ready for atomic promotion.
type StagedRestore struct {
	root             *os.Root
	stage            string
	targetRel        string
	replaceTarget    bool
	dirs             map[string]bool
	files            map[string]bool
	recoveryRequired bool
	closed           bool
}

// StageArchiveToRoot validates a tar.gz or zip stream and stages mapped files without mutating live targets.
func StageArchiveToRoot(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64, opts RestoreOptions) (*StagedRestore, error) {
	if src == nil {
		return nil, errors.New("backup stream is required")
	}
	format, reader, err := sniffArchiveReader(src)
	if err != nil {
		return nil, err
	}
	return stageFormattedArchive(ctx, root, format, reader, maxBytes, opts)
}

func stageTarGzArchiveToRoot(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64, opts RestoreOptions) (*StagedRestore, error) {
	return stageFormattedArchive(ctx, root, corev1.ArchiveFormatTarGz, src, maxBytes, opts)
}

func stageFormattedArchive(ctx context.Context, root *os.Root, format string, src io.Reader, maxBytes int64, opts RestoreOptions) (*StagedRestore, error) {
	sourceRel, targetRel, err := normalizeRestoreMapping(opts.SourceSubpath, opts.TargetSubpath)
	if err != nil {
		return nil, err
	}
	sr, staged, err := openRestoreStage(root, targetRel, opts.ReplaceTarget)
	if err != nil {
		return nil, err
	}
	defer func() { _ = staged.Close() }()
	var matched int
	switch format {
	case corev1.ArchiveFormatTarGz:
		matched, err = stageTarGzEntriesMapped(ctx, staged, src, maxBytes, sourceRel, targetRel, sr.dirs, sr.files, opts.OnProgress)
	case corev1.ArchiveFormatZip:
		matched, err = stageZipEntriesMapped(ctx, root, staged, src, maxBytes, sourceRel, targetRel, sr.dirs, sr.files, opts.OnProgress)
	default:
		err = fmt.Errorf("unsupported archive format %q", format)
	}
	if err == nil && sourceRel != "." && matched == 0 {
		err = fmt.Errorf("source directory %q was not found in backup archive", sourceRel)
	}
	if err != nil {
		_ = sr.Close()
		return nil, err
	}
	return sr, nil
}

func openRestoreStage(root *os.Root, targetRel string, replaceTarget bool) (*StagedRestore, *os.Root, error) {
	stage, err := privateFileName(".gp-restore-")
	if err != nil {
		return nil, nil, err
	}
	if err := root.Mkdir(stage, 0o700); err != nil {
		return nil, nil, err
	}
	incoming := path.Join(stage, "incoming")
	if err := root.Mkdir(incoming, 0o700); err != nil {
		_ = root.RemoveAll(stage)
		return nil, nil, err
	}
	staged, err := root.OpenRoot(incoming)
	if err != nil {
		_ = root.RemoveAll(stage)
		return nil, nil, err
	}
	return &StagedRestore{
		root:          root,
		stage:         stage,
		targetRel:     targetRel,
		replaceTarget: replaceTarget,
		dirs:          map[string]bool{},
		files:         map[string]bool{},
	}, staged, nil
}

// Promote atomically applies staged files into the live workspace, rolling back on error.
func (sr *StagedRestore) Promote(ctx context.Context) error {
	if sr == nil || sr.closed {
		return errors.New("staged restore is closed")
	}
	var err error
	if sr.replaceTarget && sr.targetRel != "." {
		err = promoteSubtreeSwap(ctx, sr.root, sr.stage, sr.targetRel)
	} else {
		err = promoteRestoredFiles(ctx, sr.root, sr.stage, sr.dirs, sr.files)
	}
	if errors.Is(err, ErrRestoreRecoveryRequired) {
		sr.recoveryRequired = true
	}
	return err
}

// Close deletes temporary staging files unless failure requires manual recovery inspection.
func (sr *StagedRestore) Close() error {
	if sr == nil || sr.closed {
		return nil
	}
	sr.closed = true
	if sr.recoveryRequired {
		return nil
	}
	return sr.root.RemoveAll(sr.stage)
}

func normalizeRestoreMapping(rawSource, rawTarget string) (string, string, error) {
	sourceRel, err := metav1.NormalizeArchiveEntryPath(rawSource, true)
	if err != nil {
		return "", "", fmt.Errorf("invalid restore sourcePath %q: %w", rawSource, err)
	}
	targetRel, err := metav1.NormalizeArchiveEntryPath(rawTarget, true)
	if err != nil {
		return "", "", fmt.Errorf("invalid restore targetPath %q: %w", rawTarget, err)
	}
	if IsReservedMetadataPath(sourceRel) || IsReservedMetadataPath(targetRel) {
		return "", "", errors.New("reserved internal workspace path cannot be used for restore")
	}
	return sourceRel, targetRel, nil
}

// IsReservedMetadataPath reports whether any path segment begins with corev1.ReservedWorkspacePrefix (".gp-").
func IsReservedMetadataPath(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(strings.TrimSpace(rel)), "/") {
		if strings.HasPrefix(seg, corev1.ReservedWorkspacePrefix) {
			return true
		}
	}
	return false
}

func mapArchiveEntryPath(rawName string, isDir bool, sourceRel, targetRel string) (string, bool, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(rawName), "/\\")
	if trimmed == "" || trimmed == "." {
		if !isDir {
			return "", false, errors.New("backup root entry must be a directory")
		}
		return "", false, nil
	}
	entryRel, err := metav1.NormalizeArchiveEntryPath(trimmed, false)
	if err != nil {
		return "", false, fmt.Errorf("invalid archive entry path %q", rawName)
	}
	if IsReservedMetadataPath(entryRel) {
		return "", false, nil
	}
	inner, matched, err := stripSourcePrefix(entryRel, isDir, sourceRel)
	if err != nil || !matched {
		return "", false, err
	}
	if inner == "." {
		return targetRel, true, nil
	}
	mapped := inner
	if targetRel != "." {
		mapped = path.Join(targetRel, inner)
	}
	if IsReservedMetadataPath(mapped) {
		return "", false, nil
	}
	return mapped, true, nil
}

func stripSourcePrefix(entryRel string, isDir bool, sourceRel string) (string, bool, error) {
	if sourceRel == "." {
		return entryRel, true, nil
	}
	if entryRel == sourceRel {
		if !isDir {
			return "", false, fmt.Errorf("source path %q refers to a file, expected a directory", sourceRel)
		}
		return ".", true, nil
	}
	if strings.HasPrefix(entryRel, sourceRel+"/") {
		return strings.TrimPrefix(entryRel, sourceRel+"/"), true, nil
	}
	return "", false, nil
}

func stageTarGzEntriesMapped(ctx context.Context, staged *os.Root, src io.Reader, maxBytes int64, sourceRel, targetRel string, dirs, files map[string]bool, onProgress func(int64, int)) (int, error) {
	compLimited := &quotaCountingReader{r: fileContextReader{ctx: ctx, r: src}, maxBytes: maxBytes}
	gz, err := gzip.NewReader(compLimited)
	if err != nil {
		return 0, fmt.Errorf("invalid gzip backup stream: %w", err)
	}
	defer func() { _ = gz.Close() }()
	decompLimited := &quotaCountingReader{r: gz, maxBytes: maxBytes}
	tr := tar.NewReader(decompLimited)
	var totalExtracted int64
	matched := 0
	for entries := 0; ; entries++ {
		if err := ctx.Err(); err != nil {
			return matched, err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, ErrFileQuotaExceeded) {
				return matched, ErrFileQuotaExceeded
			}
			return matched, fmt.Errorf("read tar header: %w", err)
		}
		if entries >= maxArchiveEntries {
			return matched, errors.New("backup contains too many entries")
		}
		n, inc, err := extractMappedTarEntry(ctx, staged, tr, hdr, sourceRel, targetRel, dirs, files, maxBytes, totalExtracted)
		if err != nil {
			return matched, err
		}
		matched += inc
		totalExtracted += n
		if onProgress != nil {
			onProgress(totalExtracted, entries+1)
		}
	}
	if err := drainBoundedGzipTrailer(decompLimited); err != nil {
		return matched, err
	}
	return matched, nil
}

func extractMappedTarEntry(ctx context.Context, staged *os.Root, tr *tar.Reader, hdr *tar.Header, sourceRel, targetRel string, dirs, files map[string]bool, maxBytes, totalExtracted int64) (int64, int, error) {
	if hdr.Typeflag != tar.TypeDir && hdr.Typeflag != tar.TypeReg {
		return 0, 0, fmt.Errorf("unsupported tar entry type %d for %q: links and special files are not allowed", hdr.Typeflag, hdr.Name)
	}
	if hdr.Size < 0 {
		return 0, 0, errors.New("invalid negative tar entry size")
	}
	if maxBytes > 0 && hdr.Size > maxBytes-totalExtracted {
		return 0, 0, ErrFileQuotaExceeded
	}
	isDir := hdr.Typeflag == tar.TypeDir
	mapped, include, err := mapArchiveEntryPath(hdr.Name, isDir, sourceRel, targetRel)
	if err != nil || !include {
		return 0, 0, err
	}
	if isDir {
		return 0, 1, ensureStagedDir(staged, mapped, dirs, files)
	}
	if err := prepareMappedFileParents(staged, mapped, dirs, files); err != nil {
		return 0, 0, err
	}
	n, err := WriteStreamInRootContext(ctx, staged, mapped, tr, hdr.Size)
	if err != nil {
		return 0, 0, fmt.Errorf("extract file %s: %w", mapped, err)
	}
	if n != hdr.Size {
		return 0, 0, io.ErrUnexpectedEOF
	}
	files[mapped] = true
	return n, 1, applyStagedMetadata(staged, mapped, os.FileMode(hdr.Mode), hdr.ModTime)
}

func stageZipEntriesMapped(ctx context.Context, root, staged *os.Root, src io.Reader, maxBytes int64, sourceRel, targetRel string, dirs, files map[string]bool, onProgress func(int64, int)) (int, error) {
	tmp, tmpName, size, err := spoolZipInRoot(ctx, root, src, maxBytes)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tmp.Close()
		_ = root.Remove(tmpName)
	}()
	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return 0, fmt.Errorf("invalid zip backup stream: %w", err)
	}
	if len(zr.File) > maxArchiveEntries {
		return 0, errors.New("backup contains too many entries")
	}
	var totalExtracted int64
	matched := 0
	for i, zf := range zr.File {
		if err := ctx.Err(); err != nil {
			return matched, err
		}
		n, inc, err := extractMappedZipEntry(ctx, staged, zf, sourceRel, targetRel, dirs, files, maxBytes, totalExtracted)
		if err != nil {
			return matched, err
		}
		matched += inc
		totalExtracted += n
		if onProgress != nil {
			onProgress(totalExtracted, i+1)
		}
	}
	return matched, nil
}

func spoolZipInRoot(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64) (*os.File, string, int64, error) {
	if root == nil {
		return nil, "", 0, errors.New("workspace root is required")
	}
	tmpName, err := privateFileName(".gp-restore-zip-")
	if err != nil {
		return nil, "", 0, err
	}
	tmp, err := root.OpenFile(tmpName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", 0, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = root.Remove(tmpName)
		}
	}()
	reader := io.Reader(fileContextReader{ctx: ctx, r: src})
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	written, err := io.Copy(tmp, reader)
	if err != nil {
		return nil, "", 0, err
	}
	if maxBytes > 0 && written > maxBytes {
		return nil, "", 0, ErrFileQuotaExceeded
	}
	cleanup = false
	return tmp, tmpName, written, nil
}

func extractMappedZipEntry(ctx context.Context, staged *os.Root, zf *zip.File, sourceRel, targetRel string, dirs, files map[string]bool, maxBytes, totalExtracted int64) (int64, int, error) {
	mode := zf.Mode()
	if mode&os.ModeSymlink != 0 || mode&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice) != 0 {
		return 0, 0, fmt.Errorf("unsupported zip entry mode for %q: links and special files are not allowed", zf.Name)
	}
	isDir := zf.FileInfo().IsDir() || strings.HasSuffix(zf.Name, "/") || strings.HasSuffix(zf.Name, "\\")
	mapped, include, err := mapArchiveEntryPath(zf.Name, isDir, sourceRel, targetRel)
	if err != nil || !include {
		return 0, 0, err
	}
	if isDir {
		return 0, 1, ensureStagedDir(staged, mapped, dirs, files)
	}
	n, err := extractZipRegularFile(ctx, staged, zf, mapped, dirs, files, maxBytes, totalExtracted)
	if err != nil {
		return 0, 0, err
	}
	return n, 1, nil
}

func extractZipRegularFile(ctx context.Context, staged *os.Root, zf *zip.File, mapped string, dirs, files map[string]bool, maxBytes, totalExtracted int64) (int64, error) {
	var remaining int64
	if maxBytes > 0 {
		remaining = maxBytes - totalExtracted
		if zf.UncompressedSize64 > uint64(remaining) {
			return 0, ErrFileQuotaExceeded
		}
	}
	if err := prepareMappedFileParents(staged, mapped, dirs, files); err != nil {
		return 0, err
	}
	rc, err := zf.Open()
	if err != nil {
		return 0, fmt.Errorf("open zip entry %s: %w", zf.Name, err)
	}
	defer func() { _ = rc.Close() }()
	limited := newZipLimitReader(ctx, rc, maxBytes > 0, remaining)
	n, err := WriteStreamInRootContext(ctx, staged, mapped, limited, remaining)
	if err != nil {
		return 0, fmt.Errorf("extract file %s: %w", mapped, err)
	}
	files[mapped] = true
	if err := applyStagedMetadata(staged, mapped, zf.Mode(), zf.Modified); err != nil {
		return 0, err
	}
	return n, nil
}

type zipLimitReader struct {
	ctx       context.Context
	r         io.Reader
	hasLimit  bool
	remaining int64
}

func newZipLimitReader(ctx context.Context, r io.Reader, hasLimit bool, remaining int64) *zipLimitReader {
	return &zipLimitReader{ctx: ctx, r: r, hasLimit: hasLimit, remaining: remaining}
}

func (z *zipLimitReader) Read(p []byte) (int, error) {
	if err := z.ctx.Err(); err != nil {
		return 0, err
	}
	if z.hasLimit && z.remaining == 0 {
		var extra [1]byte
		n, err := z.r.Read(extra[:])
		if n > 0 {
			return 0, ErrFileQuotaExceeded
		}
		if err == nil {
			return 0, nil
		}
		return 0, err
	}
	buf := p
	if z.hasLimit && int64(len(buf)) > z.remaining {
		buf = p[:z.remaining]
	}
	n, err := z.r.Read(buf)
	if z.hasLimit {
		z.remaining -= int64(n)
	}
	return n, err
}

func ensureStagedDir(staged *os.Root, mapped string, dirs, files map[string]bool) error {
	if mapped == "." {
		return nil
	}
	if files[mapped] {
		return fmt.Errorf("duplicate conflicting archive entry %q", mapped)
	}
	recordParentDirs(mapped, dirs)
	dirs[mapped] = true
	return staged.MkdirAll(filepath.FromSlash(mapped), 0o755)
}

func prepareMappedFileParents(staged *os.Root, mapped string, dirs, files map[string]bool) error {
	if files[mapped] || dirs[mapped] {
		return fmt.Errorf("duplicate conflicting archive entry %q", mapped)
	}
	recordParentDirs(mapped, dirs)
	if parent := path.Dir(mapped); parent != "." {
		return staged.MkdirAll(filepath.FromSlash(parent), 0o755)
	}
	return nil
}

func recordParentDirs(rel string, dirs map[string]bool) {
	for parent := path.Dir(rel); parent != "."; parent = path.Dir(parent) {
		dirs[parent] = true
	}
}

func applyStagedMetadata(staged *os.Root, mapped string, rawMode os.FileMode, modTime time.Time) error {
	mode := rawMode & 0o755
	if mode == 0 {
		mode = 0o644
	}
	if err := staged.Chmod(filepath.FromSlash(mapped), mode); err != nil {
		return err
	}
	if !modTime.IsZero() {
		if err := staged.Chtimes(filepath.FromSlash(mapped), time.Now(), modTime); err != nil {
			return err
		}
	}
	return nil
}

func promoteSubtreeSwap(ctx context.Context, root *os.Root, stage, targetRel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	exists, err := validateSubtreeSwapTarget(root, targetRel)
	if err != nil {
		return err
	}
	createdDirs, err := createMissingParentDirs(root, targetRel)
	if err != nil {
		return rollbackSubtreeDirs(root, createdDirs, err)
	}
	backupPath := filepath.FromSlash(path.Join(stage, "previous_subtree"))
	stagedPath := filepath.FromSlash(path.Join(stage, "incoming", targetRel))
	targetFS := filepath.FromSlash(targetRel)
	if exists {
		if err := root.Rename(targetFS, backupPath); err != nil {
			return rollbackSubtreeDirs(root, createdDirs, err)
		}
	}
	if err := root.Rename(stagedPath, targetFS); err != nil {
		return rollbackSubtreeSwap(root, targetFS, backupPath, exists, createdDirs, err)
	}
	return nil
}

func validateSubtreeSwapTarget(root *os.Root, targetRel string) (bool, error) {
	if parent := path.Dir(targetRel); parent != "." {
		parts := strings.Split(parent, "/")
		for i := range parts {
			anc := path.Join(parts[:i+1]...)
			info, err := root.Lstat(filepath.FromSlash(anc))
			if err == nil && !info.IsDir() {
				return false, fmt.Errorf("restore target parent %q conflicts with existing file or link", anc)
			}
			if err != nil && !os.IsNotExist(err) {
				return false, err
			}
		}
	}
	info, err := root.Lstat(filepath.FromSlash(targetRel))
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("restore target %q conflicts with existing non-directory", targetRel)
		}
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func createMissingParentDirs(root *os.Root, targetRel string) ([]string, error) {
	parent := path.Dir(targetRel)
	if parent == "." {
		return nil, nil
	}
	parts := strings.Split(parent, "/")
	var created []string
	for i := range parts {
		anc := path.Join(parts[:i+1]...)
		if _, err := root.Lstat(filepath.FromSlash(anc)); os.IsNotExist(err) {
			if err := root.Mkdir(filepath.FromSlash(anc), 0o755); err != nil {
				return created, err
			}
			created = append(created, anc)
		} else if err != nil {
			return created, err
		}
	}
	return created, nil
}

func rollbackSubtreeSwap(root *os.Root, targetFS, backupPath string, hadExisting bool, createdDirs []string, cause error) error {
	var rollbackErr error
	if hadExisting {
		rollbackErr = errors.Join(rollbackErr, root.Rename(backupPath, targetFS))
	}
	for i := len(createdDirs) - 1; i >= 0; i-- {
		rollbackErr = errors.Join(rollbackErr, root.Remove(filepath.FromSlash(createdDirs[i])))
	}
	if rollbackErr != nil {
		return errors.Join(cause, ErrRestoreRecoveryRequired, rollbackErr)
	}
	return cause
}

func rollbackSubtreeDirs(root *os.Root, createdDirs []string, cause error) error {
	var rollbackErr error
	for i := len(createdDirs) - 1; i >= 0; i-- {
		rollbackErr = errors.Join(rollbackErr, root.Remove(filepath.FromSlash(createdDirs[i])))
	}
	if rollbackErr != nil {
		return errors.Join(cause, ErrRestoreRecoveryRequired, rollbackErr)
	}
	return cause
}

const maxGzipTrailerBytes = 1 << 20

func drainBoundedGzipTrailer(r io.Reader) error {
	n, err := io.Copy(io.Discard, io.LimitReader(r, maxGzipTrailerBytes+1))
	if errors.Is(err, ErrFileQuotaExceeded) {
		return ErrFileQuotaExceeded
	}
	if err != nil {
		return fmt.Errorf("verify gzip backup stream: %w", err)
	}
	if n > maxGzipTrailerBytes {
		return ErrFileQuotaExceeded
	}
	return nil
}
