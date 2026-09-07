package modruntime

import (
	"fmt"
	"path/filepath"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

// UploadFileName validates user uploads using the registered provider capability.
func (s *Service) UploadFileName(key domain.ProviderKey, name string) (string, error) {
	extensions := s.Support(key).UploadExtensions
	if len(extensions) == 0 {
		return "", fmt.Errorf("uploaded mods are not supported for this provider")
	}
	return safety.SafeFileName(name, extensions...)
}

// StoredFileName additionally accepts exact provider-owned auxiliary cache names.
func (s *Service) StoredFileName(key domain.ProviderKey, name string) (string, error) {
	for _, allowed := range s.Support(key).CacheFiles {
		if name == allowed {
			return safety.SafeFileName(name, filepath.Ext(allowed))
		}
	}
	return s.UploadFileName(key, name)
}
