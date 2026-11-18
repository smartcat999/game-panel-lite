package alert

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

const (
	dedupKeyPrefix           = "alert:dedup:"
	dedupCooldownTTL         = 5 * time.Minute
	defaultGroupWaitDuration = 30 * time.Second
	deliveryLeaseDuration    = 15 * time.Second
	maxDeliveryAttempts      = 3
)

var groupWaitOverride *time.Duration

func effectiveGroupWait(state string) time.Duration {
	if state == corev1.AlertStateTest {
		return 0
	}
	if groupWaitOverride != nil {
		return *groupWaitOverride
	}
	return defaultGroupWaitDuration
}

type notificationItem = driver.NotificationItem

func shouldSuppressTransition(carrierScope, channelID, alertKey, state string) bool {
	if state == corev1.AlertStateTest {
		return false
	}
	store := activeStateStore()
	if store == nil {
		return false
	}
	identity := carrierScope + "|" + channelID + "|" + strings.TrimSpace(alertKey) + "|" + state
	sum := sha256.Sum256([]byte(identity))
	key := dedupKeyPrefix + hex.EncodeToString(sum[:12])
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if val, err := store.Get(ctx, key); err == nil && val != "" {
		return true
	}
	_ = store.Set(ctx, key, "1", dedupCooldownTTL)
	return false
}

func enqueueTenantEvent(ctx context.Context, reg *repo.Registry, bus mq.Interface, t *corev1.Tenant, ev alertEvent, targetChannelID string) {
	if t == nil {
		return
	}
	now := ev.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if targetChannelID == "" && ev.State != corev1.AlertStateTest && IsAlertEventSilenced(ctx, reg, t, ev, now) {
		if ev.State == corev1.AlertStateResolved {
			recordAlertStateAuditEvent(ctx, reg, t.Metadata.Name, t.Metadata.UID, ev)
			clearResolvedTargetSilencesIfHealthy(ctx, reg, bus, ev, now)
		}
		return
	}
	if targetChannelID == "" && ev.State != corev1.AlertStateTest {
		recordAlertStateAuditEvent(ctx, reg, t.Metadata.Name, t.Metadata.UID, ev)
	}
	enqueueAcrossChannels(ctx, reg, bus, t.Metadata.Name, t.Metadata.UID, corev1.AlertScopeTenant, ExtractTenantChannels(t), ev, targetChannelID)
	if targetChannelID == "" {
		clearResolvedTargetSilencesIfHealthy(ctx, reg, bus, ev, now)
	}
}

func enqueuePlatformEvent(ctx context.Context, reg *repo.Registry, bus mq.Interface, carrier *corev1.Tenant, ev alertEvent, targetChannelID string) {
	if carrier == nil {
		return
	}
	enqueueAcrossChannels(ctx, reg, bus, carrier.Metadata.Name, carrier.Metadata.UID, corev1.AlertScopePlatform, ExtractPlatformChannels(carrier), ev, targetChannelID)
}

func enqueueAllPlatformCarriers(ctx context.Context, reg *repo.Registry, tenantDB *tenantdb.DB, bus mq.Interface, ev alertEvent) {
	now := ev.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if ev.State != corev1.AlertStateTest && IsAlertEventSilenced(ctx, reg, nil, ev, now) {
		if ev.State == corev1.AlertStateResolved {
			recordAlertStateAuditEvent(ctx, reg, "", "", ev)
			clearResolvedTargetSilencesIfHealthy(ctx, reg, bus, ev, now)
		}
		return
	}
	recordAlertStateAuditEvent(ctx, reg, "", "", ev)
	if tenantDB == nil {
		return
	}
	tList, err := tenantDB.List(ctx, db.ListOptions{})
	if err != nil || tList == nil {
		return
	}
	for _, t := range tList.Items {
		if t != nil && len(ExtractPlatformChannels(t)) > 0 {
			enqueuePlatformEvent(ctx, reg, bus, t, ev, "")
		}
	}
	clearResolvedTargetSilencesIfHealthy(ctx, reg, bus, ev, now)
}

