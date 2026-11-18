package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// PackageInterface has methods to work with cluster-scoped Package resources (<repository>.<package>).
type PackageInterface interface {
	Get(ctx context.Context, name string) (*registryv1.Package, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Package], error)
	Create(ctx context.Context, obj *registryv1.Package) (*registryv1.Package, error)
	Update(ctx context.Context, obj *registryv1.Package) (*registryv1.Package, error)
	Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Package, error)
	Delete(ctx context.Context, name string) error
}

type packages struct {
	rest rest.Interface
}

func newPackages(r rest.Interface) *packages {
	return &packages{rest: r}
}

func (c *packages) Get(ctx context.Context, name string) (*registryv1.Package, error) {
	var out registryv1.Package
	if err := c.rest.Get(ctx, "packages", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *packages) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Package], error) {
	var out metav1.List[registryv1.Package]
	if err := c.rest.List(ctx, "packages", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *packages) Create(ctx context.Context, obj *registryv1.Package) (*registryv1.Package, error) {
	var out registryv1.Package
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *packages) Update(ctx context.Context, obj *registryv1.Package) (*registryv1.Package, error) {
	var out registryv1.Package
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *packages) Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Package, error) {
	var out registryv1.Package
	if err := c.rest.Patch(ctx, "packages", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *packages) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "packages", "", name)
}
