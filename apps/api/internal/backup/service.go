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
	zipper := zip.NewWriter(out)
	walkErr := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
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