func enqueueAcrossChannels(ctx context.Context, reg *repo.Registry, bus mq.Interface, carrierName, carrierUID, scope string, channels []corev1.AlertChannel, ev alertEvent, targetChannelID string) {
	item := toNotificationItem(ev)
	for _, ch := range channels {
		if !channelMatchesFilter(ch, ev.Severity, ev.State, targetChannelID) {
			continue
		}
		if shouldSuppressTransition(scope+":"+carrierName, ch.ID, ev.AlertKey, ev.State) {
			continue
		}
		if publishOrBatchChannelItem(ctx, reg, bus, carrierName, carrierUID, scope, ch, item) {
			continue
		}
		dispatchInMemoryFallback(ch, scope, carrierName, item)
	}
}

func channelMatchesFilter(ch corev1.AlertChannel, severity, state, targetChannelID string) bool {
	if targetChannelID != "" {
		return ch.ID == targetChannelID
	}
	if !ch.Enabled {
		return false
	}
	if state != corev1.AlertStateResolved && state != corev1.AlertStateTest {
		if ch.Severity == corev1.AlertSeverityCritical && !strings.EqualFold(severity, corev1.AlertSeverityCritical) {
			return false
		}
	}
	return true
}

func publishOrBatchChannelItem(ctx context.Context, reg *repo.Registry, bus mq.Interface, carrierName, carrierUID, scope string, ch corev1.AlertChannel, item notificationItem) bool {
	if reg == nil || reg.Pool() == nil {
		return false
	}
	now := item.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	auditTenant := carrierName
	if scope == corev1.AlertScopePlatform {
		auditTenant = ""
	}
	waitDur := effectiveGroupWait(item.State)
	auditStore := auditdb.NewDB(reg.Pool())
	var mergeFn func(*corev1.Event) (*corev1.Event, bool)
	if waitDur > 0 {
		mergeFn = func(existing *corev1.Event) (*corev1.Event, bool) {
			return mergeItemIntoBatchEvent(existing, item), true
		}
	}
	batchEv, err := auditStore.MergeOrCreateWebhookBatch(ctx, auditTenant, scope, ch.ID, now, mergeFn, func() *corev1.Event {
		return buildFreshBatchEvent(auditTenant, carrierName, carrierUID, scope, ch, item, now, waitDur)
	})
	if err != nil || batchEv == nil {
		return false
	}
	if bus != nil {
		wakeup := mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "AlertNotification", carrierName, batchEv.Metadata.Name, 0, "")
		wakeup.Topic = mq.TopicNotification
		wakeup.Attributes = map[string]string{"deliveryID": batchEv.RequestID}
		if bus.Publish(ctx, mq.TopicNotification, wakeup) == nil {
			return true
		}
	}
	batchEv.Action = corev1.WebhookActionRetrying
	batchEv.Attributes["nextRetryAt"] = now.Add(waitDur).Format(time.RFC3339)
	_ = auditStore.Save(ctx, batchEv)
	return true
}

func buildFreshBatchEvent(auditTenant, carrierName, carrierUID, scope string, ch corev1.AlertChannel, item notificationItem, now time.Time, waitDur time.Duration) *corev1.Event {
	deliveryID := newDeliveryID()
	recordID := "ntf-" + deliveryID
	items := []notificationItem{item}
	title, message, sev, st := summarizeBatchItems(scope, carrierName, items)
	itemsBytes, _ := json.Marshal(items)
	attrs := map[string]string{
		"tenant": carrierName, "scope": scope, "deliveryID": deliveryID,
		"channelID": ch.ID, "channelName": ch.Name, "channelType": ch.Type,
		"url": ch.URL, "secret": ch.Secret, "severity": sev,
		"state": st, "title": title, "message": message,
		"itemsJSON": string(itemsBytes), "attempts": "0",
	}
	if waitDur > 0 {
		attrs["notBefore"] = now.Add(waitDur).Format(time.RFC3339Nano)
	}
	if ch.ConsoleURL != "" {
		attrs["consoleURL"] = ch.ConsoleURL
	}
	if carrierUID != "" {
		attrs["tenantUID"] = carrierUID
	}
	return &corev1.Event{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Event"},
		Metadata: metav1.ObjectMeta{Name: recordID, UID: recordID, Tenant: auditTenant, CreationTimestamp: now},
		Actor:    "system:notification-controller", Verb: "create", APIGroup: corev1.GroupName,
		Resource: "alerts", Subresource: "webhook", ResourceName: ch.ID, Action: corev1.WebhookActionPending,
		TargetName: ch.Name, StatusCode: 102, ClientIP: "127.0.0.1", RequestID: deliveryID, Timestamp: now, Attributes: attrs,
	}
}

