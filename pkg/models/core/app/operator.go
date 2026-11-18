package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.App], error)
	Get(ctx context.Context, name string) (*corev1.App, error)
	Create(ctx context.Context, app *corev1.App) (*corev1.App, error)
	Update(ctx context.Context, name string, app *corev1.App) (*corev1.App, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db  *appdb.DB
	bus mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface, _ ...cache.Interface) Operator {
	op := &operator{bus: bus}
	if reg != nil {
		op.db = appdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, corev1.GroupVersion, "App", "", name, generation, resourceVersion)
	ev.Topic = mq.TopicApp
	_ = o.bus.Publish(ctx, mq.TopicApp, ev)
	wake := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, corev1.KindPrewarmJob, "", corev1.PrewarmSyncDefaultsName, 0, "")
	wake.Topic = mq.TopicPrewarmJob
	_ = o.bus.Publish(ctx, mq.TopicPrewarmJob, wake)
	if eventType == mq.EventCreated || eventType == mq.EventUpdated {
		syncEv := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", "", corev1.InstanceSyncAppMetricsPrefix+name, 0, "")
		syncEv.Topic = mq.TopicInstance
		_ = o.bus.Publish(ctx, mq.TopicInstance, syncEv)
	}
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.App], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.App, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, app *corev1.App) (*corev1.App, error) {
	app.DefaultMeta()
	normalizeAndSortAppVersions(app)
	if err := app.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	created, err := o.db.Create(ctx, app)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, app *corev1.App) (*corev1.App, error) {
	app.Metadata.Name = name
	if existing, err := o.db.Get(ctx, name); err == nil && existing != nil {
		app.BuiltIn = app.BuiltIn || existing.BuiltIn
		if app.Metadata.UID == "" {
			app.Metadata.UID = existing.Metadata.UID
		}
		if app.Metadata.ResourceVersion == "" {
			app.Metadata.ResourceVersion = existing.Metadata.ResourceVersion
		}
	}
	app.DefaultMeta()
	normalizeAndSortAppVersions(app)
	if err := app.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	updated, err := o.db.Update(ctx, app)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

// SyncAppInstanceMetrics propagates updated App metrics definitions across all instances labeled with that App.
func SyncAppInstanceMetrics(ctx context.Context, reg *repo.Registry, bus mq.Interface, appName string) error {
	if reg == nil || appName == "" {
		return nil
	}
	appObj, err := appdb.NewDB(reg.Pool()).Get(ctx, appName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	instDB := instancedb.NewDB(reg.Pool())
	const pageSize = 100
	for offset := 0; ; offset += pageSize {
		list, err := instDB.List(ctx, db.ListOptions{
			LabelSelector:  &metav1.LabelSelector{MatchLabels: map[string]string{corev1.LabelInstanceApp: appName}},
			Limit:          pageSize,
			Offset:         offset,
			SkipTotalCount: true,
		})
		if err != nil {
			return err
		}
		if list == nil || len(list.Items) == 0 {
			return nil
		}
		for _, inst := range list.Items {
			if err := syncSingleInstanceMetrics(ctx, instDB, bus, inst, appObj); err != nil {
				return err
			}
		}
		if len(list.Items) < pageSize {
			return nil
		}
	}
}

func syncSingleInstanceMetrics(ctx context.Context, instDB *instancedb.DB, bus mq.Interface, item *corev1.Instance, appObj *corev1.App) error {
	if item == nil || item.Metadata.Labels[corev1.LabelInstanceApp] != appObj.Metadata.Name {
		return nil
	}
	for range 4 {
		cur, err := instDB.Get(ctx, item.Metadata.Tenant, item.Metadata.Name)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil
			}
			return err
		}
		rendered := renderInstanceMetricsForApp(cur, appObj)
		deliveryChanged := syncInstanceConsoleDelivery(cur, appObj)
		if reflect.DeepEqual(cur.Spec.Metrics, rendered) && !deliveryChanged {
			return nil
		}
		cur.Spec.Metrics = rendered
		saved, err := instDB.Update(ctx, cur)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		if bus != nil {
			ev := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", saved.Metadata.Tenant, saved.Metadata.Name, saved.Metadata.Generation, saved.Metadata.ResourceVersion)
			ev.Topic = mq.TopicInstance
			_ = bus.Publish(ctx, mq.TopicInstance, ev)
		}
		return nil
	}
	return db.ErrConflict
}

func renderInstanceMetricsForApp(inst *corev1.Instance, updated *corev1.App) []corev1.AppMetricSpec {
	var savedCfg map[string]string
	if raw := inst.Metadata.Annotations[AnnotationAppConfig]; strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &savedCfg)
	}
	rendered := make([]corev1.AppMetricSpec, len(updated.Metrics))
	for i, m := range updated.Metrics {
		cp := m
		if cp.MaxConfigKey != "" && savedCfg != nil {
			if v := strings.TrimSpace(savedCfg[cp.MaxConfigKey]); v != "" {
				cp.MaxValue = v
			}
		}
		if cp.MaxValue == "" && cp.MaxConfigKey != "" {
			for _, f := range updated.ConfigSchema {
				if f.Name == cp.MaxConfigKey && strings.TrimSpace(f.Default) != "" {
					cp.MaxValue = strings.TrimSpace(f.Default)
					break
				}
			}
		}
		rendered[i] = cp
	}
	return rendered
}

func syncInstanceConsoleDelivery(inst *corev1.Instance, appObj *corev1.App) bool {
	if inst == nil || appObj == nil {
		return false
	}
	desired := strings.TrimSpace(appObj.ConsoleDelivery)
	if desired == "" {
		return false
	}
	if inst.Spec.BackupPolicy == nil {
		inst.Spec.BackupPolicy = &corev1.InstanceBackupPolicy{ConsoleDelivery: desired}
		return true
	}
	if strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery) == desired {
		return false
	}
	inst.Spec.BackupPolicy.ConsoleDelivery = desired
	return true
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if existing, err := o.db.Get(ctx, name); err == nil && existing != nil && existing.BuiltIn {
		return fmt.Errorf("%w: cannot delete built-in app %q (you can unpublish it instead)", models.ErrConflict, name)
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, name, 0, "")
	return nil
}
