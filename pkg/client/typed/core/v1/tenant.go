package v1

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// TenantInterface has methods to work with Tenant resources.
type TenantInterface interface {
	Get(ctx context.Context, name string) (*corev1.Tenant, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Tenant], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error)
	Update(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error)
	UpdateStatus(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error)
	Patch(ctx context.Context, name string, patchData []byte) (*corev1.Tenant, error)
	Delete(ctx context.Context, name string) error
}

type tenants struct {
	rest rest.Interface
}

func newTenants(r rest.Interface) *tenants {
	return &tenants{rest: r}
}

func (c *tenants) Get(ctx context.Context, name string) (*corev1.Tenant, error) {
	var out corev1.Tenant
	if err := c.rest.Get(ctx, "tenants", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Tenant], error) {
	var out metav1.List[corev1.Tenant]
	if err := c.rest.List(ctx, "tenants", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "tenants", "", opts, handler)
}

func (c *tenants) Create(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	var out corev1.Tenant
	if err := c.rest.Create(ctx, tenant, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) Update(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	var out corev1.Tenant
	if err := c.rest.Update(ctx, tenant, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) UpdateStatus(ctx context.Context, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	var out corev1.Tenant
	if err := c.rest.UpdateStatus(ctx, tenant, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) Patch(ctx context.Context, name string, patchData []byte) (*corev1.Tenant, error) {
	var out corev1.Tenant
	if err := c.rest.Patch(ctx, "tenants", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *tenants) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "tenants", "", name)
}
