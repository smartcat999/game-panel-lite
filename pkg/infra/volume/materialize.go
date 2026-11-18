package volume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ErrReflinkUnsupported indicates that filesystem CoW cloning failed with an unsupported/cross-device error while RequireReflink was set.
var ErrReflinkUnsupported = errors.New("filesystem CoW reflink is unsupported or cross-device")

// MaterializeOptions controls how cached artifacts are materialized into an *os.Root workspace.
type MaterializeOptions struct {
	ForceReplace   bool
	RequireReflink bool
	DisableReflink bool
	SkipRel        func(string) bool
}

type reflinkDevPair struct {
	srcDev uint64
	dstDev uint64
}

var reflinkUnsupportedPairs sync.Map // map[reflinkDevPair]bool

// DirDeviceID returns the underlying filesystem device ID for dir if available on this OS.
func DirDeviceID(dir string) (uint64, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, false
	}
	return fileDeviceID(info)
}

// MarkDeviceReflinkUnsupported records that intra-device CoW reflink cloning is unsupported on devID.
func MarkDeviceReflinkUnsupported(devID uint64) {
	if devID == 0 {
		return
	}
	reflinkUnsupportedPairs.Store(reflinkDevPair{srcDev: devID, dstDev: devID}, true)
}

func isFilePairReflinkUnsupported(srcInfo, dstInfo os.FileInfo) bool {
	srcDev, okSrc := fileDeviceID(srcInfo)
	dstDev, okDst := fileDeviceID(dstInfo)
	if !okSrc || !okDst {
		return false
	}
	_, disabled := reflinkUnsupportedPairs.Load(reflinkDevPair{srcDev: srcDev, dstDev: dstDev})
	return disabled
}

func markFilePairReflinkUnsupported(srcInfo, dstInfo os.FileInfo) {
	srcDev, okSrc := fileDeviceID(srcInfo)
	dstDev, okDst := fileDeviceID(dstInfo)
	if !okSrc || !okDst {
		return
	}
	reflinkUnsupportedPairs.Store(reflinkDevPair{srcDev: srcDev, dstDev: dstDev}, true)
}

// ProbeDirReflink probes whether dir's underlying filesystem supports intra-device CoW reflink cloning.
func ProbeDirReflink(dir string) (uint64, bool, error) {
	cleanDir := filepath.Clean(strings.TrimSpace(dir))
	if cleanDir == "" || cleanDir == "." {
		return 0, false, errors.New("probe directory is required")
	}
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return 0, false, err
	}
	srcFile, err := os.CreateTemp(cleanDir, ".gp-reflink-src-*")
	if err != nil {
		return 0, false, err
	}
	srcPath := srcFile.Name()
	defer func() { _ = os.Remove(srcPath) }()
	if _, err := srcFile.Write([]byte("p")); err != nil {
		_ = srcFile.Close()
		return 0, false, err
	}
	srcInfo, statErr := srcFile.Stat()
	if err := errors.Join(statErr, srcFile.Close()); err != nil {
		return 0, false, err
	}
	devID, _ := fileDeviceID(srcInfo)
	root, err := os.OpenRoot(cleanDir)
	if err != nil {
		return devID, false, err
	}
	defer func() { _ = root.Close() }()
	tmpName, err := privateFileName(".gp-reflink-dst-")
	if err != nil {
		return devID, false, err
	}
	defer func() { _ = root.Remove(tmpName) }()
	cloned, err := tryCloneFileInRoot(srcPath, root, ".", tmpName, tmpName, 0o600)
	if err != nil {
		return devID, false, err
	}
	if !cloned {
		MarkDeviceReflinkUnsupported(devID)
	}
	return devID, cloned, nil
}

type requireReflinkKey struct{}

// WithRequireReflink attaches the fail-closed CoW reflink policy to ctx for workspace materialization.
func WithRequireReflink(ctx context.Context, require bool) context.Context {
	return context.WithValue(ctx, requireReflinkKey{}, require)
}

// RequireReflinkFromContext reports whether ctx requires kernel CoW reflink without copy fallback.
func RequireReflinkFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(requireReflinkKey{}).(bool)
	return v
}

// RequiresReflink reports whether the given RuntimeClass mandates XFS/CoW reflink isolation.
func RequiresReflink(rc *corev1.RuntimeClass) bool {
	if rc == nil || len(rc.Parameters) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(rc.Parameters["storageQuotaProvider"]), DefaultXFSQuotaProviderName) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(rc.Parameters["requireReflink"]), "true")
}

