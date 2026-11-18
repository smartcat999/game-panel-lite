package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// UpstreamInterface has methods to work with cluster-scoped Upstream resources.
type UpstreamInterface interface {
	Get(ctx context.Context, name string) (*registryv1.Upstream, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Upstream], error)
	Create(ctx context.Context, obj *registryv1.Upstream) (*registryv1.Upstream, error)
	Update(ctx context.Context, obj *registryv1.Upstream) (*registryv1.Upstream, error)
	Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Upstream, error)
	Delete(ctx context.Context, name string) error
}

type upstreams struct {
	rest rest.Interface
}

func newUpstreams(r rest.Interface) *upstreams {
	return &upstreams{rest: r}
}

func (c *upstreams) Get(ctx context.Context, name string) (*registryv1.Upstream, error) {
	var out registryv1.Upstream
	if err := c.rest.Get(ctx, "upstreams", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *upstreams) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Upstream], error) {
	var out metav1.List[registryv1.Upstream]
	if err := c.rest.List(ctx, "upstreams", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *upstreams) Create(ctx context.Context, obj *registryv1.Upstream) (*registryv1.Upstream, error) {
	var out registryv1.Upstream
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *upstreams) Update(ctx context.Context, obj *registryv1.Upstream) (*registryv1.Upstream, error) {
	var out registryv1.Upstream
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *upstreams) Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Upstream, error) {
	var out registryv1.Upstream
	if err := c.rest.Patch(ctx, "upstreams", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *upstreams) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "upstreams", "", name)
}
