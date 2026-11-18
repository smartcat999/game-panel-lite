package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// TenantRoleBindingInterface has methods to work with TenantRoleBinding resources.
type TenantRoleBindingInterface interface {
	Get(ctx context.Context, name string) (*iamv1.TenantRoleBinding, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.TenantRoleBinding], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error)
	Update(ctx context.Context, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error)
	Patch(ctx context.Context, name string, patchData []byte) (*iamv1.TenantRoleBinding, error)
	Delete(ctx context.Context, name string) error
}

type tenantRoleBindings struct {
	rest   rest.Interface
	tenant string
}

func newTenantRoleBindings(r rest.Interface, tenant string) *tenantRoleBindings {
	return &tenantRoleBindings{rest: r, tenant: tenant}
}

func (c *tenantRoleBindings) Get(ctx context.Context, name string) (*iamv1.TenantRoleBinding, error) {
	var out iamv1.TenantRoleBinding
	if err := c.rest.Get(ctx, "tenantrolebindings", c.tenant, name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoleBindings) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.TenantRoleBinding], error) {
	var out metav1.List[iamv1.TenantRoleBinding]
	if err := c.rest.List(ctx, "tenantrolebindings", c.tenant, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoleBindings) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "tenantrolebindings", c.tenant, opts, handler)
}

func (c *tenantRoleBindings) Create(ctx context.Context, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error) {
	if binding != nil && c.tenant != "" && binding.Metadata.Tenant == "" {
		binding.Metadata.Tenant = c.tenant
	}
	var out iamv1.TenantRoleBinding
	if err := c.rest.Create(ctx, binding, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoleBindings) Update(ctx context.Context, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error) {
	if binding != nil && c.tenant != "" && binding.Metadata.Tenant == "" {
		binding.Metadata.Tenant = c.tenant
	}
	var out iamv1.TenantRoleBinding
	if err := c.rest.Update(ctx, binding, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoleBindings) Patch(ctx context.Context, name string, patchData []byte) (*iamv1.TenantRoleBinding, error) {
	var out iamv1.TenantRoleBinding
	if err := c.rest.Patch(ctx, "tenantrolebindings", c.tenant, name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoleBindings) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "tenantrolebindings", c.tenant, name)
}
