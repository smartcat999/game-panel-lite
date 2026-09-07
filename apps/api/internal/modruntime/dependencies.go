package modruntime

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
)

// ResolveDependencies traverses a provider-scoped dependency graph. ensure must
// resolve a name within the caller's server/provider and report newly assigned
// records. Calls are sequential; on failure prior assignments are not rolled back.
func ResolveDependencies(ctx context.Context, roots []domain.ModFile, ensure func(context.Context, string) (domain.ModFile, bool, error)) ([]domain.ModFile, error) {
	added := make([]domain.ModFile, 0)
	queue := append([]domain.ModFile(nil), roots...)
	visited := make(map[string]bool)
	resolved := make(map[string]bool)
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := queue[0]
		queue = queue[1:]
		key := modcatalog.Identity(item)
		if key != "" {
			if visited[key] {
				continue
			}
			visited[key] = true
		}
		for _, name := range modcatalog.Dependencies(item) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if resolved[name] || visited[name] {
				continue
			}
			dependency, created, err := ensure(ctx, name)
			if err != nil {
				return nil, err
			}
			resolved[name] = true
			if created {
				added = append(added, dependency)
			}
			queue = append(queue, dependency)
		}
	}
	return added, nil
}
