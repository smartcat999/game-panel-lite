package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// RepositoryInterface has methods to work with cluster-scoped Repository resources.
type RepositoryInterface interface {
	Get(ctx context.Context, name string) (*registryv1.Repository, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Repository], error)
	Create(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error)
	Update(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error)
	UpdateStatus(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error)
	Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Repository, error)
	Delete(ctx context.Context, name string) error
}

type repositories struct {
	rest rest.Interface
}

func newRepositories(r rest.Interface) *repositories {
	return &repositories{rest: r}
}

func (c *repositories) Get(ctx context.Context, name string) (*registryv1.Repository, error) {
	var out registryv1.Repository
	if err := c.rest.Get(ctx, "repositories", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Repository], error) {
	var out metav1.List[registryv1.Repository]
	if err := c.rest.List(ctx, "repositories", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) Create(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error) {
	var out registryv1.Repository
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) Update(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error) {
	var out registryv1.Repository
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) UpdateStatus(ctx context.Context, obj *registryv1.Repository) (*registryv1.Repository, error) {
	var out registryv1.Repository
	if err := c.rest.UpdateStatus(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Repository, error) {
	var out registryv1.Repository
	if err := c.rest.Patch(ctx, "repositories", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *repositories) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "repositories", "", name)
}
