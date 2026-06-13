package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
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
	target := filepath.Join(dir, "backup-"+time.Now().UTC().Format("20060102-150405")+".zip")
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
