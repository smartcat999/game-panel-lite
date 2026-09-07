// Package gameconfig coordinates provider-owned configuration operations.
package gameconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

type Registry interface {
	Get(domain.ProviderKey) (provider.GameProvider, bool)
	List() []provider.GameProvider
}

type Store interface {
	SaveGameServer(context.Context, *domain.GameServer) error
}

type Service struct {
	providers       Registry
	store           Store
	defaultProvider domain.ProviderKey
}

func NewService(providers Registry, store Store, defaultProvider domain.ProviderKey) *Service {
	return &Service{providers: providers, store: store, defaultProvider: defaultProvider}
}

func (s *Service) Presets() []domain.ProviderPreset {
	result := []domain.ProviderPreset{}
	defaultProvider, ok := s.providers.Get(s.defaultProvider)
	if !ok {
		return result
	}
	providers := append([]provider.GameProvider(nil), s.providers.List()...)
	sort.Slice(providers, func(i, j int) bool {
		left, right := providers[i].CatalogMetadata().Priority, providers[j].CatalogMetadata().Priority
		if left != right {
			return left < right
		}
		return providers[i].Key() < providers[j].Key()
	})
	for _, item := range providers {
		if item.GameKey() != defaultProvider.GameKey() {
			continue
		}
		if presets, ok := item.(provider.PresetProvider); ok {
			result = append(result, presets.Presets()...)
		}
	}
	return result
}

func (s *Service) Preview(key domain.ProviderKey, version int, raw json.RawMessage) (map[string]string, error) {
	if key == "" {
		key = s.defaultProvider
	}
	item, ok := s.providers.Get(key)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", key)
	}
	if err := provider.CheckConfigVersion(item, version); err != nil {
		return nil, err
	}
	preview, ok := item.(provider.ConfigPreviewProvider)
	if !ok {
		return nil, fmt.Errorf("%w: config preview for %s", provider.ErrUnsupported, key)
	}
	return preview.PreviewConfig(raw)
}

// Restore applies the provider's persisted configuration after restoring files.
// Missing configuration files are allowed. Failed parsing or persistence leaves
// the caller's resource unchanged. The caller serializes mutations per server.
func (s *Service) Restore(ctx context.Context, server *domain.GameServer) error {
	item, ok := s.providers.Get(server.ProviderKey)
	if !ok {
		return fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	if err := provider.CheckConfigVersion(item, server.Spec.ConfigVersion); err != nil {
		return err
	}
	restore, ok := item.(provider.ConfigRestoreProvider)
	if !ok {
		return nil
	}
	if strings.TrimSpace(server.Spec.Runtime.DataDir) == "" {
		return fmt.Errorf("server data directory is not ready")
	}
	root, err := os.OpenRoot(server.Spec.Runtime.DataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.Open(restore.ConfigRestoreFile())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	const maxConfigBytes = 1 << 20
	content, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return err
	}
	if len(content) > maxConfigBytes {
		return fmt.Errorf("restored configuration exceeds 1 MiB")
	}
	config, port, err := restore.RestoreConfig(server.Spec.Config, content)
	if err != nil {
		return err
	}
	updated := *server
	updated.Spec.Config = config
	updated.Spec.Network.Port = port
	updated.Spec.Generation++
	if updated.Spec.Generation <= 0 {
		updated.Spec.Generation = 1
	}
	updated.Status.Phase = domain.PhasePending
	updated.UpdatedAt = time.Now()
	if err := s.store.SaveGameServer(ctx, &updated); err != nil {
		return err
	}
	*server = updated
	return nil
}

// Check verifies a stored configuration before callers perform mutations such
// as extracting a backup. It does not inspect the backup's own format version.
func (s *Service) Check(server domain.GameServer) error {
	item, ok := s.providers.Get(server.ProviderKey)
	if !ok {
		return fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	return provider.CheckConfigVersion(item, server.Spec.ConfigVersion)
}

// CheckBackup validates immutable source metadata as well as the target before
// extraction. Legacy records without source metadata retain format-1 semantics.
func (s *Service) CheckBackup(server domain.GameServer, backup domain.Backup) error {
	if err := s.Check(server); err != nil {
		return err
	}
	if backup.ProviderKey != "" && backup.ProviderKey != server.ProviderKey {
		return fmt.Errorf("backup provider does not match target server")
	}
	item, _ := s.providers.Get(server.ProviderKey)
	return provider.CheckConfigVersion(item, backup.ConfigVersion)
}
