package server

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	runtimepkg "github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type RuntimeAdapterClient struct {
	adapter runtimepkg.WorkloadAdapter
}

func NewRuntimeAdapterClient(adapter runtimepkg.WorkloadAdapter) *RuntimeAdapterClient {
	return &RuntimeAdapterClient{adapter: adapter}
}

func (c *RuntimeAdapterClient) Create(ctx context.Context, spec domain.WorkloadSpec) (string, error) {
	return c.adapter.CreateWorkload(ctx, spec)
}

func (c *RuntimeAdapterClient) Start(ctx context.Context, runtimeID string) error {
	return c.adapter.StartWorkload(ctx, runtimeID)
}

func (c *RuntimeAdapterClient) Stop(ctx context.Context, runtimeID string) error {
	return c.adapter.StopWorkload(ctx, runtimeID)
}

func (c *RuntimeAdapterClient) Remove(ctx context.Context, runtimeID string) error {
	return c.adapter.RemoveWorkload(ctx, runtimeID)
}

func (c *RuntimeAdapterClient) Inspect(ctx context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	return c.adapter.InspectWorkload(ctx, runtimeID)
}
