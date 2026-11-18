package instance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for Instance across instances (spec/metadata/resourceControl)
// and instance_observations (runtime observations, schedule statuses, and backup mirrors).
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilSlice[T any](in []T) []T {
	if len(in) == 0 {
		return []T{}
	}
	return append([]T(nil), in...)
}

func nonNilResList(in corev1.ResourceList) corev1.ResourceList {
	if in == nil {
		return corev1.ResourceList{}
	}
	cp := make(corev1.ResourceList, len(in))
	for k, v := range in {
		cp[k] = v
	}
	return cp
}

func isControlConditionType(t string) bool {
	switch t {
	case metav1.ConditionTypeQuotaReserved,
		metav1.ConditionTypeScheduled,
		metav1.ConditionTypeResourcesApproved,
		metav1.ConditionTypeResourcesSettled,
		metav1.ConditionTypeReconciled:
		return true
	default:
		return false
	}
}

func filterConditions(conds []metav1.Condition, wantControl bool) []metav1.Condition {
	out := make([]metav1.Condition, 0, len(conds))
	for _, c := range conds {
		if isControlConditionType(c.Type) == wantControl {
			out = append(out, c)
		}
	}
	return out
}

func stripObservedControl(rc *corev1.InstanceResourceControl) *corev1.InstanceResourceControl {
	if rc == nil {
		return nil
	}
	cp := *rc
	cp.ObservedRevision = 0
	cp.ObservedAttemptID = ""
	cp.ObservedEpoch = 0
	cp.ObservedResources = nil
	cp.ObservedPorts = nil
	return &cp
}

