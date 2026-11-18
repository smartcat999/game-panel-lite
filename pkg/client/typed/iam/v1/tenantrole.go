package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// TenantRoleInterface has methods to work with TenantRole resources.
type TenantRoleInterface interface {
	Get(ctx context.Context, name string) (*iamv1.TenantRole, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.TenantRole], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, role *iamv1.TenantRole) (*iamv1.TenantRole, error)
	Update(ctx context.Context, role *iamv1.TenantRole) (*iamv1.TenantRole, error)
	Patch(ctx context.Context, name string, patchData []byte) (*iamv1.TenantRole, error)
	Delete(ctx context.Context, name string) error
}

type tenantRoles struct {
	rest   rest.Interface
	tenant string
}

func newTenantRoles(r rest.Interface, tenant string) *tenantRoles {
	return &tenantRoles{rest: r, tenant: tenant}
}

func (c *tenantRoles) Get(ctx context.Context, name string) (*iamv1.TenantRole, error) {
	var out iamv1.TenantRole
	if err := c.rest.Get(ctx, "tenantroles", c.tenant, name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoles) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.TenantRole], error) {
	var out metav1.List[iamv1.TenantRole]
	if err := c.rest.List(ctx, "tenantroles", c.tenant, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoles) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "tenantroles", c.tenant, opts, handler)
}

func (c *tenantRoles) Create(ctx context.Context, role *iamv1.TenantRole) (*iamv1.TenantRole, error) {
	if role != nil && c.tenant != "" && role.Metadata.Tenant == "" {
		role.Metadata.Tenant = c.tenant
	}
	var out iamv1.TenantRole
	if err := c.rest.Create(ctx, role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoles) Update(ctx context.Context, role *iamv1.TenantRole) (*iamv1.TenantRole, error) {
	if role != nil && c.tenant != "" && role.Metadata.Tenant == "" {
		role.Metadata.Tenant = c.tenant
	}
	var out iamv1.TenantRole
	if err := c.rest.Update(ctx, role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoles) Patch(ctx context.Context, name string, patchData []byte) (*iamv1.TenantRole, error) {
	var out iamv1.TenantRole
	if err := c.rest.Patch(ctx, "tenantroles", c.tenant, name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenantRoles) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "tenantroles", c.tenant, name)
}
