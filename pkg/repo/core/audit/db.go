// Package audit implements append-only persistence, indexed querying, window checkpoint queries,
// and retention pruning for operational and metering audit events.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

// AuditEventRecord is an alias for corev1.AuditEventRecord.
type AuditEventRecord = corev1.AuditEventRecord

// PruneOptions makes retention an explicit caller decision. Preserved
// resources are excluded from both age and count pruning.
type PruneOptions struct {
	Tenant            string
	MaxCount          int
	MaxAge            time.Duration
	PreserveResources []string
}

// PruneOperational prunes operational audit logs under the configured count and age limits.
func PruneOperational(ctx context.Context, d *DB, tenant string, maxCount int, maxAge time.Duration) error {
	if d == nil {
		return nil
	}
	return d.Prune(ctx, tenant, maxCount, maxAge)
}

// DB manages append-only audit_events persistence using GORM.
type DB struct {
	orm *gorm.DB
}

// NewDB creates a PostgreSQL-backed audit DB.
func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func (s *DB) Record(ctx context.Context, ev *corev1.Event) error {
	_, err := s.RecordOnce(ctx, ev)
	return err
}

// RecordOnce inserts ev once keyed on Metadata.Name (id) and reports whether this call inserted a new row.
func (s *DB) RecordOnce(ctx context.Context, ev *corev1.Event) (bool, error) {
	if s == nil || s.orm == nil || ev == nil {
		return false, errors.New("audit event is nil")
	}
	ev.DefaultMeta()
	if ev.Metadata.Name == "" {
		ev.Metadata.Name = db.NewUID()
	}
	if ev.Metadata.UID == "" {
		ev.Metadata.UID = ev.Metadata.Name
	}
	if ev.Metadata.CreationTimestamp.IsZero() {
		if !ev.Timestamp.IsZero() {
			ev.Metadata.CreationTimestamp = ev.Timestamp.UTC()
		} else {
			ev.Metadata.CreationTimestamp = time.Now().UTC()
		}
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = ev.Metadata.CreationTimestamp
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return false, err
	}
	rec := corev1.AuditEventRecord{
		ID: ev.Metadata.Name, Tenant: ev.Metadata.Tenant, Actor: ev.Actor,
		Verb: ev.Verb, APIGroup: ev.APIGroup, Resource: ev.Resource,
		Subresource: ev.Subresource, ResourceName: ev.ResourceName, Action: ev.Action,
		TargetName: ev.TargetName, StatusCode: ev.StatusCode, ClientIP: ev.ClientIP,
		RequestID: ev.RequestID, CreatedAt: ev.Timestamp.UTC(), Payload: payload,
	}
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&rec)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// Save upserts a corev1.Event record (updating action, status_code, and payload on conflict).
func (s *DB) Save(ctx context.Context, ev *corev1.Event) error {
	if s == nil || s.orm == nil || ev == nil {
		return errors.New("audit event is nil")
	}
	ev.DefaultMeta()
	if ev.Metadata.Name == "" {
		ev.Metadata.Name = db.NewUID()
	}
	if ev.Metadata.UID == "" {
		ev.Metadata.UID = ev.Metadata.Name
	}
	if ev.Metadata.CreationTimestamp.IsZero() {
		ev.Metadata.CreationTimestamp = time.Now().UTC()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = ev.Metadata.CreationTimestamp
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	rec := corev1.AuditEventRecord{
		ID:           ev.Metadata.Name,
		Tenant:       ev.Metadata.Tenant,
		Actor:        ev.Actor,
		Verb:         ev.Verb,
		APIGroup:     ev.APIGroup,
		Resource:     ev.Resource,
		Subresource:  ev.Subresource,
		ResourceName: ev.ResourceName,
		Action:       ev.Action,
		TargetName:   ev.TargetName,
		StatusCode:   ev.StatusCode,
		ClientIP:     ev.ClientIP,
		RequestID:    ev.RequestID,
		CreatedAt:    ev.Metadata.CreationTimestamp.UTC(),
		Payload:      payload,
	}
	return s.orm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"action", "status_code", "payload"}),
	}).Create(&rec).Error
}