func mergeItemIntoBatchEvent(existing *corev1.Event, incoming notificationItem) *corev1.Event {
	cp := *existing
	attrs := make(map[string]string, len(existing.Attributes)+2)
	for k, v := range existing.Attributes {
		attrs[k] = v
	}
	cp.Attributes = attrs
	items := decodeNotificationItems(attrs)
	replaced := false
	for i := range items {
		if items[i].AlertKey != "" && items[i].AlertKey == incoming.AlertKey {
			items[i] = incoming
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, incoming)
	}
	title, message, sev, st := summarizeBatchItems(attrs["scope"], attrs["tenant"], items)
	itemsBytes, _ := json.Marshal(items)
	attrs["itemsJSON"] = string(itemsBytes)
	attrs["title"] = title
	attrs["message"] = message
	attrs["severity"] = sev
	attrs["state"] = st
	return &cp
}

func summarizeBatchItems(scope, tenant string, items []notificationItem) (title, message, severity, state string) {
	if len(items) == 1 {
		it := items[0]
		return formatSingleItemTitle(it), formatSingleItemMessage(it), it.Severity, it.State
	}
	firingCount, resolvedCount := 0, 0
	severity = corev1.AlertSeverityWarning
	lines := make([]string, 0, min(len(items), 5))
	for i, it := range items {
		if it.State == corev1.AlertStateResolved {
			resolvedCount++
		} else {
			firingCount++
			if it.Severity == corev1.AlertSeverityCritical {
				severity = corev1.AlertSeverityCritical
			}
		}
		if i < 5 {
			lines = append(lines, fmt.Sprintf("• [%s] %s (%s): %s", stateShortLabel(it.State), it.TargetName, it.RuleTitle, it.MetricValue))
		}
	}
	state = corev1.AlertStateFiring
	if firingCount == 0 {
		state = corev1.AlertStateResolved
	}
	prefix := fmt.Sprintf("工作区 %s", tenant)
	if scope == corev1.AlertScopePlatform || tenant == "" {
		prefix = "平台集群"
	}
	switch {
	case firingCount == 0:
		title = fmt.Sprintf("%s · %d 项告警已恢复", prefix, resolvedCount)
	case resolvedCount == 0:
		title = fmt.Sprintf("%s · %d 项告警触发", prefix, firingCount)
	default:
		title = fmt.Sprintf("%s · %d 项触发 / %d 项恢复", prefix, firingCount, resolvedCount)
	}
	message = strings.Join(lines, "；")
	return title, message, severity, state
}

func formatSingleItemTitle(it notificationItem) string {
	return driver.FormatSingleItemTitle(it)
}

func formatSingleItemMessage(it notificationItem) string {
	return driver.FormatSingleItemMessage(it)
}

func stateShortLabel(state string) string {
	return driver.StateShortLabel(state)
}

func toNotificationItem(ev alertEvent) notificationItem {
	ts := ev.OccurredAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return notificationItem{
		AlertKey: ev.AlertKey, Scope: ev.Scope, Tenant: ev.Tenant,
		TargetKind: ev.TargetKind, TargetName: ev.TargetName,
		RuleID: ev.RuleID, RuleTitle: ev.RuleTitle,
		Severity: ev.Severity, State: ev.State,
		MetricValue: ev.MetricValue, Summary: ev.Summary, OccurredAt: ts,
	}
}

