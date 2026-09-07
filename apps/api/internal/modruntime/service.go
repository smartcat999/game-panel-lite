// Package modruntime materializes provider-owned mod manifests.
package modruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

type Registry interface {
	Get(domain.ProviderKey) (provider.GameProvider, bool)
}
type Store interface {
	ListMods(context.Context, string) ([]domain.ModFile, error)
}
type Service struct {
	providers Registry
	store     Store
}

func NewService(providers Registry, store Store) *Service {
	return &Service{providers: providers, store: store}
}

func (s *Service) Support(key domain.ProviderKey) domain.ModSupport {
	item, ok := s.providers.Get(key)
	if !ok || !item.Capabilities().Mods {
		return domain.ModSupport{}
	}
	support, ok := item.(provider.ModSupportProvider)
	if !ok {
		return domain.ModSupport{}
	}
	return support.ModSupport()
}

func (s *Service) Paths(key domain.ProviderKey, filename string) []string {
	item, ok := s.providers.Get(key)
	if !ok {
		return nil
	}
	layout, ok := item.(provider.ModFilesProvider)
	if !ok {
		return nil
	}
	paths := []string{}
	for _, path := range layout.RuntimeModFiles(filename) {
		clean := filepath.Clean(path)
		if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		paths = append(paths, clean)
	}
	return paths
}

// Sync writes only manifests returned by the provider, inside the instance root.
// Callers serialize lifecycle and mod mutations for the instance.
func (s *Service) Sync(ctx context.Context, server domain.GameServer) error {
	item, ok := s.providers.Get(server.ProviderKey)
	if !ok {
		return fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	if !item.Capabilities().Mods {
		return nil
	}
	manifest, ok := item.(provider.ModManifestProvider)
	if !ok {
		return nil
	}
	mods, err := s.store.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	files, err := manifest.ModManifest(mods)
	if err != nil {
		return err
	}
	if strings.TrimSpace(server.Spec.Runtime.DataDir) == "" {
		return fmt.Errorf("server data dir is empty")
	}
	if err := os.MkdirAll(server.Spec.Runtime.DataDir, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(server.Spec.Runtime.DataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	for path, content := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeManifest(root, path, content); err != nil {
			return err
		}
	}
	return nil
}

func writeManifest(root *os.Root, path, content string) error {
	if err := root.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	if err := root.Chmod(filepath.Dir(path), 0777); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), ".manifest-"+uuid.NewString()+".tmp")
	file, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err := file.Chmod(0666); err != nil {
		_ = file.Close()
		return err
	}
	_, err = file.WriteString(content)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return root.Rename(tmp, path)
}
