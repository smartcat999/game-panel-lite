package prewarmjob

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for PrewarmJob across prewarm_jobs and prewarm_job_states.
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilItems[T any](in []T) []T {
	if len(in) == 0 {
		return []T{}
	}
	return append([]T(nil), in...)
}

func toJobRecords(in *corev1.PrewarmJob, rv, sv int64) (corev1.PrewarmJobRecord, corev1.PrewarmJobStateRecord) {
	phase := in.Status.Phase
	if phase == "" {
		phase = corev1.PrewarmJobPhasePending
	}
	rec := corev1.PrewarmJobRecord{
		Name:            in.Metadata.Name,
		UID:             in.Metadata.UID,
		NodeNames:       pgutil.NonNilStrings(in.Spec.NodeNames),
		NodeSelector:    pgutil.NonNilMap(in.Spec.NodeSelector),
		Artifacts:       nonNilItems(in.Spec.Artifacts),
		Images:          pgutil.NonNilStrings(in.Spec.Images),
		ResourceVersion: rv,
		Generation:      in.Metadata.Generation,
		CreatedAt:       in.Metadata.CreationTimestamp,
		DeletedAt:       in.Metadata.DeletionTimestamp,
		Finalizers:      pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:          pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:     pgutil.NonNilMap(in.Metadata.Annotations),
	}
	st := corev1.PrewarmJobStateRecord{
		JobUID:             in.Metadata.UID,
		Version:            sv,
		ObservedGeneration: in.Status.ObservedGeneration,
		Phase:              string(phase),
		TargetNodes:        in.Status.TargetNodes,
		ReadyNodes:         in.Status.ReadyNodes,
		FailedNodes:        in.Status.FailedNodes,
		Nodes:              nonNilItems(in.Status.Nodes),
		Conditions:         pgutil.NonNilConditions(in.Status.Conditions),
		UpdatedAt:          time.Now().UTC(),
	}
	return rec, st
}

func fromJobRecords(rec corev1.PrewarmJobRecord, st corev1.PrewarmJobStateRecord, hasState bool) *corev1.PrewarmJob {
	out := &corev1.PrewarmJob{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindPrewarmJob},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        rec.Generation,
			CreationTimestamp: rec.CreatedAt,
			DeletionTimestamp: rec.DeletedAt,
		},
		Spec: corev1.PrewarmJobSpec{
			NodeNames: append([]string(nil), rec.NodeNames...),
			Artifacts: append([]corev1.ArtifactRef(nil), rec.Artifacts...),
			Images:    append([]string(nil), rec.Images...),
		},
		Status: corev1.PrewarmJobStatus{Phase: corev1.PrewarmJobPhasePending},
	}
	if len(rec.NodeSelector) > 0 {
		out.Spec.NodeSelector = pgutil.NonNilMap(rec.NodeSelector)
	}
	if hasState {
		out.Metadata.StatusVersion = pgutil.FormatVersion(st.Version)
		out.Status = corev1.PrewarmJobStatus{
			ObservedGeneration: st.ObservedGeneration,
			Phase:              corev1.PrewarmJobPhase(st.Phase),
			TargetNodes:        st.TargetNodes,
			ReadyNodes:         st.ReadyNodes,
			FailedNodes:        st.FailedNodes,
			Nodes:              append([]corev1.PrewarmNodeStatus(nil), st.Nodes...),
			Conditions:         append([]metav1.Condition(nil), st.Conditions...),
		}
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
	return out
}

func (s *DB) Create(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
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
	rec, st := toJobRecords(&cp, 1, 1)
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	if err := tx.Create(&st).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return fromJobRecords(rec, st, true), nil
}

func (s *DB) Get(ctx context.Context, name string) (*corev1.PrewarmJob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.PrewarmJobRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var st corev1.PrewarmJobStateRecord
	err := s.orm.WithContext(ctx).Where("job_uid = ?", rec.UID).Take(&st).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromJobRecords(rec, st, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.PrewarmJob], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.PrewarmJobRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("prewarm_jobs.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "prewarm_jobs.name", val)
		case "status.phase":
			tx = tx.Joins("JOIN prewarm_job_states ON prewarm_job_states.job_uid = prewarm_jobs.uid").
				Where("prewarm_job_states.phase = ?", val)
		default:
			return nil, fmt.Errorf("unsupported prewarmjob fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "prewarm_jobs.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.PrewarmJobRecord](tx, "prewarm_jobs.created_at DESC, prewarm_jobs.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateJobList(ctx, rows, total, opts.LabelSelector)
}

func (s *DB) hydrateJobList(ctx context.Context, rows []corev1.PrewarmJobRecord, total int, sel *metav1.LabelSelector) (*metav1.List[*corev1.PrewarmJob], error) {
	stMap := make(map[string]corev1.PrewarmJobStateRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var states []corev1.PrewarmJobStateRecord
		if err := s.orm.WithContext(ctx).Where("job_uid IN ?", uids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, st := range states {
			stMap[st.JobUID] = st
		}
	}
	items := make([]*corev1.PrewarmJob, 0, len(rows))
	for _, row := range rows {
		st, ok := stMap[row.UID]
		item := fromJobRecords(row, st, ok)
		if sel != nil && !sel.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*corev1.PrewarmJob]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "PrewarmJobList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	return s.updateSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	return s.updateSpec(ctx, in, true)
}

func (s *DB) updateSpec(ctx context.Context, in *corev1.PrewarmJob, requireCAS bool) (*corev1.PrewarmJob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	if requireCAS && (in.Metadata.UID == "" || in.Metadata.ResourceVersion == "") {
		return nil, db.ErrConflict
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, cp.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (cp.Metadata.UID != "" && cp.Metadata.UID != existing.Metadata.UID) ||
		(cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if !reflect.DeepEqual(existing.Spec, cp.Spec) {
		cp.Metadata.Generation++
	}
	return s.commitSpecUpdate(ctx, &cp, oldRV, oldRV+1)
}

func (s *DB) commitSpecUpdate(ctx context.Context, cp *corev1.PrewarmJob, oldRV, newRV int64) (*corev1.PrewarmJob, error) {
	rec, _ := toJobRecords(cp, newRV, 1)
	namesJSON, _ := pgutil.MarshalJSONCol(rec.NodeNames)
	selJSON, _ := pgutil.MarshalJSONCol(rec.NodeSelector)
	artJSON, _ := pgutil.MarshalJSONCol(rec.Artifacts)
	imgJSON, _ := pgutil.MarshalJSONCol(rec.Images)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&corev1.PrewarmJobRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"node_names": namesJSON, "node_selector": selJSON, "artifacts": artJSON, "images": imgJSON,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "prewarm_jobs", "name = ?", []any{name}, expectedUID)
}
