package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// UserInterface has methods to work with User resources.
type UserInterface interface {
	Get(ctx context.Context, name string) (*iamv1.User, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.User], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, user *iamv1.User) (*iamv1.User, error)
	Update(ctx context.Context, user *iamv1.User) (*iamv1.User, error)
	UpdateStatus(ctx context.Context, user *iamv1.User) (*iamv1.User, error)
	Patch(ctx context.Context, name string, patchData []byte) (*iamv1.User, error)
	Delete(ctx context.Context, name string) error
}

type users struct {
	rest rest.Interface
}

func newUsers(r rest.Interface) *users {
	return &users{rest: r}
}

func (c *users) Get(ctx context.Context, name string) (*iamv1.User, error) {
	var out iamv1.User
	if err := c.rest.Get(ctx, "users", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.User], error) {
	var out metav1.List[iamv1.User]
	if err := c.rest.List(ctx, "users", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "users", "", opts, handler)
}

func (c *users) Create(ctx context.Context, user *iamv1.User) (*iamv1.User, error) {
	var out iamv1.User
	if err := c.rest.Create(ctx, user, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) Update(ctx context.Context, user *iamv1.User) (*iamv1.User, error) {
	var out iamv1.User
	if err := c.rest.Update(ctx, user, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) UpdateStatus(ctx context.Context, user *iamv1.User) (*iamv1.User, error) {
	var out iamv1.User
	if err := c.rest.UpdateStatus(ctx, user, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) Patch(ctx context.Context, name string, patchData []byte) (*iamv1.User, error) {
	var out iamv1.User
	if err := c.rest.Patch(ctx, "users", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *users) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "users", "", name)
}
