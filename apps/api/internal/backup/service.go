package backup

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

type Service struct {
	dataDir  string
	metadata *Metadata
}

func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

func (s *Service) Create(instanceID string, sourceDir string) (string, int64, error) {
	return s.CreateContext(context.Background(), instanceID, sourceDir)
}

// CreateContext archives a caller-stabilized directory. Cancellation bounds
// cooperative file copying; the caller still owns execution authority and locks.
func (s *Service) CreateContext(ctx context.Context, instanceID, sourceDir string) (string, int64, error) {
	return s.create(ctx, instanceID, sourceDir, sourceDir)
}

// CreateSubtree archives only the requested subtree while preserving its path
// relative to rootDir. This keeps save-only backups small and still allows the
// regular Restore method to put every file back in its original location.
func (s *Service) CreateSubtree(instanceID string, rootDir string, relativeSubtree string) (string, int64, error) {
	return s.CreateSubtreeContext(context.Background(), instanceID, rootDir, relativeSubtree)
}

func (s *Service) CreateSubtreeContext(ctx context.Context, instanceID, rootDir, relativeSubtree string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	cleanRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", 0, err
	}
	if filepath.IsAbs(relativeSubtree) {
		return "", 0, fmt.Errorf("backup subtree must be relative")
	}
	cleanRelative := filepath.Clean(relativeSubtree)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", 0, fmt.Errorf("invalid backup subtree")
	}
	sourceDir, err := filepath.Abs(filepath.Join(cleanRoot, cleanRelative))
	if err != nil {
		return "", 0, err
	}
	if sourceDir != cleanRoot && !strings.HasPrefix(sourceDir, cleanRoot+string(filepath.Separator)) {
		return "", 0, fmt.Errorf("backup subtree escapes data directory")
	}
	realRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", 0, fmt.Errorf("resolve backup data directory: %w", err)
	}
	realSource, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return "", 0, fmt.Errorf("resolve backup subtree: %w", err)
	}
	realRelative, err := filepath.Rel(realRoot, realSource)
	if err != nil || realRelative == ".." || filepath.IsAbs(realRelative) || strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
		return "", 0, fmt.Errorf("backup subtree resolves outside data directory")
	}
	return s.create(ctx, instanceID, cleanRoot, sourceDir)
}

func (s *Service) create(ctx context.Context, instanceID string, archiveRoot string, sourceDir string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	root, err := os.OpenRoot(archiveRoot)
	if err != nil {
		return "", 0, err
	}
	defer root.Close()
	dir, err := safety.SafeJoin(s.dataDir, "backups", instanceID)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	target := filepath.Join(dir, "backup-"+time.Now().UTC().Format("20060102-150405.000000000")+".zip")
	out, err := os.Create(target)
	if err != nil {
		return "", 0, err
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.Remove(target)
		}
	}()
	zipper := zip.NewWriter(out)
	if s.metadata != nil {
		if err := writeMetadata(zipper, *s.metadata); err != nil {
			_ = zipper.Close()
			_ = out.Close()
			return "", 0, err
		}
	}
	walkErr := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup source contains a symbolic link")
		}
		rel, err := filepath.Rel(archiveRoot, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == metadataPath {
			return fmt.Errorf("backup source uses reserved metadata filename")
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("backup source contains a non-regular file")
		}
		writer, err := zipper.Create(rel)
		if err != nil {
			return err
		}
		in, err := root.Open(rel)
		if err != nil {
			return err
		}
		defer in.Close()
		info, err := in.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup source is not a regular file")
		}
		stop := context.AfterFunc(ctx, func() { _ = in.Close() })
		defer stop()
		_, err = io.Copy(writer, archiveContextReader{ctx: ctx, source: in})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	})
	closeErr := zipper.Close()
	fileErr := out.Close()
	if walkErr != nil {
		return "", 0, walkErr
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	if fileErr != nil {
		return "", 0, fileErr
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", 0, err
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	completed = true
	return target, info.Size(), nil
}

func (s *Service) Path(instanceID string, fileName string) (string, error) {
	safeName, err := safety.SafeFileName(fileName, ".zip")
	if err != nil {
		return "", err
	}
	return safety.SafeJoin(s.dataDir, "backups", instanceID, safeName)
}

func (s *Service) Restore(instanceID string, fileName string, targetDir string) error {
	return s.RestoreChecked(instanceID, fileName, targetDir, RestoreHooks{})
}

// RestoreChecked validates archive metadata before creating or modifying target
// files. Extraction is staged and rolls back on publication or commit failure.
// Commit must only return success after its own persistence succeeds.
func (s *Service) RestoreChecked(instanceID string, fileName string, targetDir string, hooks RestoreHooks) error {
	backupPath, err := s.Path(instanceID, fileName)
	if err != nil {
		return err
	}
	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	return RestoreArchiveChecked(file, info.Size(), targetDir, hooks)
}

// RestoreArchiveChecked restores a caller-owned, seekable archive, including a
// verified object-store download staged on local disk. The caller must authorize
// the backup, verify its expected size and digest, stop the game, and serialize
// mutations before calling. Archive metadata is compatibility, not authorization.
// This function does not close source. It retains the local restore rollback
// behavior; process-crash recovery and extraction quotas require orchestration.
func RestoreArchiveChecked(source io.ReaderAt, size int64, targetDir string, hooks RestoreHooks) error {
	if source == nil || size < 0 {
		return fmt.Errorf("invalid backup archive source")
	}
	reader, err := zip.NewReader(source, size)
	if err != nil {
		return err
	}
	metadata, err := readMetadata(reader.File)
	if err != nil {
		return err
	}
	if hooks.Validate != nil {
		if err := hooks.Validate(metadata); err != nil {
			return err
		}
	}
	cleanTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(cleanTarget)
	if err != nil {
		return err
	}
	defer root.Close()
	return restoreFiles(root, reader.File, hooks.Commit)
}

// Keep io.Copy on its bounded read loop rather than an uncancellable WriterTo.
type archiveContextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r archiveContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
