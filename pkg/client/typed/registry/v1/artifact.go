package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// ArtifactInterface has methods to work with cluster-scoped Artifact resources (<repository>.<package>.<version>).
type ArtifactInterface interface {
	Get(ctx context.Context, name string) (*registryv1.Artifact, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Artifact], error)
	Create(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error)
	Update(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error)
	UpdateStatus(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error)
	Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Artifact, error)
	Delete(ctx context.Context, name string) error
	PushLogs(ctx context.Context, name string, chunk *registryv1.ArtifactLogChunk) error
}

type artifacts struct {
	rest rest.Interface
}

func newArtifacts(r rest.Interface) *artifacts {
	return &artifacts{rest: r}
}

func (c *artifacts) Get(ctx context.Context, name string) (*registryv1.Artifact, error) {
	var out registryv1.Artifact
	if err := c.rest.Get(ctx, "artifacts", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Artifact], error) {
	var out metav1.List[registryv1.Artifact]
	if err := c.rest.List(ctx, "artifacts", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) Create(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error) {
	var out registryv1.Artifact
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) Update(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error) {
	var out registryv1.Artifact
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) UpdateStatus(ctx context.Context, obj *registryv1.Artifact) (*registryv1.Artifact, error) {
	var out registryv1.Artifact
	if err := c.rest.UpdateStatus(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) Patch(ctx context.Context, name string, patchData []byte) (*registryv1.Artifact, error) {
	var out registryv1.Artifact
	if err := c.rest.Patch(ctx, "artifacts", "", name, patchData, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *artifacts) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "artifacts", "", name)
}

func (c *artifacts) PushLogs(ctx context.Context, name string, chunk *registryv1.ArtifactLogChunk) error {
	return c.rest.PostSubresource(ctx, "artifacts", "", name, registryv1.SubresourceLog, chunk, nil)
}
