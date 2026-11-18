package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// GlobalRoleBindingInterface has methods to work with GlobalRoleBinding resources.
type GlobalRoleBindingInterface interface {
	Get(ctx context.Context, name string) (*iamv1.GlobalRoleBinding, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.GlobalRoleBinding], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error)
	Update(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error)
	Patch(ctx context.Context, name string, patchData []byte) (*iamv1.GlobalRoleBinding, error)
	Delete(ctx context.Context, name string) error
}

type globalRoleBindings struct {
	rest rest.Interface
}

func newGlobalRoleBindings(r rest.Interface) *globalRoleBindings {
	return &globalRoleBindings{rest: r}
}

func (c *globalRoleBindings) Get(ctx context.Context, name string) (*iamv1.GlobalRoleBinding, error) {
	var out iamv1.GlobalRoleBinding
	if err := c.rest.Get(ctx, "globalrolebindings", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoleBindings) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.GlobalRoleBinding], error) {
	var out metav1.List[iamv1.GlobalRoleBinding]
	if err := c.rest.List(ctx, "globalrolebindings", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoleBindings) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "globalrolebindings", "", opts, handler)
}

func (c *globalRoleBindings) Create(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error) {
	var out iamv1.GlobalRoleBinding
	if err := c.rest.Create(ctx, binding, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoleBindings) Update(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error) {
	var out iamv1.GlobalRoleBinding
	if err := c.rest.Update(ctx, binding, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoleBindings) Patch(ctx context.Context, name string, patchData []byte) (*iamv1.GlobalRoleBinding, error) {
	var out iamv1.GlobalRoleBinding
	if err := c.rest.Patch(ctx, "globalrolebindings", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoleBindings) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "globalrolebindings", "", name)
}
