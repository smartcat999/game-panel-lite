package blob

import (
	"context"
	"errors"
	"fmt"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	blobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Blob], error)
	Get(ctx context.Context, digest string) (*registryv1.Blob, error)
	Register(ctx context.Context, in *registryv1.Blob) (*registryv1.Blob, error)
	PresignDownload(ctx context.Context, digest, publicHost string) (*registryv1.Blob, string, time.Time, error)
	Delete(ctx context.Context, digest string) error
}

type operator struct {
	db *blobdb.DB
	s3 s3.Interface
}

func NewOperator(reg *repo.Registry, s3Cli s3.Interface) Operator {
	op := &operator{s3: s3Cli}
	if reg != nil {
		op.db = blobdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) s3Client() s3.Interface {
	if o.s3 != nil {
		return o.s3
	}
	return s3.Default()
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Blob], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, digest string) (*registryv1.Blob, error) {
	return o.db.Get(ctx, digest)
}

func (o *operator) Register(ctx context.Context, in *registryv1.Blob) (*registryv1.Blob, error) {
	created, err := o.db.CreateOrGet(ctx, in)
	if err != nil {
		if errors.Is(err, db.ErrConflict) || errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	return created, nil
}

func (o *operator) PresignDownload(ctx context.Context, digest, publicHost string) (*registryv1.Blob, string, time.Time, error) {
	b, err := o.db.Get(ctx, digest)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	cli := o.s3Client()
	if cli == nil || !cli.IsConfigured() {
		return nil, "", time.Time{}, fmt.Errorf("%w: s3 storage is not configured", models.ErrBadGateway)
	}
	ttl := config.Get().Registry.BlobPresignTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	dlURL, exp, err := cli.PresignGet(ctx, b.ObjectKey, s3.PresignOptions{PublicHostOverride: publicHost, Expires: ttl})
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	return b, dlURL, exp, nil
}

func (o *operator) Delete(ctx context.Context, digest string) error {
	b, err := o.db.Get(ctx, digest)
	if err != nil {
		return err
	}
	if err := o.db.DeleteUnreferenced(ctx, b.Digest); err != nil {
		return err
	}
	if cli := o.s3Client(); cli != nil && cli.IsConfigured() && b.ObjectKey != "" {
		_ = cli.Delete(ctx, b.ObjectKey)
	}
	return nil
}
