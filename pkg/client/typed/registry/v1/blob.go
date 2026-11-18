package v1

import (
	"context"
	"io"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// BlobInterface has methods to work with global content-addressed Blob resources.
type BlobInterface interface {
	GetMetadata(ctx context.Context, digest string) (*registryv1.Blob, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Blob], error)
	Register(ctx context.Context, obj *registryv1.Blob) (*registryv1.Blob, error)
	Download(ctx context.Context, digest string) (io.ReadCloser, error)
	DownloadURL(digest string) string
	Delete(ctx context.Context, digest string) error
}

type blobs struct {
	rest rest.Interface
}

func newBlobs(r rest.Interface) *blobs {
	return &blobs{rest: r}
}

func (c *blobs) GetMetadata(ctx context.Context, digest string) (*registryv1.Blob, error) {
	var out registryv1.Blob
	if err := c.rest.GetWithQuery(ctx, "blobs", "", digest, map[string]string{"metadata": "true"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobs) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[registryv1.Blob], error) {
	var out metav1.List[registryv1.Blob]
	if err := c.rest.List(ctx, "blobs", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobs) Register(ctx context.Context, obj *registryv1.Blob) (*registryv1.Blob, error) {
	var out registryv1.Blob
	if err := c.rest.Create(ctx, obj, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *blobs) Download(ctx context.Context, digest string) (io.ReadCloser, error) {
	return c.rest.DownloadBlob(ctx, digest)
}

func (c *blobs) DownloadURL(digest string) string {
	return c.rest.BaseURL() + "/apis/" + registryv1.GroupVersion + "/blobs/" + digest
}

func (c *blobs) Delete(ctx context.Context, digest string) error {
	return c.rest.Delete(ctx, "blobs", "", digest)
}
