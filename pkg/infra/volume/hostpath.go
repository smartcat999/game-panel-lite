package volume

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

const DefaultHostPathDriverName = corev1.DefaultHostPathDriverName

func init() {
	Register(DefaultHostPathDriverName, NewHostPathDriver)
}

// HostPathDriver implements the default host-directory volume plugin ("hostpath")
// with os.Root path-traversal isolation.
type HostPathDriver struct {
	workDir string
}

// NewHostPathDriver constructs a HostPathDriver rooted at workDir.
func NewHostPathDriver(workDir string) Driver {
	return &HostPathDriver{workDir: workDir}
}

func (d *HostPathDriver) Handler() string { return DefaultHostPathDriverName }

func (d *HostPathDriver) ResolveWorkDir(uid, tenant, name string, create bool) (string, error) {
	return ResolveInstanceWorkDir(d.workDir, uid, tenant, name, create)
}

func (d *HostPathDriver) OpenRoot(uid, tenant, name string, create bool) (*os.Root, string, error) {
	return OpenInstanceRoot(d.workDir, uid, tenant, name, create)
}

func (d *HostPathDriver) RemoveWorkDir(uid, tenant, name string) error {
	return RemoveInstanceWorkDir(d.workDir, uid, tenant, name)
}

func (d *HostPathDriver) WriteFiles(hostDir string, files []corev1.InstanceFile) error {
	return WriteInstanceFiles(hostDir, files)
}

// ErrFileQuotaExceeded indicates that a file write or archive stream exceeded its allowed byte quota.
var ErrFileQuotaExceeded = errors.New("file size exceeds allowed MaxBytes quota")

// ErrRestoreRecoveryRequired preserves staged originals if even rollback fails.
var ErrRestoreRecoveryRequired = errors.New("restore rollback incomplete; staged originals require recovery")

// ResolveInstanceWorkDir resolves the host working directory for an instance (<workDir>/instances/<uid>).
func ResolveInstanceWorkDir(workDir, uid, tenant, name string, create bool) (string, error) {
	cleanBase := filepath.Clean(strings.TrimSpace(workDir))
	if cleanBase == "" || cleanBase == "." {
		return "", errors.New("instance base workDir is required")
	}
	if err := metav1.ValidateSingleSegmentName("tenant", tenant); err != nil {
		return "", err
	}
	if err := metav1.ValidateSingleSegmentName("instance", name); err != nil {
		return "", err
	}
	uid = strings.TrimSpace(uid)
	if uid != "" {
		if err := metav1.ValidateSingleSegmentName("instance uid", uid); err != nil {
			return "", err
		}
		uidDir := filepath.Join(cleanBase, "instances", uid)
		if create {
			if err := os.MkdirAll(uidDir, 0o755); err != nil {
				return "", fmt.Errorf("create instance workdir: %w", err)
			}
		}
		return uidDir, nil
	}
	return "", errors.New("instance UID is required for workdir access")
}

// OpenInstanceRoot resolves the instance working directory and opens an os.Root boundary around it.
func OpenInstanceRoot(workDir, uid, tenant, name string, create bool) (*os.Root, string, error) {
	dir, err := ResolveInstanceWorkDir(workDir, uid, tenant, name, create)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", err
	}
	return root, dir, nil
}

// RemoveInstanceWorkDir removes the instance working directory inside workDir using os.Root isolation.
func RemoveInstanceWorkDir(workDir, uid, tenant, name string) error {
	if err := metav1.ValidateSingleSegmentName("instance uid", strings.TrimSpace(uid)); err != nil {
		return err
	}
	cleanBase := filepath.Clean(strings.TrimSpace(workDir))
	if cleanBase == "" || cleanBase == "." {
		return nil
	}
	if _, err := os.Stat(cleanBase); os.IsNotExist(err) {
		return nil
	}
	baseRoot, err := os.OpenRoot(cleanBase)
	if err != nil {
		return err
	}
	defer func() { _ = baseRoot.Close() }()

	if uid = strings.TrimSpace(uid); uid != "" {
		if err := metav1.ValidateSingleSegmentName("instance uid", uid); err != nil {
			return err
		}
		if err := baseRoot.RemoveAll(filepath.Join("instances", uid)); err != nil && !os.IsNotExist(err) {
			return err
		}
		_ = baseRoot.RemoveAll(filepath.Join("state", "instances", uid))
		_ = baseRoot.Remove(filepath.Join(".quota", uid+".json"))
		return nil
	}
	return errors.New("instance UID is required for workdir cleanup")
}