// IsImmutableRegularCacheFile reports whether info describes a read-only regular file without extra hardlinks.
func IsImmutableRegularCacheFile(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o222 == 0 && hasSingleHardlink(info)
}

// EnsureSafeDirInRoot creates rawDir inside root segment-by-segment, rejecting any symlink or non-directory component.
func EnsureSafeDirInRoot(root *os.Root, rawDir string) (string, error) {
	if root == nil {
		return "", errors.New("workspace root is required")
	}
	rel, err := metav1.NormalizeRelativeFilePath(rawDir, true)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == "" {
		return ".", nil
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return "", err
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		segPath := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		if err := ensureSingleSafeDirSegment(root, segPath); err != nil {
			return "", err
		}
	}
	return rel, nil
}

func ensureSingleSafeDirSegment(root *os.Root, segPath string) error {
	info, err := root.Lstat(segPath)
	if os.IsNotExist(err) {
		if mkErr := root.Mkdir(segPath, 0o755); mkErr != nil && !os.IsExist(mkErr) {
			return mkErr
		}
		info, err = root.Lstat(segPath)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("directory component %q must be a real directory without symlinks", segPath)
	}
	return nil
}

// WriteFileAtomicInRoot atomically writes data to rawPath inside root via an exclusive randomized temp file.
func WriteFileAtomicInRoot(root *os.Root, rawPath string, data []byte, perm os.FileMode) error {
	if root == nil {
		return errors.New("workspace root is required")
	}
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return err
	}
	parent := path.Dir(rel)
	if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
		return err
	}
	if info, err := root.Lstat(filepath.FromSlash(rel)); err == nil && info.IsDir() {
		return fmt.Errorf("cannot overwrite directory %q with regular file", rel)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeBytesViaRandomTempInRoot(root, parent, rel, data, normalizeWorkspacePerm(perm))
}

func normalizeWorkspacePerm(perm os.FileMode) os.FileMode {
	if perm == 0 {
		return 0o644
	}
	return perm & 0o777
}

func writeBytesViaRandomTempInRoot(root *os.Root, parent, rel string, data []byte, perm os.FileMode) error {
	tmpName, err := privateFileName(".gp-mat-")
	if err != nil {
		return err
	}
	tmpRel := filepath.FromSlash(path.Join(parent, tmpName))
	f, err := root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmpRel) }()

	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(perm)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, chmodErr, syncErr, closeErr); err != nil {
		return err
	}
	if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
		return err
	}
	return root.Rename(tmpRel, filepath.FromSlash(rel))
}

// WriteJSONAtomicInRoot encodes v as JSON and writes it atomically into root.
func WriteJSONAtomicInRoot(root *os.Root, rawPath string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return WriteFileAtomicInRoot(root, rawPath, raw, 0o644)
}

// ReadOptionalFileInRoot reads rawPath inside root when present, returning (nil, nil) if absent and rejecting symlinks.
func ReadOptionalFileInRoot(root *os.Root, rawPath string) ([]byte, error) {
	if root == nil {
		return nil, errors.New("workspace root is required")
	}
	rel, err := metav1.NormalizeRelativeFilePath(rawPath, false)
	if err != nil {
		return nil, err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return nil, err
	}
	ok, err := checkOptionalRegularPath(root, rel)
	if err != nil || !ok {
		return nil, err
	}
	f, err := root.OpenFile(filepath.FromSlash(rel), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to read non-regular file %q", rel)
	}
	return io.ReadAll(f)
}

func checkOptionalRegularPath(root *os.Root, rel string) (bool, error) {
	parts := strings.Split(rel, "/")
	for i := range parts {
		seg := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, err := root.Lstat(seg)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if i < len(parts)-1 && !info.IsDir() {
			return false, fmt.Errorf("parent %q must be a real directory without symlinks", seg)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return false, fmt.Errorf("refusing to read non-regular file %q", rel)
		}
	}
	return true, nil
}

