package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// MergeOrCreateWebhookBatch atomically coalesces an incoming notification into an open
// WebhookPending batch record for (auditTenant, scope, channelID) via optimistic CAS,
// or creates a fresh pending record if no open batch window exists.
func (s *DB) MergeOrCreateWebhookBatch(
	ctx context.Context,
	auditTenant, scope, channelID string,
	now time.Time,
	mergeFn func(existing *corev1.Event) (*corev1.Event, bool),
	buildNewFn func() *corev1.Event,
) (*corev1.Event, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("audit DB is nil")
	}
	if mergeFn != nil && strings.TrimSpace(channelID) != "" {
		for range 3 {
			merged, done, err := s.tryMergeOpenBatch(ctx, auditTenant, scope, channelID, now, mergeFn)
			if err != nil {
				return nil, err
			}
			if done {
				return merged, nil
			}
		}
	}
	fresh := buildNewFn()
	if fresh == nil {
		return nil, errors.New("webhook batch event is nil")
	}
	if err := s.Record(ctx, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

func (s *DB) tryMergeOpenBatch(
	ctx context.Context,
	auditTenant, scope, channelID string,
	now time.Time,
	mergeFn func(existing *corev1.Event) (*corev1.Event, bool),
) (*corev1.Event, bool, error) {
	var rec corev1.AuditEventRecord
	err := s.orm.WithContext(ctx).
		Where("tenant = ? AND resource = ? AND subresource = ? AND resource_name = ? AND action = ?",
			auditTenant, "alerts", "webhook", channelID, corev1.WebhookActionPending).
		Order("created_at DESC, id DESC").
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var cur corev1.Event
	if json.Unmarshal(rec.Payload, &cur) != nil || !isBatchOpenForMerge(&cur, scope, now) {
		return nil, false, nil
	}
	merged, ok := mergeFn(&cur)
	if !ok || merged == nil {
		return nil, false, nil
	}
	payload, err := json.Marshal(merged)
	if err != nil {
		return nil, false, err
	}
	res := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
		Where("id = ? AND action = ? AND payload = ?", rec.ID, corev1.WebhookActionPending, rec.Payload).
		Updates(map[string]any{"target_name": merged.TargetName, "payload": payload})
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected > 0 {
		return merged, true, nil
	}
	return nil, false, nil
}

func isBatchOpenForMerge(ev *corev1.Event, scope string, now time.Time) bool {
	if ev == nil || ev.Attributes == nil || parseWebhookAttempts(ev) != 0 {
		return false
	}
	if strings.TrimSpace(ev.Attributes["scope"]) != scope {
		return false
	}
	notBefore, ok := parseWebhookNotBefore(ev)
	return ok && now.Before(notBefore)
}

// ClaimWebhookDelivery verifies that a notification record is due and atomically transitions
// it to WebhookSending via CAS. If the record is still within its group_wait window (now < notBefore),
// waitRemaining > 0 and claimed == false are returned so the caller can schedule a wakeup at notBefore.
func (s *DB) ClaimWebhookDelivery(
	ctx context.Context,
	id string,
	fromResync bool,
	now time.Time,
	leaseTTL time.Duration,
) (*corev1.Event, int, time.Duration, bool, error) {
	if s == nil || s.orm == nil || strings.TrimSpace(id) == "" {
		return nil, 0, 0, false, nil
	}
	var rec corev1.AuditEventRecord
	err := s.orm.WithContext(ctx).
		Where("id = ? AND resource = ? AND subresource = ?", id, "alerts", "webhook").
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, 0, false, nil
		}
		return nil, 0, 0, false, err
	}
	var ev corev1.Event
	if json.Unmarshal(rec.Payload, &ev) != nil {
		return nil, 0, 0, false, nil
	}
	if rec.Action == corev1.WebhookActionPending {
		if notBefore, ok := parseWebhookNotBefore(&ev); ok && now.Before(notBefore) {
			return nil, 0, notBefore.Sub(now), false, nil
		}
	}
	if !matchesWebhookDeliveryPrecondition(rec.Action, &ev, rec.CreatedAt, fromResync, now) {
		return nil, 0, 0, false, nil
	}
	return s.casClaimWebhookRecord(ctx, &rec, &ev, now, leaseTTL)
}

func (s *DB) casClaimWebhookRecord(
	ctx context.Context,
	rec *corev1.AuditEventRecord,
	ev *corev1.Event,
	now time.Time,
	leaseTTL time.Duration,
) (*corev1.Event, int, time.Duration, bool, error) {
	attempt := parseWebhookAttempts(ev) + 1
	if ev.Attributes == nil {
		ev.Attributes = make(map[string]string)
	}
	ev.Action = corev1.WebhookActionSending
	ev.Attributes["attempts"] = strconv.Itoa(attempt)
	ev.Attributes["leaseUntil"] = now.Add(leaseTTL).Format(time.RFC3339)
	payload, err := json.Marshal(ev)
	if err != nil {
		return nil, 0, 0, false, err
	}
	res := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
		Where("id = ? AND action = ? AND payload = ?", rec.ID, rec.Action, rec.Payload).
		Updates(map[string]any{"action": corev1.WebhookActionSending, "payload": payload})
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, 0, 0, false, res.Error
	}
	return ev, attempt, 0, true, nil
}