func rejectReservedMetadataPath(rel string) error {
	if IsReservedMetadataPath(rel) {
		return fmt.Errorf("reserved workspace path %q cannot be modified", rel)
	}
	return nil
}

// ValidateInstanceFiles validates relative file paths on an InstanceSpec.
func ValidateInstanceFiles(files []corev1.InstanceFile) error {
	for _, f := range files {
		rel, err := metav1.NormalizeRelativeFilePath(f.Path, false)
		if err != nil {
			return err
		}
		if err := rejectReservedMetadataPath(rel); err != nil {
			return err
		}
	}
	return nil
}

// WriteInstanceFiles writes rendered configuration files inside hostDir using os.Root isolation.
func WriteInstanceFiles(hostDir string, files []corev1.InstanceFile) error {
	baseDir := filepath.Clean(hostDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create instance workdir: %w", err)
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return fmt.Errorf("open instance root: %w", err)
	}
	defer func() { _ = root.Close() }()

	for _, f := range files {
		if err := metav1.ValidateRelativeFilePath(f.Path); err != nil {
			return err
		}
		rel, err := metav1.NormalizeRelativeFilePath(f.Path, false)
		if err != nil {
			return err
		}
		parent := path.Dir(rel)
		if parent != "." && parent != "" {
			if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", f.Path, err)
			}
		}
		if _, err := WriteStreamInRoot(root, rel, strings.NewReader(f.Content), 0); err != nil {
			return fmt.Errorf("write file %s: %w", f.Path, err)
		}
	}
	return nil
}

// ListEntriesInRoot lists a directory inside root using os.Root.FS().
func ListEntriesInRoot(root *os.Root, rawPath string) ([]corev1.InstanceFileEntry, error) {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, true)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root.FS(), rel)
	if err != nil {
		return nil, err
	}
	items := make([]corev1.InstanceFileEntry, 0, len(entries))
	for _, entry := range entries {
		if IsReservedMetadataPath(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryPath := "/" + entry.Name()
		if rel != "." {
			entryPath = "/" + path.Join(rel, entry.Name())
		}
		items = append(items, corev1.InstanceFileEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC(),
		})
	}
	return items, nil
}

// StatInRoot returns the FileInfo and normalized relative path inside root.
func StatInRoot(root *os.Root, rawPath string, allowRoot bool) (os.FileInfo, string, error) {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, allowRoot)
	if err != nil {
		return nil, "", err
	}
	if rel != "." {
		if err := rejectReservedMetadataPath(rel); err != nil {
			return nil, "", err
		}
	}
	info, err := root.Stat(filepath.FromSlash(rel))
	if err != nil {
		return nil, "", err
	}
	return info, rel, nil
}

// OpenReadInRoot opens a regular file inside root for reading.
func OpenReadInRoot(root *os.Root, rawPath string) (*os.File, os.FileInfo, string, error) {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return nil, nil, "", err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return nil, nil, "", err
	}
	if err := validateRegularTarget(root, rel, false); err != nil {
		return nil, nil, "", err
	}
	f, err := root.OpenFile(filepath.FromSlash(rel), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		if err == nil {
			err = errors.New("only regular files can be read")
		}
		return nil, nil, "", err
	}
	return f, info, rel, nil
}

// ReadFileInRoot reads a file inside root via os.Root.
func ReadFileInRoot(root *os.Root, rawPath string) ([]byte, error) {
	f, _, _, err := OpenReadInRoot(root, rawPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func validateRegularTarget(root *os.Root, rel string, allowMissing bool) error {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := range parts {
		name := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, err := root.Lstat(name)
		if err != nil {
			if allowMissing && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return fmt.Errorf("file parent %q must be a directory without links", name)
			}
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("file %q must be regular without links", name)
		}
	}
	return nil
}

func privateFileName(prefix string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(entropy[:]), nil
}

type fileContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r fileContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// WriteStreamInRoot replaces a regular file only after the whole stream and quota checks succeed.
func WriteStreamInRoot(root *os.Root, rawPath string, src io.Reader, maxBytes int64) (int64, error) {
	return WriteStreamInRootContext(context.Background(), root, rawPath, src, maxBytes)
}

// WriteStreamInRootContext replaces a regular file with context cancellation and maxBytes enforcement.
func WriteStreamInRootContext(ctx context.Context, root *os.Root, rawPath string, src io.Reader, maxBytes int64) (int64, error) {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return 0, err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return 0, err
	}
	if err := validateRegularTarget(root, rel, true); err != nil {
		return 0, err
	}
	mode := os.FileMode(0o644)
	if current, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
		mode = current.Mode().Perm()
	}
	parent := path.Dir(rel)
	if parent != "." {
		if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
			return 0, fmt.Errorf("create parent dir: %w", err)
		}
	}
	tmpName, err := privateFileName(".gp-upload-")
	if err != nil {
		return 0, err
	}
	tmpRel := filepath.FromSlash(path.Join(parent, tmpName))
	f, err := root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Remove(tmpRel) }()
	written, err := copyAndSyncTempFile(ctx, f, src, maxBytes, mode)
	if err != nil {
		return written, err
	}
	if err := validateRegularTarget(root, rel, true); err != nil {
		return written, err
	}
	return written, root.Rename(tmpRel, filepath.FromSlash(rel))
}

