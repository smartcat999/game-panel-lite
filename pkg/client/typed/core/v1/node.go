package v1

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// NodeInterface has methods to work with Node resources.
type NodeInterface interface {
	Get(ctx context.Context, name string) (*corev1.Node, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Node], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, node *corev1.Node) (*corev1.Node, error)
	Update(ctx context.Context, node *corev1.Node) (*corev1.Node, error)
	UpdateStatus(ctx context.Context, node *corev1.Node) (*corev1.Node, error)
	Patch(ctx context.Context, name string, patchData []byte) (*corev1.Node, error)
	Delete(ctx context.Context, name string) error
	Cordon(ctx context.Context, name string) (*corev1.Node, error)
	Uncordon(ctx context.Context, name string) (*corev1.Node, error)
}

type nodes struct {
	rest rest.Interface
}

func newNodes(r rest.Interface) *nodes {
	return &nodes{rest: r}
}

func (c *nodes) Get(ctx context.Context, name string) (*corev1.Node, error) {
	var out corev1.Node
	if err := c.rest.Get(ctx, "nodes", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.Node], error) {
	var out metav1.List[corev1.Node]
	if err := c.rest.List(ctx, "nodes", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "nodes", "", opts, handler)
}

func (c *nodes) Create(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	var out corev1.Node
	if err := c.rest.Create(ctx, node, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) Update(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	var out corev1.Node
	if err := c.rest.Update(ctx, node, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) UpdateStatus(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	var out corev1.Node
	if err := c.rest.UpdateStatus(ctx, node, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) Patch(ctx context.Context, name string, patchData []byte) (*corev1.Node, error) {
	var out corev1.Node
	if err := c.rest.Patch(ctx, "nodes", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *nodes) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "nodes", "", name)
}

func (c *nodes) Cordon(ctx context.Context, name string) (*corev1.Node, error) {
	return c.setUnschedulable(ctx, name, true)
}

func (c *nodes) Uncordon(ctx context.Context, name string) (*corev1.Node, error) {
	return c.setUnschedulable(ctx, name, false)
}

func (c *nodes) setUnschedulable(ctx context.Context, name string, unschedulable bool) (*corev1.Node, error) {
	node, err := c.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	node.Spec.Unschedulable = unschedulable
	return c.Update(ctx, node)
}
