package v1

import (
	"context"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// BlobUploadInterface has methods to initiate, complete, and abort direct S3 BlobUpload sessions.
type BlobUploadInterface interface {
	Get(ctx context.Context, name string) (*registryv1.BlobUpload, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.BlobUpload], error)
	Create(ctx context.Context, obj *registryv1.BlobUpload) (*registryv1.BlobUpload, error)
	Complete(ctx context.Context, obj *registryv1.BlobUpload) (*registryv1.BlobUpload, error)
	Delete(ctx context.Context, name string) error
}

type blobUploads struct {
	rest rest.Interface
}

func newBlobUploads(r rest.Interface) *blobUploads {
	return &blobUploads{rest: r}
}

func (c *blobUploads) Get(ctx context.Context, name string) (*registryv1.BlobUpload, error) {
	var out registryv1.BlobUpload
	if err := c.rest.Get(ctx, "blobuploads", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobUploads) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.BlobUpload], error) {
	var out metav1.List[registryv1.BlobUpload]
	if err := c.rest.List(ctx, "blobuploads", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobUploads) Create(ctx context.Context, obj *registryv1.BlobUpload) (*registryv1.BlobUpload, error) {
	var out registryv1.BlobUpload
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobUploads) Complete(ctx context.Context, obj *registryv1.BlobUpload) (*registryv1.BlobUpload, error) {
	var out registryv1.BlobUpload
	if err := c.rest.Update(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobUploads) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "blobuploads", "", name)
}