func (s *DB) Delete(ctx context.Context, tenant, name string) error {
	q := s.orm.WithContext(ctx).Where("id = ?", name)
	if tenant != "" {
		q = q.Where("tenant = ?", tenant)
	}
	return q.Delete(&corev1.AuditEventRecord{}).Error
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Event], error) {
	baseTx, err := applyAuditWhere(s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}), opts)
	if err != nil {
		return nil, err
	}
	var total int64
	if !opts.SkipTotalCount {
		if err := baseTx.Count(&total).Error; err != nil {
			return nil, err
		}
	}
	queryTx := baseTx.Session(&gorm.Session{})
	offset := max(opts.Offset, 0)
	limit := auditLimit(opts.Limit)
	var records []corev1.AuditEventRecord
	err = queryTx.Select("payload").Order("created_at DESC, id DESC").
		Limit(limit).Offset(offset).Find(&records).Error
	if err != nil {
		return nil, err
	}
	items, err := decodeAuditRecords(records, nil)
	if err != nil {
		return nil, err
	}
	return &metav1.List[*corev1.Event]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "EventList"},
		Metadata: metav1.ListMeta{TotalCount: int(total)},
		Items:    items,
	}, nil
}

func decodeAuditRecords(records []corev1.AuditEventRecord, labelSel *metav1.LabelSelector) ([]*corev1.Event, error) {
	items := make([]*corev1.Event, 0, len(records))
	for _, rec := range records {
		var event corev1.Event
		if err := json.Unmarshal(rec.Payload, &event); err != nil {
			return nil, err
		}
		if labelSel == nil || labelSel.Matches(event.Metadata.Labels) {
			items = append(items, &event)
		}
	}
	return items, nil
}

func (s *DB) Prune(ctx context.Context, tenant string, maxCount int, maxAge time.Duration) error {
	return s.PruneWithOptions(ctx, PruneOptions{Tenant: tenant, MaxCount: maxCount, MaxAge: maxAge})
}

func (s *DB) PruneWithOptions(ctx context.Context, opts PruneOptions) error {
	if opts.MaxAge > 0 {
		q := s.orm.WithContext(ctx).Where("created_at < ?", time.Now().UTC().Add(-opts.MaxAge))
		if opts.Tenant != "" {
			q = q.Where("tenant = ?", opts.Tenant)
		}
		if len(opts.PreserveResources) > 0 {
			q = q.Where("resource NOT IN ?", opts.PreserveResources)
		}
		if err := q.Delete(&corev1.AuditEventRecord{}).Error; err != nil {
			return err
		}
	}
	if opts.MaxCount > 0 {
		sub := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
			Select("id, ROW_NUMBER() OVER (PARTITION BY tenant ORDER BY created_at DESC, id DESC) AS position")
		if opts.Tenant != "" {
			sub = sub.Where("tenant = ?", opts.Tenant)
		}
		if len(opts.PreserveResources) > 0 {
			sub = sub.Where("resource NOT IN ?", opts.PreserveResources)
		}
		ranked := s.orm.WithContext(ctx).Table("(?) AS ranked", sub).Select("id").Where("position > ?", opts.MaxCount)
		if err := s.orm.WithContext(ctx).Where("id IN (?)", ranked).Delete(&corev1.AuditEventRecord{}).Error; err != nil {
			return err
		}
	}
	return nil
}

type auditField struct{ path, literal, value string }
type auditSelection struct {
	fields []auditField
	since  time.Time
}