func decodeNotificationItems(attrs map[string]string) []notificationItem {
	if raw := strings.TrimSpace(attrs["itemsJSON"]); raw != "" {
		var items []notificationItem
		if json.Unmarshal([]byte(raw), &items) == nil && len(items) > 0 {
			return items
		}
	}
	if title := strings.TrimSpace(attrs["title"]); title != "" {
		return []notificationItem{{
			Scope:     attrs["scope"],
			Tenant:    attrs["tenant"],
			RuleTitle: title,
			Severity:  attrs["severity"],
			State:     attrs["state"],
			Summary:   attrs["message"],
		}}
	}
	return nil
}

func recordAlertStateAuditEvent(ctx context.Context, reg *repo.Registry, carrierName, carrierUID string, ev alertEvent) {
	if reg == nil || reg.Pool() == nil {
		return
	}
	now := ev.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	auditTenant := carrierName
	if ev.Scope == corev1.AlertScopePlatform {
		auditTenant = ""
	}
	id := "alt-" + newDeliveryID()
	it := toNotificationItem(ev)
	attrs := map[string]string{
		"tenant": carrierName, "scope": ev.Scope, "severity": ev.Severity, "state": ev.State,
		"alertKey": ev.AlertKey, "ruleID": ev.RuleID, "ruleTitle": ev.RuleTitle,
		"targetKind": ev.TargetKind, "targetName": ev.TargetName, "condition": ev.Condition,
		"metricValue": ev.MetricValue,
		"title":       formatSingleItemTitle(it), "message": formatSingleItemMessage(it),
	}
	if carrierUID != "" {
		attrs["tenantUID"] = carrierUID
	}
	_ = auditdb.NewDB(reg.Pool()).Record(ctx, &corev1.Event{
		TypeMeta:     metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Event"},
		Metadata:     metav1.ObjectMeta{Name: id, UID: id, Tenant: auditTenant, CreationTimestamp: now},
		Actor:        iamv1.PrincipalAlertEvaluator,
		Verb:         iamv1.VerbCreate,
		APIGroup:     corev1.GroupName,
		Resource:     corev1.ResourceAlerts,
		Subresource:  corev1.SubresourceState,
		ResourceName: id,
		Action:       corev1.AlertActionTransition,
		TargetName:   ev.TargetName,
		StatusCode:   200,
		ClientIP:     "127.0.0.1",
		RequestID:    id,
		Timestamp:    now,
		Attributes:   attrs,
	})
}

func dispatchInMemoryFallback(ch corev1.AlertChannel, scope, carrierName string, item notificationItem) {
	dID := newDeliveryID()
	title := formatSingleItemTitle(item)
	msg := formatSingleItemMessage(item)
	go func(c corev1.AlertChannel) {
		for attempt := 1; attempt <= maxDeliveryAttempts; attempt++ {
			if err := deliverBatchSync(context.Background(), c, dID, title, msg, []notificationItem{item}); err == nil {
				return
			} else if attempt < maxDeliveryAttempts {
				time.Sleep(retryBackoffDelay(attempt))
			} else {
				log.Warn("alert webhook fallback delivery failed", "scope", scope, "tenant", carrierName, "channel", c.Name, "error", err)
			}
		}
	}(ch)
}

// ListDueNotificationEvents returns undelivered notification Outbox records eligible for Resync processing.
func ListDueNotificationEvents(ctx context.Context, reg *repo.Registry, now time.Time, limit int) ([]*corev1.Event, error) {
	if reg == nil || reg.Pool() == nil {
		return nil, nil
	}
	return auditdb.NewDB(reg.Pool()).ListDueWebhookDeliveries(ctx, now, limit)
}

// ProcessNotification executes one atomic claim-and-deliver step for an Outbox notification record.
// When retryAfter > 0 is returned, the record is buffered in its group_wait window or scheduled for backoff retry.
func ProcessNotification(ctx context.Context, reg *repo.Registry, bus mq.Interface, recordID string, fromResync bool) (time.Duration, error) {
	return processNotificationAt(ctx, reg, bus, recordID, fromResync, time.Now().UTC())
}

