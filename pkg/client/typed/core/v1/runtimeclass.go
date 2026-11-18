package v1

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// RuntimeClassInterface has methods to work with RuntimeClass resources.
type RuntimeClassInterface interface {
	Get(ctx context.Context, name string) (*corev1.RuntimeClass, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.RuntimeClass], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error)
	Update(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error)
	Patch(ctx context.Context, name string, patchData []byte) (*corev1.RuntimeClass, error)
	Delete(ctx context.Context, name string) error
}

type runtimeClasses struct {
	rest rest.Interface
}

func newRuntimeClasses(r rest.Interface) *runtimeClasses {
	return &runtimeClasses{rest: r}
}

func (c *runtimeClasses) Get(ctx context.Context, name string) (*corev1.RuntimeClass, error) {
	var out corev1.RuntimeClass
	if err := c.rest.Get(ctx, "runtimeclasses", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *runtimeClasses) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.RuntimeClass], error) {
	var out metav1.List[corev1.RuntimeClass]
	if err := c.rest.List(ctx, "runtimeclasses", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *runtimeClasses) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "runtimeclasses", "", opts, handler)
}

func (c *runtimeClasses) Create(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	var out corev1.RuntimeClass
	if err := c.rest.Create(ctx, rc, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *runtimeClasses) Update(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	var out corev1.RuntimeClass
	if err := c.rest.Update(ctx, rc, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *runtimeClasses) Patch(ctx context.Context, name string, patchData []byte) (*corev1.RuntimeClass, error) {
	var out corev1.RuntimeClass
	if err := c.rest.Patch(ctx, "runtimeclasses", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *runtimeClasses) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "runtimeclasses", "", name)
}
