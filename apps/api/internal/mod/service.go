package mod

import (
	"io"
	"os"
	"path/filepath"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

type Service struct {
	dataDir      string
	validateFile func(domain.ProviderKey, string) (string, error)
}

// NewService requires a provider-aware filename policy. The policy returns a
// validated basename or an error, and is shared by cache reads and writes.
func NewService(dataDir string, validateFile func(domain.ProviderKey, string) (string, error)) *Service {
	return &Service{dataDir: dataDir, validateFile: validateFile}
}

func (s *Service) Upload(instanceID string, providerKey domain.ProviderKey, fileName string, reader io.Reader) (string, int64, error) {
	safeName, err := s.validateFile(providerKey, fileName)
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

func (s *Service) Path(instanceID string, providerKey domain.ProviderKey, fileName string) (string, error) {
	safeName, err := s.validateFile(providerKey, fileName)
	if err != nil {
		return "", err
	}
	return safety.SafeJoin(s.dataDir, "mods", instanceID, safeName)
}