func copyAndSyncTempFile(ctx context.Context, f *os.File, src io.Reader, maxBytes int64, mode os.FileMode) (int64, error) {
	src = fileContextReader{ctx: ctx, r: src}
	reader := src
	if maxBytes > 0 {
		reader = io.LimitReader(src, maxBytes)
	}
	written, copyErr := io.Copy(f, reader)
	if copyErr == nil && maxBytes > 0 && written == maxBytes {
		var extra [1]byte
		if n, err := io.ReadFull(src, extra[:]); n > 0 {
			copyErr = ErrFileQuotaExceeded
		} else if err != nil && !errors.Is(err, io.EOF) {
			copyErr = err
		}
	}
	if copyErr == nil {
		copyErr = ctx.Err()
	}
	if copyErr == nil {
		copyErr = f.Chmod(mode)
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return written, err
	}
	return written, ctx.Err()
}

// MkdirAllInRoot creates a directory and its parents inside root.
func MkdirAllInRoot(root *os.Root, rawPath string) error {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return err
	}
	_, err = EnsureSafeDirInRoot(root, rel)
	return err
}

// RenameInRoot renames a file or directory inside root.
func RenameInRoot(root *os.Root, rawOldPath, rawNewPath string) error {
	oldRel, err := metav1.NormalizeRelativeFilePath(rawOldPath, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(oldRel); err != nil {
		return err
	}
	newRel, err := metav1.NormalizeRelativeFilePath(rawNewPath, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(newRel); err != nil {
		return err
	}
	osOldRel := filepath.FromSlash(oldRel)
	osNewRel := filepath.FromSlash(newRel)
	if _, err := root.Lstat(osOldRel); err != nil {
		return err
	}
	if oldRel == newRel {
		return nil
	}
	if _, err := root.Lstat(osNewRel); err == nil {
		return os.ErrExist
	}
	newParent := path.Dir(newRel)
	if newParent != "." && newParent != "" {
		if err := root.MkdirAll(filepath.FromSlash(newParent), 0o755); err != nil {
			return err
		}
	}
	return root.Rename(osOldRel, osNewRel)
}

// RemoveAllInRoot removes a file or directory tree inside root.
func RemoveAllInRoot(root *os.Root, rawPath string) error {
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return err
	}
	return root.RemoveAll(filepath.FromSlash(rel))
}

type quotaCountingWriter struct {
	w        io.Writer
	written  int64
	maxBytes int64
	onWrite  func(written int64)
}

func (q *quotaCountingWriter) Write(p []byte) (int, error) {
	if q.maxBytes > 0 && q.written+int64(len(p)) > q.maxBytes {
		return 0, ErrFileQuotaExceeded
	}
	n, err := q.w.Write(p)
	q.written += int64(n)
	if n > 0 && q.onWrite != nil {
		q.onWrite(q.written)
	}
	return n, err
}

// ArchiveRootToTarGz streams a gzipped tar archive of root to dst using os.Root.
func ArchiveRootToTarGz(root *os.Root, dst io.Writer, maxBytes int64) (int64, string, error) {
	return ArchivePathsToTarGz(root, nil, dst, maxBytes)
}

// ArchivePathsToTarGz streams a gzipped tar archive of the selected relative paths in root
// (or the entire root when includePaths is empty) to dst using os.Root.
func ArchivePathsToTarGz(root *os.Root, includePaths []string, dst io.Writer, maxBytes int64) (int64, string, error) {
	return ArchivePathsToTarGzWithProgress(root, includePaths, dst, maxBytes, nil)
}

// ArchivePathsToTarGzWithProgress streams a gzipped tar archive and invokes onProgress as entries are archived.
func ArchivePathsToTarGzWithProgress(root *os.Root, includePaths []string, dst io.Writer, maxBytes int64, onProgress func(writtenBytes int64, entries int)) (int64, string, error) {
	hasher := sha256.New()
	var entries int
	counter := &quotaCountingWriter{
		w:        io.MultiWriter(dst, hasher),
		maxBytes: maxBytes,
		onWrite: func(written int64) {
			if onProgress != nil {
				onProgress(written, entries)
			}
		},
	}
	gz := gzip.NewWriter(counter)
	tw := tar.NewWriter(gz)
	walkErr := walkArchiveTargets(root, tw, includePaths, func() {
		entries++
		if onProgress != nil {
			onProgress(counter.written, entries)
		}
	})
	if walkErr != nil {
		_ = tw.Close()
		_ = gz.Close()
		return counter.written, "", walkErr
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return counter.written, "", err
	}
	if err := gz.Close(); err != nil {
		return counter.written, "", err
	}
	if onProgress != nil {
		onProgress(counter.written, entries)
	}
	return counter.written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func walkArchiveTargets(root *os.Root, tw *tar.Writer, includePaths []string, onEntry func()) error {
	roots := normalizeArchiveRoots(includePaths)
	seen := make(map[string]bool)
	for _, start := range roots {
		if start != "." {
			if _, _, err := StatInRoot(root, start, false); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
		}
		err := fs.WalkDir(root.FS(), start, func(relPath string, d fs.DirEntry, err error) error {
			if seen[relPath] {
				return nil
			}
			seen[relPath] = true
			if err := archiveWalkEntry(root, tw, relPath, d, err); err != nil {
				return err
			}
			if onEntry != nil {
				onEntry()
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizeArchiveRoots(includePaths []string) []string {
	if len(includePaths) == 0 {
		return []string{"."}
	}
	var out []string
	seen := make(map[string]bool, len(includePaths))
	for _, raw := range includePaths {
		rel, err := metav1.NormalizeRelativeFilePath(raw, true)
		if err != nil {
			continue
		}
		if rel == "." {
			return []string{"."}
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	if len(out) == 0 {
		return []string{"."}
	}
	return out
}

func archiveWalkEntry(root *os.Root, tw *tar.Writer, relPath string, d fs.DirEntry, err error) error {
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if relPath == "." {
		return nil
	}
	if IsReservedMetadataPath(relPath) {
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
		return nil
	}
	if d.IsDir() {
		info, infoErr := d.Info()
		if infoErr != nil {
			if os.IsNotExist(infoErr) {
				return nil
			}
			return infoErr
		}
		hdr, hdrErr := tar.FileInfoHeader(info, "")
		if hdrErr != nil {
			return hdrErr
		}
		hdr.Name = strings.TrimSuffix(filepath.ToSlash(relPath), "/") + "/"
		return tw.WriteHeader(hdr)
	}
	return archiveRegularFile(root, tw, relPath)
}

func archiveRegularFile(root *os.Root, tw *tar.Writer, relPath string) error {
	f, info, _, err := OpenReadInRoot(root, relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		_ = f.Close()
		return err
	}
	hdr.Name = filepath.ToSlash(relPath)
	if err := tw.WriteHeader(hdr); err != nil {
		_ = f.Close()
		return err
	}
	written, copyErr := io.Copy(tw, io.LimitReader(f, hdr.Size))
	if copyErr == nil && written < hdr.Size {
		_, copyErr = io.CopyN(tw, zeroReader{}, hdr.Size-written)
	}
	return errors.Join(copyErr, f.Close())
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// ExtractTarGzToRoot validates and stages the complete archive before changing existing data.
func ExtractTarGzToRoot(root *os.Root, src io.Reader, maxBytes int64) error {
	return ExtractTarGzToRootContext(context.Background(), root, src, maxBytes)
}

type quotaCountingReader struct {
	r        io.Reader
	read     int64
	maxBytes int64
}

func (r *quotaCountingReader) Read(p []byte) (int, error) {
	if r.maxBytes > 0 {
		remaining := r.maxBytes - r.read
		if remaining < 0 {
			return 0, ErrFileQuotaExceeded
		}
		if remaining < int64(len(p)) {
			p = p[:remaining+1]
		}
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	if r.maxBytes > 0 && r.read > r.maxBytes {
		return n, ErrFileQuotaExceeded
	}
	return n, err
}

// ExtractTarGzToRootContext validates, stages, and atomically promotes a gzipped tar archive into root.
func ExtractTarGzToRootContext(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64) error {
	return ExtractTarGzToRootWithProgress(ctx, root, src, maxBytes, nil)
}

// ExtractTarGzToRootWithProgress validates, stages, and promotes a tar.gz archive while reporting progress.
func ExtractTarGzToRootWithProgress(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64, onProgress func(extractedBytes int64, entries int)) error {
	if src == nil {
		return errors.New("backup stream is required")
	}
	staged, err := stageTarGzArchiveToRoot(ctx, root, src, maxBytes, RestoreOptions{
		SourceSubpath: ".",
		TargetSubpath: ".",
		OnProgress:    onProgress,
	})
	if err != nil {
		return err
	}
	defer func() { _ = staged.Close() }()
	return staged.Promote(ctx)
}

type promotedFile struct {
	rel string
	old bool
	new bool
}

func promoteRestoredFiles(ctx context.Context, root *os.Root, stage string, dirs, files map[string]bool) (result error) {
	dirNames, fileNames, err := validateRestoreTargets(root, dirs, files)
	if err != nil {
		return err
	}
	var promoted []promotedFile
	var createdDirs []string
	defer func() {
		if result != nil {
			result = rollbackPromotedFiles(root, stage, promoted, createdDirs, result)
		}
	}()
	for _, rel := range dirNames {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := root.Lstat(filepath.FromSlash(rel)); os.IsNotExist(err) {
			if err := root.Mkdir(filepath.FromSlash(rel), 0o755); err != nil {
				return err
			}
			createdDirs = append(createdDirs, rel)
		}
	}
	for _, rel := range fileNames {
		if err := promoteSingleFile(ctx, root, stage, rel, &promoted); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func validateRestoreTargets(root *os.Root, dirs, files map[string]bool) ([]string, []string, error) {
	dirNames := make([]string, 0, len(dirs))
	for rel := range dirs {
		dirNames = append(dirNames, rel)
	}
	sort.Slice(dirNames, func(i, j int) bool {
		if strings.Count(dirNames[i], "/") != strings.Count(dirNames[j], "/") {
			return strings.Count(dirNames[i], "/") < strings.Count(dirNames[j], "/")
		}
		return dirNames[i] < dirNames[j]
	})
	fileNames := make([]string, 0, len(files))
	for rel := range files {
		fileNames = append(fileNames, rel)
	}
	sort.Strings(fileNames)
	for _, rel := range dirNames {
		info, err := root.Lstat(filepath.FromSlash(rel))
		if err == nil && !info.IsDir() {
			return nil, nil, fmt.Errorf("restore directory %q conflicts with existing data", rel)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, err
		}
	}
	for _, rel := range fileNames {
		if err := validateRegularTarget(root, rel, true); err != nil {
			return nil, nil, err
		}
	}
	return dirNames, fileNames, nil
}

func promoteSingleFile(ctx context.Context, root *os.Root, stage, rel string, promoted *[]promotedFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRegularTarget(root, rel, true); err != nil {
		return err
	}
	p := promotedFile{rel: rel}
	backup := filepath.FromSlash(path.Join(stage, "previous", rel))
	if _, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
		if err := root.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
			return err
		}
		if err := root.Rename(filepath.FromSlash(rel), backup); err != nil {
			return err
		}
		p.old = true
	} else if !os.IsNotExist(err) {
		return err
	}
	*promoted = append(*promoted, p)
	if err := root.Rename(filepath.FromSlash(path.Join(stage, "incoming", rel)), filepath.FromSlash(rel)); err != nil {
		return err
	}
	(*promoted)[len(*promoted)-1].new = true
	return nil
}

func rollbackPromotedFiles(root *os.Root, stage string, promoted []promotedFile, createdDirs []string, cause error) error {
	var rollbackErr error
	for i := len(promoted) - 1; i >= 0; i-- {
		p := promoted[i]
		if p.new {
			rollbackErr = errors.Join(rollbackErr, root.Remove(filepath.FromSlash(p.rel)))
		}
		if p.old {
			rollbackErr = errors.Join(rollbackErr, root.Rename(filepath.FromSlash(path.Join(stage, "previous", p.rel)), filepath.FromSlash(p.rel)))
		}
	}
	for i := len(createdDirs) - 1; i >= 0; i-- {
		rollbackErr = errors.Join(rollbackErr, root.Remove(filepath.FromSlash(createdDirs[i])))
	}
	if rollbackErr != nil {
		return errors.Join(cause, ErrRestoreRecoveryRequired, rollbackErr)
	}
	return cause
}
