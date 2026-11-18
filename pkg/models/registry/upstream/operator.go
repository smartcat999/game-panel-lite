package upstream

import (
	"context"
	"errors"
	"fmt"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Upstream], error)
	Get(ctx context.Context, name string) (*registryv1.Upstream, error)
	Create(ctx context.Context, in *registryv1.Upstream) (*registryv1.Upstream, error)
	Update(ctx context.Context, name string, in *registryv1.Upstream) (*registryv1.Upstream, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db     *upstreamdb.DB
	repoDB *repositorydb.DB
}

func NewOperator(reg *repo.Registry) Operator {
	op := &operator{}
	if reg != nil {
		op.db = upstreamdb.NewDB(reg.Pool())
		op.repoDB = repositorydb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Upstream], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.Upstream, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, in *registryv1.Upstream) (*registryv1.Upstream, error) {
	created, err := o.db.Create(ctx, in)
	if err != nil {
		return nil, wrapWriteErr(err)
	}
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, in *registryv1.Upstream) (*registryv1.Upstream, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	cp := *in
	cp.Metadata.Name = existing.Metadata.Name
	if cp.APIKey == "" {
		cp.APIKey = existing.APIKey
	}
	updated, err := o.db.Update(ctx, &cp)
	if err != nil {
		return nil, wrapWriteErr(err)
	}
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if _, err := o.db.Get(ctx, name); err != nil {
		return err
	}
	repos, err := o.repoDB.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"spec.upstreamName": name},
		Limit:         1,
	})
	if err != nil {
		return err
	}
	if repos != nil && len(repos.Items) > 0 {
		return fmt.Errorf("%w: upstream %q is referenced by repository %q", models.ErrConflict, name, repos.Items[0].Metadata.Name)
	}
	return o.db.Delete(ctx, name)
}

func wrapWriteErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, db.ErrNotFound) || errors.Is(err, db.ErrAlreadyExists) || errors.Is(err, db.ErrConflict) {
		return err
	}
	return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
}