func processNotificationAt(ctx context.Context, reg *repo.Registry, bus mq.Interface, recordID string, fromResync bool, now time.Time) (time.Duration, error) {
	if reg == nil || reg.Pool() == nil || strings.TrimSpace(recordID) == "" {
		return 0, nil
	}
	auditDB := auditdb.NewDB(reg.Pool())
	record, attempt, waitRem, claimed, err := auditDB.ClaimWebhookDelivery(ctx, recordID, fromResync, now, deliveryLeaseDuration)
	if err != nil || !claimed || record == nil {
		return waitRem, err
	}
	ch := resolveLatestChannel(ctx, reg, record)
	rawItems := decodeNotificationItems(record.Attributes)
	items := filterUnsilencedNotificationItems(ctx, reg, record.Attributes["scope"], record.Attributes["tenant"], rawItems, now)
	if len(items) == 0 && record.Attributes["state"] != corev1.AlertStateTest {
		record.Attributes["silenced"] = "true"
		return finalizeNotificationAttempt(ctx, reg, bus, auditDB, record, ch, attempt, nil, now), nil
	}
	title, message := record.Attributes["title"], record.Attributes["message"]
	if len(items) < len(rawItems) {
		title, message, _, _ = summarizeBatchItems(record.Attributes["scope"], record.Attributes["tenant"], items)
		record.Attributes["title"], record.Attributes["message"] = title, message
	}
	deliveryErr := deliverBatchSync(ctx, ch, record.RequestID, title, message, items)
	return finalizeNotificationAttempt(ctx, reg, bus, auditDB, record, ch, attempt, deliveryErr, now), nil
}

