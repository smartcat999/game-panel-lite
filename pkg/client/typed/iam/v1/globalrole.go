package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// GlobalRoleInterface has methods to work with GlobalRole resources.
type GlobalRoleInterface interface {
	Get(ctx context.Context, name string) (*iamv1.GlobalRole, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.GlobalRole], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error)
	Update(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error)
	Patch(ctx context.Context, name string, patchData []byte) (*iamv1.GlobalRole, error)
	Delete(ctx context.Context, name string) error
}

type globalRoles struct {
	rest rest.Interface
}

func newGlobalRoles(r rest.Interface) *globalRoles {
	return &globalRoles{rest: r}
}

func (c *globalRoles) Get(ctx context.Context, name string) (*iamv1.GlobalRole, error) {
	var out iamv1.GlobalRole
	if err := c.rest.Get(ctx, "globalroles", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoles) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.GlobalRole], error) {
	var out metav1.List[iamv1.GlobalRole]
	if err := c.rest.List(ctx, "globalroles", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoles) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "globalroles", "", opts, handler)
}

func (c *globalRoles) Create(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	var out iamv1.GlobalRole
	if err := c.rest.Create(ctx, role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoles) Update(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	var out iamv1.GlobalRole
	if err := c.rest.Update(ctx, role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoles) Patch(ctx context.Context, name string, patchData []byte) (*iamv1.GlobalRole, error) {
	var out iamv1.GlobalRole
	if err := c.rest.Patch(ctx, "globalroles", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *globalRoles) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "globalroles", "", name)
}