func toInstanceRecord(in *corev1.Instance, rv int64) corev1.InstanceRecord {
	var stage string
	if in.Status.ResourceControl != nil {
		stage = in.Status.ResourceControl.Stage
	}
	return corev1.InstanceRecord{
		Tenant: in.Metadata.Tenant, Name: in.Metadata.Name, UID: in.Metadata.UID,
		RuntimeClassName: in.Spec.RuntimeClassName, State: string(in.Spec.State),
		NodeName: in.Spec.NodeName, NodeSelector: pgutil.NonNilMap(in.Spec.NodeSelector),
		Image: in.Spec.Image, Command: pgutil.NonNilStrings(in.Spec.Command),
		Args: pgutil.NonNilStrings(in.Spec.Args), WorkingDir: in.Spec.WorkingDir,
		ServerArtifact: in.Spec.ServerArtifact, Addons: nonNilSlice(in.Spec.Addons),
		Resources: corev1.ResourceRequirements{
			Requests: nonNilResList(in.Spec.Resources.Requests),
			Limits:   nonNilResList(in.Spec.Resources.Limits),
		},
		Ports: nonNilSlice(in.Spec.Ports), Env: pgutil.NonNilMap(in.Spec.Env),
		Files: nonNilSlice(in.Spec.Files), BackupPolicy: in.Spec.BackupPolicy,
		Schedules: nonNilSlice(in.Spec.Schedules), Metrics: nonNilSlice(in.Spec.Metrics),
		ControlStage: stage, ResourceControl: stripObservedControl(in.Status.ResourceControl),
		ControlConditions: filterConditions(in.Status.Conditions, true),
		ResourceVersion:   rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func toObservationRecord(in *corev1.Instance, sv int64) corev1.InstanceObservationRecord {
	phase := in.Status.Phase
	if phase == "" {
		phase = corev1.InstancePhasePending
	}
	obs := corev1.InstanceObservationRecord{
		InstanceUID: in.Metadata.UID, Tenant: in.Metadata.Tenant, InstanceName: in.Metadata.Name,
		Version: sv, ObservedGeneration: in.Status.ObservedGeneration,
		Phase: string(phase), RuntimeID: in.Status.RuntimeID, Endpoint: in.Status.Endpoint,
		StartedAt: in.Status.StartedAt, ObservedAt: in.Status.ObservedAt,
		TransitionID:      in.Status.TransitionID,
		ObservedResources: corev1.ResourceList{}, ObservedPorts: []corev1.InstancePort{},
		Conditions:       filterConditions(in.Status.Conditions, false),
		ScheduleStatuses: nonNilSlice(in.Status.ScheduleStatuses),
		Backups:          nonNilSlice(in.Status.Backups),
		Usage:            in.Status.Usage, UpdatedAt: time.Now().UTC(),
	}
	if rc := in.Status.ResourceControl; rc != nil {
		obs.ObservedRevision = rc.ObservedRevision
		obs.ObservedAttemptID = rc.ObservedAttemptID
		obs.ObservedEpoch = rc.ObservedEpoch
		obs.ObservedResources = nonNilResList(rc.ObservedResources)
		obs.ObservedPorts = nonNilSlice(rc.ObservedPorts)
	}
	return obs
}

func fromInstanceRecords(rec corev1.InstanceRecord, obs corev1.InstanceObservationRecord, hasObs bool) *corev1.Instance {
	out := &corev1.Instance{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Instance"},
		Metadata: metav1.ObjectMeta{
			Tenant: rec.Tenant, Name: rec.Name, UID: rec.UID,
			ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion), Generation: rec.Generation,
			CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: rec.RuntimeClassName, State: corev1.InstanceStateValue(rec.State),
			NodeName: rec.NodeName, Image: rec.Image,
			Command: append([]string(nil), rec.Command...), Args: append([]string(nil), rec.Args...),
			WorkingDir: rec.WorkingDir, ServerArtifact: rec.ServerArtifact,
			Addons: append([]corev1.InstanceAddon(nil), rec.Addons...), Resources: rec.Resources,
			Ports: append([]corev1.InstancePort(nil), rec.Ports...),
			Files: append([]corev1.InstanceFile(nil), rec.Files...), BackupPolicy: rec.BackupPolicy,
			Schedules: append([]corev1.InstanceSchedule(nil), rec.Schedules...),
			Metrics:   append([]corev1.AppMetricSpec(nil), rec.Metrics...),
		},
		Status: corev1.InstanceStatus{Phase: corev1.InstancePhasePending},
	}
	mergeInstanceMetaAndControl(out, rec, obs, hasObs)
	return out
}

func mergeInstanceMetaAndControl(out *corev1.Instance, rec corev1.InstanceRecord, obs corev1.InstanceObservationRecord, hasObs bool) {
	if len(rec.NodeSelector) > 0 {
		out.Spec.NodeSelector = pgutil.NonNilMap(rec.NodeSelector)
	}
	if len(rec.Env) > 0 {
		out.Spec.Env = pgutil.NonNilMap(rec.Env)
	}
	if len(rec.Finalizers) > 0 {
		out.Metadata.Finalizers = append([]string(nil), rec.Finalizers...)
	}
	if len(rec.Labels) > 0 {
		out.Metadata.Labels = pgutil.NonNilMap(rec.Labels)
	}
	if len(rec.Annotations) > 0 {
		out.Metadata.Annotations = pgutil.NonNilMap(rec.Annotations)
	}
	if hasObs {
		out.Metadata.StatusVersion = pgutil.FormatVersion(obs.Version)
		out.Status = corev1.InstanceStatus{
			ObservedGeneration: obs.ObservedGeneration, Phase: corev1.InstancePhase(obs.Phase),
			RuntimeID: obs.RuntimeID, Endpoint: obs.Endpoint, Usage: obs.Usage,
			Backups:          append([]corev1.InstanceBackupRecord(nil), obs.Backups...),
			ScheduleStatuses: append([]corev1.InstanceScheduleStatus(nil), obs.ScheduleStatuses...),
			StartedAt:        obs.StartedAt, ObservedAt: obs.ObservedAt, TransitionID: obs.TransitionID,
		}
	}
	mergeControlAndConditions(out, rec, obs, hasObs)
}

func mergeControlAndConditions(out *corev1.Instance, rec corev1.InstanceRecord, obs corev1.InstanceObservationRecord, hasObs bool) {
	if rec.ResourceControl != nil {
		cp := *rec.ResourceControl
		if hasObs {
			cp.ObservedRevision = obs.ObservedRevision
			cp.ObservedAttemptID = obs.ObservedAttemptID
			cp.ObservedEpoch = obs.ObservedEpoch
			if len(obs.ObservedResources) > 0 {
				cp.ObservedResources = nonNilResList(obs.ObservedResources)
			}
			if len(obs.ObservedPorts) > 0 {
				cp.ObservedPorts = append([]corev1.InstancePort(nil), obs.ObservedPorts...)
			}
		}
		out.Status.ResourceControl = &cp
	}
	conds := make([]metav1.Condition, 0, len(rec.ControlConditions)+len(obs.Conditions))
	conds = append(conds, rec.ControlConditions...)
	if hasObs {
		conds = append(conds, obs.Conditions...)
	}
	if len(conds) > 0 {
		out.Status.Conditions = conds
	}
	if rec.DeletedAt != nil {
		out.Status.Phase = corev1.InstancePhaseTerminating
	}
}

func (s *DB) Create(ctx context.Context, in *corev1.Instance) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	cp.Metadata.StatusVersion = "1"
	rec := toInstanceRecord(&cp, 1)
	obs := toObservationRecord(&cp, 1)
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant"}, {Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	if err := tx.Create(&obs).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return fromInstanceRecords(rec, obs, true), nil
}

