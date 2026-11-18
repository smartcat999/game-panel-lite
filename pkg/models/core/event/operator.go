package event

import (
	"context"
	"fmt"
	"maps"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Event], error)
}

type operator struct {
	db       *auditdb.DB
	tenantDB *tenantdb.DB
}

func NewOperator(reg *repo.Registry) Operator {
	op := &operator{}
	if reg != nil {
		op.db = auditdb.NewDB(reg.Pool())
		op.tenantDB = tenantdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Event], error) {
	if o.db == nil {
		return &metav1.List[*corev1.Event]{
			TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "EventList"},
			Items:    []*corev1.Event{},
		}, nil
	}
	tenantUID := ""
	if opts.Tenant != "" {
		if o.tenantDB == nil {
			return nil, fmt.Errorf("%w: current tenant identity is unavailable", models.ErrConflict)
		}
		tenant, err := o.tenantDB.Get(ctx, opts.Tenant)
		if err != nil {
			return nil, err
		}
		if tenant == nil || tenant.Metadata.UID == "" {
			return nil, fmt.Errorf("%w: current tenant UID is required", models.ErrConflict)
		}
		tenantUID = tenant.Metadata.UID
		opts.FieldSelector = maps.Clone(opts.FieldSelector)
		if opts.FieldSelector == nil {
			opts.FieldSelector = make(map[string]string)
		}
		opts.FieldSelector["attributes.tenantUID"] = tenantUID
	}
	list, err := o.db.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	if tenantUID != "" && list != nil {
		for _, event := range list.Items {
			if event == nil || event.Metadata.Tenant != opts.Tenant || event.Attributes["tenantUID"] != tenantUID {
				return nil, fmt.Errorf("%w: audit store returned a different tenant identity", models.ErrConflict)
			}
		}
	}
	return list, nil
}