func compileAuditSelection(opts db.ListOptions) (auditSelection, error) {
	var selection auditSelection
	keys := make([]string, 0, len(opts.FieldSelector))
	for path := range opts.FieldSelector {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		literal, err := db.JSONFieldPathLiteral(path)
		if err != nil {
			return selection, err
		}
		value := opts.FieldSelector[path]
		if path == "since" || path == "spec.since" {
			if strings.TrimSpace(value) != "" {
				selection.since, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
				if err != nil {
					return selection, fmt.Errorf("invalid audit since time: %w", err)
				}
			}
			continue
		}
		if (path == "actor" || path == "spec.actor" || path == "resource" || path == "spec.resource" || path == "verb" || path == "spec.verb") && value == "" {
			continue
		}
		selection.fields = append(selection.fields, auditField{path: path, literal: literal, value: value})
	}
	return selection, nil
}

func applyAuditWhere(tx *gorm.DB, opts db.ListOptions) (*gorm.DB, error) {
	selection, err := compileAuditSelection(opts)
	if err != nil {
		return nil, err
	}
	if opts.Tenant != "" {
		tx = tx.Where("tenant = ?", opts.Tenant)
	}
	for _, field := range selection.fields {
		switch field.path {
		case "actor", "spec.actor":
			tx = tx.Where("actor = ?", field.value)
		case "resource", "spec.resource":
			tx = tx.Where("resource = ?", field.value)
		case "verb", "spec.verb":
			tx = tx.Where("verb = ?", field.value)
		default:
			tx = tx.Where("(payload #>> "+field.literal+") = ?", field.value)
		}
	}
	if !selection.since.IsZero() {
		tx = tx.Where("created_at >= ?", selection.since.UTC())
	}
	return applyAuditLabelWhere(tx, opts.LabelSelector), nil
}

func applyAuditLabelClause(tx *gorm.DB, labels, key string, values []string, negate bool) *gorm.DB {
	if len(values) == 0 {
		if negate {
			return tx.Where("TRUE")
		}
		return tx.Where("FALSE")
	}
	var alternatives []string
	var args []any
	for _, value := range values {
		if value == "*" {
			alternatives = append(alternatives, fmt.Sprintf("jsonb_exists(%s, ?)", labels))
			args = append(args, key)
		} else if strings.Contains(value, "*") {
			alternatives = append(alternatives, fmt.Sprintf("(%s ->> ? LIKE ? ESCAPE '\\')", labels))
			args = append(args, key, db.WildcardToSQLLike(value))
		} else {
			alternatives = append(alternatives, fmt.Sprintf("(%s ->> ? = ?)", labels))
			args = append(args, key, value)
		}
	}
	clauseStr := "(" + strings.Join(alternatives, " OR ") + ")"
	if negate {
		clauseStr = "NOT COALESCE(" + clauseStr + ", FALSE)"
	}
	return tx.Where(clauseStr, args...)
}

func applyAuditLabelWhere(tx *gorm.DB, selector *metav1.LabelSelector) *gorm.DB {
	if selector == nil {
		return tx
	}
	labels := "COALESCE(payload #> '{metadata,labels}','{}'::jsonb)"
	keys := make([]string, 0, len(selector.MatchLabels))
	for key := range selector.MatchLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		tx = applyAuditLabelClause(tx, labels, key, []string{selector.MatchLabels[key]}, false)
	}
	for _, expression := range selector.MatchExpressions {
		switch expression.Operator {
		case metav1.LabelSelectorOpExists, "exists":
			tx = tx.Where(fmt.Sprintf("jsonb_exists(%s, ?)", labels), expression.Key)
		case metav1.LabelSelectorOpDoesNotExist, "doesnotexist":
			tx = tx.Where(fmt.Sprintf("NOT jsonb_exists(%s, ?)", labels), expression.Key)
		case metav1.LabelSelectorOpIn, "in", "=":
			tx = applyAuditLabelClause(tx, labels, expression.Key, expression.Values, false)
		case metav1.LabelSelectorOpNotIn, "notin", "!=":
			tx = applyAuditLabelClause(tx, labels, expression.Key, expression.Values, true)
		default:
			tx = tx.Where("FALSE")
		}
	}
	return tx
}

func auditLimit(value int) int {
	if value <= 0 {
		return 100
	}
	if value > 10000 {
		return 10000
	}
	return value
}
