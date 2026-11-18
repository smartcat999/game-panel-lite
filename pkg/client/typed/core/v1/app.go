package v1

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// AppInterface has methods to work with cluster-scoped App resources in the marketplace.
type AppInterface interface {
	Get(ctx context.Context, name string) (*corev1.App, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.App], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, app *corev1.App) (*corev1.App, error)
	Update(ctx context.Context, app *corev1.App) (*corev1.App, error)
	Patch(ctx context.Context, name string, patchData []byte) (*corev1.App, error)
	Delete(ctx context.Context, name string) error
}

type apps struct {
	rest rest.Interface
}

func newApps(r rest.Interface) *apps {
	return &apps{rest: r}
}

func (c *apps) Get(ctx context.Context, name string) (*corev1.App, error) {
	var out corev1.App
	if err := c.rest.Get(ctx, "apps", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *apps) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.App], error) {
	var out metav1.List[corev1.App]
	if err := c.rest.List(ctx, "apps", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *apps) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "apps", "", opts, handler)
}

func (c *apps) Create(ctx context.Context, app *corev1.App) (*corev1.App, error) {
	var out corev1.App
	if err := c.rest.Create(ctx, app, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *apps) Update(ctx context.Context, app *corev1.App) (*corev1.App, error) {
	var out corev1.App
	if err := c.rest.Update(ctx, app, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *apps) Patch(ctx context.Context, name string, patchData []byte) (*corev1.App, error) {
	var out corev1.App
	if err := c.rest.Patch(ctx, "apps", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *apps) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "apps", "", name)
}