// CompleteWebhookAttempt atomically commits the outcome of a claimed webhook delivery attempt
// only if the record is still owned by the given attempt number.
func (s *DB) CompleteWebhookAttempt(ctx context.Context, ev *corev1.Event, attempt int) (bool, error) {
	if s == nil || s.orm == nil || ev == nil || ev.Metadata.Name == "" {
		return false, nil
	}
	var rec corev1.AuditEventRecord
	err := s.orm.WithContext(ctx).
		Where("id = ? AND action = ?", ev.Metadata.Name, corev1.WebhookActionSending).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var cur corev1.Event
	if json.Unmarshal(rec.Payload, &cur) != nil || parseWebhookAttempts(&cur) != attempt {
		return false, nil
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return false, err
	}
	res := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
		Where("id = ? AND action = ? AND payload = ?", rec.ID, rec.Action, rec.Payload).
		Updates(map[string]any{"action": ev.Action, "status_code": ev.StatusCode, "payload": payload})
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// ListDueWebhookDeliveries returns undelivered notification records eligible for Resync delivery or retry.
func (s *DB) ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]*corev1.Event, error) {
	if s == nil || s.orm == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	var records []corev1.AuditEventRecord
	err := s.orm.WithContext(ctx).
		Where("resource = ? AND subresource = ? AND action IN ?",
			"alerts", "webhook",
			[]string{corev1.WebhookActionPending, corev1.WebhookActionRetrying, corev1.WebhookActionSending}).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	var due []*corev1.Event
	for _, rec := range records {
		var ev corev1.Event
		if json.Unmarshal(rec.Payload, &ev) != nil {
			continue
		}
		if matchesWebhookDeliveryPrecondition(rec.Action, &ev, rec.CreatedAt, true, now) {
			due = append(due, &ev)
		}
	}
	return due, nil
}

func matchesWebhookDeliveryPrecondition(action string, ev *corev1.Event, createdAt time.Time, fromResync bool, now time.Time) bool {
	if ev == nil || action == corev1.WebhookActionDelivered || action == corev1.WebhookActionDeliveryFailed {
		return false
	}
	if leaseStr := strings.TrimSpace(ev.Attributes["leaseUntil"]); leaseStr != "" {
		if leaseTime, err := time.Parse(time.RFC3339, leaseStr); err == nil && now.Before(leaseTime) {
			return false
		}
	}
	if !fromResync {
		if action == corev1.WebhookActionPending && parseWebhookAttempts(ev) == 0 {
			return true
		}
		if action == corev1.WebhookActionRetrying {
			return isRetryDue(ev, now)
		}
		return false
	}
	switch action {
	case corev1.WebhookActionRetrying:
		return isRetryDue(ev, now)
	case corev1.WebhookActionPending:
		if notBefore, ok := parseWebhookNotBefore(ev); ok {
			return !now.Before(notBefore.Add(5 * time.Second))
		}
		return !now.Before(createdAt.Add(time.Minute))
	case corev1.WebhookActionSending:
		return true
	default:
		return false
	}
}

func isRetryDue(ev *corev1.Event, now time.Time) bool {
	nextStr := strings.TrimSpace(ev.Attributes["nextRetryAt"])
	if nextStr == "" {
		return true
	}
	nextTime, err := time.Parse(time.RFC3339, nextStr)
	return err != nil || !now.Before(nextTime)
}

func parseWebhookNotBefore(ev *corev1.Event) (time.Time, bool) {
	if ev == nil || ev.Attributes == nil {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(ev.Attributes["notBefore"])
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func parseWebhookAttempts(ev *corev1.Event) int {
	if ev == nil || ev.Attributes == nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(ev.Attributes["attempts"]))
	if n < 0 {
		return 0
	}
	return n
}

// ListRecentAlertTransitions returns the latest AlertTransition records across the given
// tenant scopes (pass "" inside tenants to include cluster-level platform alerts).
func (s *DB) ListRecentAlertTransitions(ctx context.Context, tenants []string, since time.Time, limit int) ([]*corev1.Event, error) {
	if s == nil || s.orm == nil || len(tenants) == 0 {
		return nil, nil
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
		Select("payload").
		Where("resource = ? AND subresource = ? AND action = ? AND tenant IN ?",
			"alerts", "state", corev1.AlertActionTransition, tenants)
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since.UTC())
	}
	var records []corev1.AuditEventRecord
	if err := tx.Order("created_at DESC, id DESC").Limit(auditLimit(limit)).Find(&records).Error; err != nil {
		return nil, err
	}
	return decodeAuditRecords(records, nil)
}

// ListRecentTaskFailures returns the latest asynchronous background task failure records
// (backup archive, backup restore, scheduled tasks) across the given workspace tenants.
func (s *DB) ListRecentTaskFailures(ctx context.Context, tenants []string, since time.Time, limit int) ([]*corev1.Event, error) {
	if s == nil || s.orm == nil || len(tenants) == 0 {
		return nil, nil
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.AuditEventRecord{}).
		Select("payload").
		Where("resource = ? AND subresource = ? AND action = ? AND tenant IN ?",
			"instances", "tasks", corev1.TaskActionFailed, tenants)
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since.UTC())
	}
	var records []corev1.AuditEventRecord
	if err := tx.Order("created_at DESC, id DESC").Limit(auditLimit(limit)).Find(&records).Error; err != nil {
		return nil, err
	}
	return decodeAuditRecords(records, nil)
}
