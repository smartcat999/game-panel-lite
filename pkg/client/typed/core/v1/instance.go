package v1

import (
	"context"
	"io"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// InstanceInterface has methods to work with Instance resources and their subresources.
type InstanceInterface interface {
	Get(ctx context.Context, name string) (*corev1.Instance, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Instance], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error)
	Update(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error)
	UpdateStatus(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error)
	Patch(ctx context.Context, name string, patchData []byte) (*corev1.Instance, error)
	Delete(ctx context.Context, name string) error
	UpdateState(ctx context.Context, name string, state corev1.InstanceStateValue) (*corev1.Instance, error)
	StreamLogs(ctx context.Context, name string, opts rest.LogOptions) (io.ReadCloser, error)
	Exec(ctx context.Context, name string, cmd []string) (*corev1.InstanceExec, error)
	ExecInteractive(ctx context.Context, name string, in io.Reader, out io.Writer, cmd []string) error
	Files(ctx context.Context, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error)
}

type instances struct {
	rest   rest.Interface
	tenant string
}

func newInstances(r rest.Interface, tenant string) *instances {
	return &instances{rest: r, tenant: tenant}
}

func (c *instances) Get(ctx context.Context, name string) (*corev1.Instance, error) {
	var out corev1.Instance
	if err := c.rest.Get(ctx, "instances", c.tenant, name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Instance], error) {
	var out metav1.List[corev1.Instance]
	if err := c.rest.List(ctx, "instances", c.tenant, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "instances", c.tenant, opts, handler)
}

func (c *instances) Create(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error) {
	if instance != nil && c.tenant != "" && instance.Metadata.Tenant == "" {
		instance.Metadata.Tenant = c.tenant
	}
	var out corev1.Instance
	if err := c.rest.Create(ctx, instance, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) Update(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error) {
	if instance != nil && c.tenant != "" && instance.Metadata.Tenant == "" {
		instance.Metadata.Tenant = c.tenant
	}
	var out corev1.Instance
	if err := c.rest.Update(ctx, instance, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) UpdateStatus(ctx context.Context, instance *corev1.Instance) (*corev1.Instance, error) {
	if instance != nil && c.tenant != "" && instance.Metadata.Tenant == "" {
		instance.Metadata.Tenant = c.tenant
	}
	var out corev1.Instance
	if err := c.rest.UpdateStatus(ctx, instance, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) Patch(ctx context.Context, name string, patchData []byte) (*corev1.Instance, error) {
	var out corev1.Instance
	if err := c.rest.Patch(ctx, "instances", c.tenant, name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "instances", c.tenant, name)
}

func (c *instances) UpdateState(ctx context.Context, name string, state corev1.InstanceStateValue) (*corev1.Instance, error) {
	var out corev1.Instance
	if err := c.rest.UpdateInstanceState(ctx, c.tenant, name, state, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *instances) StreamLogs(ctx context.Context, name string, opts rest.LogOptions) (io.ReadCloser, error) {
	return c.rest.StreamLogs(ctx, c.tenant, name, opts)
}

func (c *instances) Exec(ctx context.Context, name string, cmd []string) (*corev1.InstanceExec, error) {
	return c.rest.Exec(ctx, c.tenant, name, cmd)
}

func (c *instances) ExecInteractive(ctx context.Context, name string, in io.Reader, out io.Writer, cmd []string) error {
	return c.rest.ExecInteractive(ctx, c.tenant, name, in, out, cmd)
}

func (c *instances) Files(ctx context.Context, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	return c.rest.Files(ctx, c.tenant, name, req)
}
