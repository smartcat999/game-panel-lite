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
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	size, err := io.Copy(out, reader)
	return target, size, err
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