func resolveLatestChannel(ctx context.Context, reg *repo.Registry, record *corev1.Event) corev1.AlertChannel {
	attrs := record.Attributes
	ch := corev1.AlertChannel{
		ID: record.ResourceName, Name: record.TargetName,
		Type: attrs["channelType"], URL: attrs["url"], Secret: attrs["secret"],
		ConsoleURL: attrs["consoleURL"], Enabled: true,
	}
	if ch.ID == "" {
		ch.ID = attrs["channelID"]
	}
	if ch.Name == "" {
		ch.Name = attrs["channelName"]
	}
	tenantName := strings.TrimSpace(attrs["tenant"])
	if tenantName == "" {
		tenantName = record.Metadata.Tenant
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil || tenantName == "" {
		return ch
	}
	t, err := tenantDB.Get(ctx, tenantName)
	if err != nil || t == nil {
		return ch
	}
	if t.Metadata.UID != "" && attrs["tenantUID"] == "" {
		attrs["tenantUID"] = t.Metadata.UID
	}
	channels := ExtractTenantChannels(t)
	if strings.TrimSpace(attrs["scope"]) == corev1.AlertScopePlatform {
		channels = ExtractPlatformChannels(t)
	}
	for _, latest := range channels {
		if latest.ID == ch.ID && strings.TrimSpace(latest.URL) != "" {
			ch.Name, ch.Type, ch.URL, ch.Secret = latest.Name, latest.Type, latest.URL, latest.Secret
			attrs["channelName"], attrs["channelType"], attrs["url"], attrs["secret"] = ch.Name, ch.Type, ch.URL, ch.Secret
			if latest.ConsoleURL != "" {
				ch.ConsoleURL = latest.ConsoleURL
				attrs["consoleURL"] = ch.ConsoleURL
			}
			break
		}
	}
	return ch
}

func finalizeNotificationAttempt(ctx context.Context, reg *repo.Registry, bus mq.Interface, auditDB *auditdb.DB, record *corev1.Event, ch corev1.AlertChannel, attempt int, deliveryErr error, now time.Time) time.Duration {
	attrs := record.Attributes
	if attrs == nil {
		attrs = make(map[string]string)
		record.Attributes = attrs
	}
	delete(attrs, "leaseUntil")
	delete(attrs, "notBefore")
	tenantName := strings.TrimSpace(attrs["tenant"])
	if tenantName == "" {
		tenantName = record.Metadata.Tenant
	}
	scope := strings.TrimSpace(attrs["scope"])
	if scope == "" {
		scope = corev1.AlertScopeTenant
	}
	record.Timestamp = now
	if deliveryErr == nil {
		record.Action = corev1.WebhookActionDelivered
		record.StatusCode = 200
		delete(attrs, "error")
		delete(attrs, "nextRetryAt")
		if ok, _ := auditDB.CompleteWebhookAttempt(ctx, record, attempt); ok && attrs["silenced"] != "true" {
			updateChannelHealth(ctx, reg, bus, tenantName, scope, ch.ID, func(target *corev1.AlertChannel) {
				target.State, target.RetryCount, target.LastDeliveryAt = corev1.ChannelStateHealthy, 0, now.Format(time.RFC3339)
				target.LastError, target.NextRetryAt, target.PendingTitle, target.PendingMessage = "", "", "", ""
			})
		}
		return 0
	}
	return recordFailedDelivery(ctx, reg, bus, auditDB, record, ch, tenantName, scope, attempt, deliveryErr, now)
}

func recordFailedDelivery(ctx context.Context, reg *repo.Registry, bus mq.Interface, auditDB *auditdb.DB, record *corev1.Event, ch corev1.AlertChannel, tenantName, scope string, attempt int, deliveryErr error, now time.Time) time.Duration {
	errMsg := deliveryErr.Error()
	if len(errMsg) > 240 {
		errMsg = errMsg[:240]
	}
	attrs := record.Attributes
	attrs["error"] = errMsg
	record.StatusCode = 502
	if attempt < maxDeliveryAttempts {
		delay := retryBackoffDelay(attempt)
		nextRetry := now.Add(delay).Format(time.RFC3339)
		record.Action = corev1.WebhookActionRetrying
		attrs["nextRetryAt"] = nextRetry
		if ok, _ := auditDB.CompleteWebhookAttempt(ctx, record, attempt); ok {
			updateChannelHealth(ctx, reg, bus, tenantName, scope, ch.ID, func(target *corev1.AlertChannel) {
				target.State, target.RetryCount, target.LastError = corev1.ChannelStateRetrying, attempt, errMsg
				target.NextRetryAt, target.PendingTitle, target.PendingMessage = nextRetry, attrs["title"], attrs["message"]
			})
		}
		return delay
	}
	log.Warn("alert notification delivery failed after retries", "scope", scope, "tenant", tenantName, "channel", ch.Name, "attempts", attempt, "error", errMsg)
	record.Action = corev1.WebhookActionDeliveryFailed
	delete(attrs, "nextRetryAt")
	if ok, _ := auditDB.CompleteWebhookAttempt(ctx, record, attempt); ok {
		updateChannelHealth(ctx, reg, bus, tenantName, scope, ch.ID, func(target *corev1.AlertChannel) {
			target.State, target.RetryCount, target.LastError = corev1.ChannelStateFailed, attempt, errMsg
			target.NextRetryAt, target.PendingTitle, target.PendingMessage = "", "", ""
		})
	}
	return 0
}

func updateChannelHealth(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenantName, scope, channelID string, mutate func(*corev1.AlertChannel)) {
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil || tenantName == "" {
		return
	}
	t, err := tenantDB.Get(ctx, tenantName)
	if err != nil || t == nil {
		return
	}
	annotationKey := corev1.AnnotationAlertChannels
	channels := ExtractTenantChannels(t)
	if scope == corev1.AlertScopePlatform {
		annotationKey = corev1.AnnotationPlatformAlertChannels
		channels = ExtractPlatformChannels(t)
	}
	changed := false
	for i := range channels {
		if channels[i].ID == channelID {
			mutate(&channels[i])
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	if scope == corev1.AlertScopePlatform {
		t.Spec.PlatformAlertChannels = channels
	} else {
		t.Spec.AlertChannels = channels
	}
	if t.Metadata.Annotations == nil {
		t.Metadata.Annotations = make(map[string]string)
	}
	if b, err := json.Marshal(channels); err == nil {
		t.Metadata.Annotations[annotationKey] = string(b)
	}
	if updated, err := tenantDB.Update(ctx, t); err == nil && updated != nil && bus != nil {
		_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Tenant", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
}

func retryBackoffDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

func newDeliveryID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "dlv-" + hex.EncodeToString(b)
}