func (s *DB) Get(ctx context.Context, tenant, name string) (*corev1.Instance, error) {
	out, err := s.getRaw(ctx, tenant, name)
	if err == nil {
		overlayCachedUsage(ctx, out)
	}
	return out, err
}

func (s *DB) getRaw(ctx context.Context, tenant, name string) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.InstanceRecord
	if err := s.orm.WithContext(ctx).Where("tenant = ? AND name = ?", tenant, name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var obs corev1.InstanceObservationRecord
	err := s.orm.WithContext(ctx).Where("instance_uid = ?", rec.UID).Take(&obs).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromInstanceRecords(rec, obs, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Instance], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.InstanceRecord{})
	if opts.Tenant != "" {
		tx = tx.Where("instances.tenant = ?", opts.Tenant)
	}
	tx, err := applyInstanceFieldSelectors(tx, opts.FieldSelector)
	if err != nil {
		return nil, err
	}
	tx = db.ApplyLabelSelectorCol(tx, "instances.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.InstanceRecord](tx, "instances.created_at DESC, instances.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateInstanceList(ctx, rows, total, opts.LabelSelector)
}

func applyInstanceFieldSelectors(tx *gorm.DB, selectors map[string]string) (*gorm.DB, error) {
	for _, field := range pgutil.SortedFieldKeys(selectors) {
		val := selectors[field]
		switch field {
		case "metadata.tenant":
			tx = tx.Where("instances.tenant = ?", val)
		case "metadata.name":
			tx = tx.Where("instances.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "instances.name", val)
		case "spec.nodeName":
			tx = tx.Where("instances.node_name = ?", val)
		case "spec.state":
			tx = tx.Where("instances.state = ?", val)
		case "spec.runtimeClassName":
			tx = tx.Where("instances.runtime_class_name = ?", val)
		case "status.resourceControl.stage":
			tx = tx.Where("instances.control_stage = ?", val)
		case "status.phase":
			tx = tx.Joins("JOIN instance_observations ON instance_observations.instance_uid = instances.uid").
				Where("CASE WHEN instances.deleted_at IS NOT NULL THEN ? ELSE instance_observations.phase END = ?", string(corev1.InstancePhaseTerminating), val)
		default:
			return nil, fmt.Errorf("unsupported instance fieldSelector %q", field)
		}
	}
	return tx, nil
}

func (s *DB) hydrateInstanceList(ctx context.Context, rows []corev1.InstanceRecord, total int, sel *metav1.LabelSelector) (*metav1.List[*corev1.Instance], error) {
	obsMap := make(map[string]corev1.InstanceObservationRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var obsRows []corev1.InstanceObservationRecord
		if err := s.orm.WithContext(ctx).Where("instance_uid IN ?", uids).Find(&obsRows).Error; err != nil {
			return nil, err
		}
		for _, o := range obsRows {
			obsMap[o.InstanceUID] = o
		}
	}
	items := make([]*corev1.Instance, 0, len(rows))
	for _, row := range rows {
		obs, ok := obsMap[row.UID]
		item := fromInstanceRecords(row, obs, ok)
		if sel != nil && !sel.Matches(item.Metadata.Labels) {
			continue
		}
		overlayCachedUsage(ctx, item)
		items = append(items, item)
	}
	return &metav1.List[*corev1.Instance]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Delete(ctx context.Context, tenant, name string) error {
	return s.DeleteIfUID(ctx, tenant, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, tenant, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	if expectedUID != "" {
		deleteCachedUsage(ctx, expectedUID)
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "instances", "tenant = ? AND name = ?", []any{tenant, name}, expectedUID)
}

type cachedUsageEntry struct {
	ObservedAt *time.Time            `json:"observedAt,omitempty"`
	Usage      *corev1.InstanceUsage `json:"usage,omitempty"`
}

func usageCacheKey(uid string) string { return "inst:usage:" + uid }

func overlayCachedUsage(ctx context.Context, inst *corev1.Instance) {
	if inst == nil || inst.Metadata.UID == "" || inst.Metadata.DeletionTimestamp != nil || inst.Status.Phase != corev1.InstancePhaseRunning || cache.Default() == nil {
		return
	}
	if cached, ok := cache.GetJSON[cachedUsageEntry](ctx, nil, usageCacheKey(inst.Metadata.UID)); ok && cached.Usage != nil {
		if cached.ObservedAt == nil || inst.Status.ObservedAt == nil || !cached.ObservedAt.Before(*inst.Status.ObservedAt) {
			inst.Status.Usage = cached.Usage
			if cached.ObservedAt != nil {
				inst.Status.ObservedAt = cached.ObservedAt
			}
		}
	}
}

func deleteCachedUsage(ctx context.Context, uid string) {
	if store := cache.Default(); store != nil && uid != "" {
		_ = store.Delete(ctx, usageCacheKey(uid))
	}
}
