package world

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

func (s *Service) Import(instanceID string, fileName string, reader io.Reader) (string, int64, error) {
	safeName, err := safety.SafeFileName(fileName, ".wld")
	if err != nil {
		return "", 0, err
	}
	dir, err := safety.SafeJoin(s.dataDir, "worlds", instanceID)
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
	safeName, err := safety.SafeFileName(fileName, ".wld")
	if err != nil {
		return "", err
	}
	return safety.SafeJoin(s.dataDir, "worlds", instanceID, safeName)
}

func (s *Service) Duplicate(instanceID string, sourceName string, targetName string) (string, int64, error) {
	source, err := s.Path(instanceID, sourceName)
	if err != nil {
		return "", 0, err
	}
	safeTarget, err := safety.SafeFileName(targetName, ".wld")
	if err != nil {
		return "", 0, err
	}
	targetDir, err := safety.SafeJoin(s.dataDir, "worlds", instanceID)
	if err != nil {
		return "", 0, err
	}
	target := filepath.Join(targetDir, safeTarget)
	in, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	size, err := io.Copy(out, in)
	return target, size, err
}