// ListRegularFilesInRoot lists sorted regular file basenames inside rawDir without following symlinks.
func ListRegularFilesInRoot(root *os.Root, rawDir string) ([]string, error) {
	if root == nil {
		return nil, errors.New("workspace root is required")
	}
	rel, err := metav1.NormalizeRelativeFilePath(rawDir, true)
	if err != nil {
		return nil, err
	}
	if rel != "." {
		if err := rejectReservedMetadataPath(rel); err != nil {
			return nil, err
		}
		if ok, err := checkExistingDirChain(root, rel); err != nil || !ok {
			return nil, err
		}
	}
	entries, err := fs.ReadDir(root.FS(), rel)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && !IsReservedMetadataPath(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func checkExistingDirChain(root *os.Root, rel string) (bool, error) {
	parts := strings.Split(rel, "/")
	for i := range parts {
		seg := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, err := root.Lstat(seg)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() {
			return false, fmt.Errorf("directory component %q must be a real directory without symlinks", seg)
		}
	}
	return true, nil
}

// MaterializeFileInRoot creates an isolated CoW clone or exclusive atomic copy of srcPath at dstRel inside root.
func MaterializeFileInRoot(srcPath string, root *os.Root, dstRel string, opts MaterializeOptions) error {
	if root == nil {
		return errors.New("workspace root is required")
	}
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source artifact %q must be a regular file", srcPath)
	}
	rel, err := metav1.NormalizeRelativeFilePath(dstRel, false)
	if err != nil {
		return err
	}
	if err := rejectReservedMetadataPath(rel); err != nil {
		return err
	}
	return materializeCheckedFileInRoot(srcPath, srcInfo, root, rel, materializedFilePerm(srcInfo.Mode()), opts)
}

func materializedFilePerm(srcMode os.FileMode) os.FileMode {
	if srcMode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func materializeCheckedFileInRoot(srcPath string, srcInfo os.FileInfo, root *os.Root, rel string, perm os.FileMode, opts MaterializeOptions) error {
	parent := path.Dir(rel)
	if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
		return err
	}
	if info, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
		if info.IsDir() {
			return fmt.Errorf("cannot overwrite directory %q with regular file", rel)
		}
		if !opts.ForceReplace {
			if !info.Mode().IsRegular() || !hasSingleHardlink(info) || os.SameFile(srcInfo, info) {
				return fmt.Errorf("file %q must be regular without links", rel)
			}
			if info.Size() == srcInfo.Size() && info.Mode().Perm() == perm {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tmpName, err := privateFileName(".gp-cow-")
	if err != nil {
		return err
	}
	tmpRel := filepath.FromSlash(path.Join(parent, tmpName))
	defer func() { _ = root.Remove(tmpRel) }()
	cloned, err := tryCloneIfSupported(srcPath, srcInfo, root, parent, tmpName, tmpRel, perm, opts)
	if err != nil {
		return err
	}
	return finishMaterializeFile(srcPath, root, parent, rel, tmpRel, perm, cloned, opts.RequireReflink)
}

func tryCloneIfSupported(srcPath string, srcInfo os.FileInfo, root *os.Root, parent, tmpName, tmpRel string, perm os.FileMode, opts MaterializeOptions) (bool, error) {
	if opts.DisableReflink {
		return false, nil
	}
	var dstInfo os.FileInfo
	if !opts.RequireReflink {
		dstInfo, _ = root.Lstat(".")
		if isFilePairReflinkUnsupported(srcInfo, dstInfo) {
			return false, nil
		}
	}
	cloned, err := tryCloneFileInRoot(srcPath, root, parent, tmpName, tmpRel, perm)
	if err != nil {
		return false, err
	}
	if !cloned && !opts.RequireReflink {
		markFilePairReflinkUnsupported(srcInfo, dstInfo)
	}
	return cloned, nil
}

func finishMaterializeFile(srcPath string, root *os.Root, parent, rel, tmpRel string, perm os.FileMode, cloned, requireReflink bool) error {
	if !cloned {
		if requireReflink {
			return ErrReflinkUnsupported
		}
		if err := copyFileToTempInRoot(srcPath, root, tmpRel, perm); err != nil {
			return err
		}
	}
	if _, err := EnsureSafeDirInRoot(root, parent); err != nil {
		return err
	}
	return root.Rename(tmpRel, filepath.FromSlash(rel))
}

func copyFileToTempInRoot(srcPath string, root *os.Root, tmpRel string, perm os.FileMode) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	tmpFile, err := root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tmpFile, srcFile)
	chmodErr := tmpFile.Chmod(perm)
	closeErr := tmpFile.Close()
	if err := errors.Join(copyErr, chmodErr, closeErr); err != nil {
		_ = root.Remove(tmpRel)
		return err
	}
	return nil
}

func disableTreeReflinkIfKnownUnsupported(opts *MaterializeOptions, srcInfo, dstInfo os.FileInfo) {
	if opts.DisableReflink || opts.RequireReflink || srcInfo == nil || dstInfo == nil {
		return
	}
	if isFilePairReflinkUnsupported(srcInfo, dstInfo) {
		opts.DisableReflink = true
	}
}

// MaterializeTreeInRoot materializes regular files under srcDir into dstSubdir inside root and returns relative slash paths.
func MaterializeTreeInRoot(srcDir string, root *os.Root, dstSubdir string, opts MaterializeOptions) ([]string, error) {
	baseSub, err := resolveMaterializeTreeSubdir(root, dstSubdir)
	if err != nil {
		return nil, err
	}
	srcDirInfo, _ := os.Stat(srcDir)
	dstRootInfo, _ := root.Lstat(".")
	disableTreeReflinkIfKnownUnsupported(&opts, srcDirInfo, dstRootInfo)

	var materialized []string
	err = filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, corev1.ReservedWorkspacePrefix) || strings.HasSuffix(name, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, srcPath)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if opts.SkipRel != nil && opts.SkipRel(slashRel) {
			return nil
		}
		dstRel := slashRel
		if baseSub != "" {
			dstRel = path.Join(baseSub, slashRel)
		}
		if err := MaterializeFileInRoot(srcPath, root, dstRel, opts); err != nil {
			return err
		}
		disableTreeReflinkIfKnownUnsupported(&opts, srcDirInfo, dstRootInfo)
		materialized = append(materialized, slashRel)
		return nil
	})
	return materialized, err
}

