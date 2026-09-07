package modruntime

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

// Inspect extracts optional metadata without interpreting any game format.
// Providers without an inspector return empty metadata. This is not a complete
// package safety or integrity validation.
func (s *Service) Inspect(ctx context.Context, key domain.ProviderKey, path string) (domain.ModMetadata, error) {
	inspector, err := s.modInspector(ctx, key)
	if err != nil || inspector == nil {
		return domain.ModMetadata{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	defer file.Close()
	return inspector.InspectMod(contextReader{ctx, file})
}

// InspectReader consumes an already confined file handle or upload stream.
func (s *Service) InspectReader(ctx context.Context, key domain.ProviderKey, reader io.Reader) (domain.ModMetadata, error) {
	inspector, err := s.modInspector(ctx, key)
	if err != nil || inspector == nil {
		return domain.ModMetadata{}, err
	}
	return inspector.InspectMod(contextReader{ctx, reader})
}
func (s *Service) modInspector(ctx context.Context, key domain.ProviderKey) (provider.ModInspector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, ok := s.providers.Get(key)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", key)
	}
	inspector, _ := item.(provider.ModInspector)
	return inspector, nil
}
