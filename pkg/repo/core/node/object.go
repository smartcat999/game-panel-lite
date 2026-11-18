package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func nonNilResourceMap(in corev1.ResourceList) corev1.ResourceList {
	if in == nil {
		return corev1.ResourceList{}
	}
	cp := make(corev1.ResourceList, len(in))
	for k, v := range in {
		cp[k] = v
	}
	return cp
}

func nonNilItems[T any](in []T) []T {
	if len(in) == 0 {
		return []T{}
	}
	return append([]T(nil), in...)
}

func toNodeRecords(in *corev1.Node, rv, sv int64) (corev1.NodeRecord, corev1.NodeHeartbeatRecord) {
	phase := in.Status.Phase
	if phase == "" {
		phase = corev1.NodePhaseReady
	}
	rec := corev1.NodeRecord{
		Name:            in.Metadata.Name,
		UID:             in.Metadata.UID,
		Unschedulable:   in.Spec.Unschedulable,
		ExternalIP:      in.Spec.ExternalIP,
		AllowedApps:     pgutil.NonNilStrings(in.Spec.AllowedApps),
		ResourceVersion: rv,
		Generation:      in.Metadata.Generation,
		CreatedAt:       in.Metadata.CreationTimestamp,
		DeletedAt:       in.Metadata.DeletionTimestamp,
		Finalizers:      pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:          pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:     pgutil.NonNilMap(in.Metadata.Annotations),
	}
	hb := corev1.NodeHeartbeatRecord{
		NodeUID:            in.Metadata.UID,
		NodeName:           in.Metadata.Name,
		Version:            sv,
		ObservedGeneration: in.Status.ObservedGeneration,
		Phase:              string(phase),
		Addresses:          nonNilItems(in.Status.Addresses),
		Runtimes:           nonNilItems(in.Status.Runtimes),
		Capacity:           nonNilResourceMap(in.Status.Capacity),
		Allocatable:        nonNilResourceMap(in.Status.Allocatable),
		Usage:              in.Status.Usage,
		CachedArtifacts:    pgutil.NonNilStrings(in.Status.CachedArtifacts),
		CachedBlobs:        pgutil.NonNilStrings(in.Status.CachedBlobs),
		WarmImages:         pgutil.NonNilStrings(in.Status.WarmImages),
		LastHeartbeatTime:  in.Status.LastHeartbeatTime,
		Conditions:         pgutil.NonNilConditions(in.Status.Conditions),
		UpdatedAt:          time.Now().UTC(),
	}
	return rec, hb
}

func fromNodeRecords(rec corev1.NodeRecord, hb corev1.NodeHeartbeatRecord, hasHB bool) *corev1.Node {
	out := &corev1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Node"},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        rec.Generation,
			CreationTimestamp: rec.CreatedAt,
			DeletionTimestamp: rec.DeletedAt,
		},
		Spec:   corev1.NodeSpec{Unschedulable: rec.Unschedulable, ExternalIP: rec.ExternalIP},
		Status: corev1.NodeStatus{Phase: corev1.NodePhaseReady},
	}
	if hasHB {
		out.Metadata.StatusVersion = pgutil.FormatVersion(hb.Version)
		out.Status = corev1.NodeStatus{
			ObservedGeneration: hb.ObservedGeneration,
			Phase:              corev1.NodePhase(hb.Phase),
			Addresses:          append([]corev1.NodeAddress(nil), hb.Addresses...),
			Runtimes:           append([]corev1.NodeRuntime(nil), hb.Runtimes...),
			Usage:              hb.Usage,
			CachedArtifacts:    append([]string(nil), hb.CachedArtifacts...),
			CachedBlobs:        append([]string(nil), hb.CachedBlobs...),
			WarmImages:         append([]string(nil), hb.WarmImages...),
			LastHeartbeatTime:  hb.LastHeartbeatTime,
			Conditions:         append([]metav1.Condition(nil), hb.Conditions...),
		}
		if len(hb.Capacity) > 0 {
			out.Status.Capacity = nonNilResourceMap(hb.Capacity)
		}
		if len(hb.Allocatable) > 0 {
			out.Status.Allocatable = nonNilResourceMap(hb.Allocatable)
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
	if len(rec.AllowedApps) > 0 {
		out.Spec.AllowedApps = append([]string(nil), rec.AllowedApps...)
	}
	return out
}

func (s *DB) Create(ctx context.Context, in *corev1.Node) (*corev1.Node, error) {
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
	rec, hb := toNodeRecords(&cp, 1, 1)
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
	if err := tx.Create(&hb).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return fromNodeRecords(rec, hb, true), nil
}

func (s *DB) Get(ctx context.Context, name string) (*corev1.Node, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.NodeRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var hb corev1.NodeHeartbeatRecord
	err := s.orm.WithContext(ctx).Where("node_uid = ?", rec.UID).Take(&hb).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromNodeRecords(rec, hb, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Node], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.NodeRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("nodes.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "nodes.name", val)
		case "spec.unschedulable":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean fieldSelector %s=%q", field, val)
			}
			tx = tx.Where("nodes.unschedulable = ?", b)
		case "status.phase":
			tx = tx.Joins("JOIN node_heartbeats ON node_heartbeats.node_uid = nodes.uid").Where("node_heartbeats.phase = ?", val)
		default:
			return nil, fmt.Errorf("unsupported node fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "nodes.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.NodeRecord](tx, "nodes.created_at DESC, nodes.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateNodeList(ctx, rows, total, opts.LabelSelector)
}

func (s *DB) hydrateNodeList(ctx context.Context, rows []corev1.NodeRecord, total int, sel *metav1.LabelSelector) (*metav1.List[*corev1.Node], error) {
	hbMap := make(map[string]corev1.NodeHeartbeatRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var hbs []corev1.NodeHeartbeatRecord
		if err := s.orm.WithContext(ctx).Where("node_uid IN ?", uids).Find(&hbs).Error; err != nil {
			return nil, err
		}
		for _, hb := range hbs {
			hbMap[hb.NodeUID] = hb
		}
	}
	items := make([]*corev1.Node, 0, len(rows))
	for _, row := range rows {
		hb, ok := hbMap[row.UID]
		item := fromNodeRecords(row, hb, ok)
		if sel != nil && !sel.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*corev1.Node]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "NodeList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "nodes", "name = ?", []any{name}, expectedUID)
}
