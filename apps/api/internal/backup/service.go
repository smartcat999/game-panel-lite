package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

type Service struct {
	dataDir string
}

func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

func (s *Service) Create(instanceID string, sourceDir string) (string, int64, error) {
	return s.create(instanceID, sourceDir, sourceDir)
}

// CreateSubtree archives only the requested subtree while preserving its path
// relative to rootDir. This keeps save-only backups small and still allows the
// regular Restore method to put every file back in its original location.
func (s *Service) CreateSubtree(instanceID string, rootDir string, relativeSubtree string) (string, int64, error) {
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
	return s.create(instanceID, cleanRoot, sourceDir)
}

func (s *Service) create(instanceID string, archiveRoot string, sourceDir string) (string, int64, error) {
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
	walkErr := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup source contains a symbolic link")
		}
		rel, err := filepath.Rel(archiveRoot, path)
		if err != nil {
			return err
		}
		writer, err := zipper.Create(rel)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(writer, in)
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
	backupPath, err := s.Path(instanceID, fileName)
	if err != nil {
		return err
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	cleanTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
		return err
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(file.Name, "..") || filepath.IsAbs(file.Name) {
			return fmt.Errorf("backup contains unsafe path")
		}
		target, err := filepath.Abs(filepath.Join(cleanTarget, file.Name))
		if err != nil {
			return err
		}
		if target != cleanTarget && !strings.HasPrefix(target, cleanTarget+string(filepath.Separator)) {
			return fmt.Errorf("backup contains unsafe path")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
	}
	return nil
}
