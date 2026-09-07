package modruntime

import (
	"context"
	"fmt"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

// Inspect extracts optional metadata without interpreting any game format.
// Providers without an inspector return empty metadata. This is not a complete
// package safety or integrity validation.
func (s *Service) Inspect(ctx context.Context, key domain.ProviderKey, path string) (domain.ModMetadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.ModMetadata{}, err
	}
	item, ok := s.providers.Get(key)
	if !ok {
		return domain.ModMetadata{}, fmt.Errorf("unknown provider: %s", key)
	}
	inspector, ok := item.(provider.ModInspector)
	if !ok {
		return domain.ModMetadata{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	defer file.Close()
	return inspector.InspectMod(contextReader{ctx, file})
}