func resolveMaterializeTreeSubdir(root *os.Root, dstSubdir string) (string, error) {
	trimmed := strings.TrimSpace(dstSubdir)
	if trimmed == "" || trimmed == "." {
		return "", nil
	}
	norm, err := EnsureSafeDirInRoot(root, dstSubdir)
	if err != nil {
		return "", err
	}
	if norm == "." {
		return "", nil
	}
	return norm, nil
}

// ResolveInstanceStateID resolves the host-side state directory identifier outside container bind mounts.
func ResolveInstanceStateID(workDir, hostDir, uid string) (string, error) {
	if trimmedUID := strings.TrimSpace(uid); trimmedUID != "" {
		if err := metav1.ValidateSingleSegmentName("instance uid", trimmedUID); err != nil {
			return "", err
		}
		return trimmedUID, nil
	}
	if cleanWork := filepath.Clean(strings.TrimSpace(workDir)); cleanWork != "" && cleanWork != "." {
		if rel, err := filepath.Rel(filepath.Join(cleanWork, "instances"), filepath.Clean(hostDir)); err == nil &&
			rel != "." && !strings.HasPrefix(rel, "..") && !strings.ContainsRune(rel, filepath.Separator) {
			if err := metav1.ValidateSingleSegmentName("instance workdir segment", rel); err != nil {
				return "", err
			}
			return rel, nil
		}
	}
	sum := sha256.Sum256([]byte(filepath.Clean(hostDir)))
	return "dir-" + hex.EncodeToString(sum[:12]), nil
}

// OpenInstanceStateRoot opens <workDir>/state/instances/<stateID> outside container-visible hostDir mounts.
func OpenInstanceStateRoot(workDir, stateID string, create bool) (*os.Root, error) {
	trimmed := strings.TrimSpace(stateID)
	if err := metav1.ValidateSingleSegmentName("instance state id", trimmed); err != nil {
		return nil, err
	}
	cleanBase := filepath.Clean(strings.TrimSpace(workDir))
	if cleanBase == "" || cleanBase == "." {
		return nil, errors.New("state base workDir is required")
	}
	if !create {
		if _, err := os.Stat(cleanBase); os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
	} else if err := os.MkdirAll(cleanBase, 0o755); err != nil {
		return nil, err
	}
	baseRoot, err := os.OpenRoot(cleanBase)
	if err != nil {
		return nil, err
	}
	defer func() { _ = baseRoot.Close() }()
	relDir := path.Join("state", "instances", trimmed)
	if create {
		if _, err := EnsureSafeDirInRoot(baseRoot, relDir); err != nil {
			return nil, err
		}
	}
	return baseRoot.OpenRoot(filepath.FromSlash(relDir))
}

// RemoveInstanceState removes host-side materialization state for stateID under workDir.
func RemoveInstanceState(workDir, stateID string) error {
	trimmed := strings.TrimSpace(stateID)
	if err := metav1.ValidateSingleSegmentName("instance state id", trimmed); err != nil {
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
	err = baseRoot.RemoveAll(filepath.Join("state", "instances", trimmed))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
