package mod

import (
	"io"
	"os"
	"path/filepath"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

type Service struct {
	dataDir string
}

func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

func (s *Service) Upload(instanceID string, fileName string, reader io.Reader) (string, int64, error) {
	safeName, err := safeModFile(fileName)
	if err != nil {
		return "", 0, err
	}
	dir, err := safety.SafeJoin(s.dataDir, "mods", instanceID)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	target := filepath.Join(dir, safeName)
	out, err := os.CreateTemp(dir, "."+safeName+".*.tmp")
	if err != nil {
		return "", 0, err
	}
	tmpName := out.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	size, err := io.Copy(out, reader)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", 0, err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", 0, err
	}
	return target, size, nil
}

func (s *Service) Path(instanceID string, fileName string) (string, error) {
	safeName, err := safeModFile(fileName)
	if err != nil {
		return "", err
	}
	return safety.SafeJoin(s.dataDir, "mods", instanceID, safeName)
}

func safeModFile(fileName string) (string, error) {
	if fileName == "install.txt" || fileName == "enabled.json" {
		return safety.SafeFileName(fileName, ".txt", ".json")
	}
	return safety.SafeFileName(fileName, ".tmod")
}
