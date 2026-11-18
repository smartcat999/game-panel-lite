package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// ReplicationPolicyInterface has methods to work with cluster-scoped ReplicationPolicy resources.
type ReplicationPolicyInterface interface {
	Get(ctx context.Context, name string) (*registryv1.ReplicationPolicy, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.ReplicationPolicy], error)
	Create(ctx context.Context, obj *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error)
	Update(ctx context.Context, obj *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error)
	Patch(ctx context.Context, name string, patchData []byte) (*registryv1.ReplicationPolicy, error)
	Delete(ctx context.Context, name string) error
}

type replicationPolicies struct {
	rest rest.Interface
}

func newReplicationPolicies(r rest.Interface) *replicationPolicies {
	return &replicationPolicies{rest: r}
}

func (c *replicationPolicies) Get(ctx context.Context, name string) (*registryv1.ReplicationPolicy, error) {
	var out registryv1.ReplicationPolicy
	if err := c.rest.Get(ctx, "replicationpolicies", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *replicationPolicies) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.ReplicationPolicy], error) {
	var out metav1.List[registryv1.ReplicationPolicy]
	if err := c.rest.List(ctx, "replicationpolicies", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *replicationPolicies) Create(ctx context.Context, obj *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	var out registryv1.ReplicationPolicy
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *replicationPolicies) Update(ctx context.Context, obj *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	var out registryv1.ReplicationPolicy
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *replicationPolicies) Patch(ctx context.Context, name string, patchData []byte) (*registryv1.ReplicationPolicy, error) {
	var out registryv1.ReplicationPolicy
	if err := c.rest.Patch(ctx, "replicationpolicies", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *replicationPolicies) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "replicationpolicies", "", name)
}
