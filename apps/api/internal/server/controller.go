package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type ControllerStore interface {
	ListGameServers(context.Context) ([]domain.GameServer, error)
	SaveGameServer(context.Context, *domain.GameServer) error
}

type deletingControllerStore interface {
	DeleteGameServer(context.Context, string) error
}

type activityControllerStore interface {
	CreateActivity(context.Context, *domain.ActivityEvent) error
}

type Controller struct {
	store      ControllerStore
	reconciler *Reconciler
	logger     *slog.Logger
	interval   time.Duration
	locksMu    sync.Mutex
	locks      map[string]*sync.Mutex
}

func NewController(store ControllerStore, reconciler *Reconciler, logger *slog.Logger) *Controller {
	if reconciler == nil {
		reconciler = NewReconciler()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Controller{
		store:      store,
		reconciler: reconciler,
		logger:     logger,
		interval:   3 * time.Second,
		locks:      map[string]*sync.Mutex{},
	}
}

func (c *Controller) WithInterval(interval time.Duration) *Controller {
	if interval > 0 {
		c.interval = interval
	}
	return c
}

func (c *Controller) Start(ctx context.Context) {
	if c.interval <= 0 {
		c.interval = 3 * time.Second
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		c.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Controller) RunOnce(ctx context.Context) {
	servers, err := c.store.ListGameServers(ctx)
	if err != nil {
		c.logger.Warn("failed to list servers for reconciliation", "error", err)
		return
	}
	for _, item := range servers {
		if !c.reconciler.NeedsReconcile(item) {
			continue
		}
		c.reconcileOne(ctx, item)
	}
}

func (c *Controller) reconcileOne(ctx context.Context, item domain.GameServer) {
	lock := c.lockFor(item.ID)
	lock.Lock()
	defer lock.Unlock()

	updated, lifecycleEvents, err := c.reconciler.ReconcileWithEvents(ctx, item)
	if err != nil {
		c.logger.Warn("server reconciliation failed", "server", item.ID, "error", err)
		return
	}
	if updated.Status.Phase == domain.PhaseDeleted {
		if deletingStore, ok := c.store.(deletingControllerStore); ok {
			if err := cleanupOwnedResources(ctx, c.store, updated); err != nil {
				c.logger.Warn("failed to clean owned server resources", "server", item.ID, "error", err)
				return
			}
			if err := deletingStore.DeleteGameServer(ctx, updated.ID); err != nil {
				c.logger.Warn("failed to delete reconciled server resource", "server", item.ID, "error", err)
				return
			}
			c.recordReconcileEvents(ctx, item, updated, lifecycleEvents)
			return
		}
	}
	if err := c.store.SaveGameServer(ctx, &updated); err != nil {
		c.logger.Warn("failed to save reconciled server", "server", item.ID, "error", err)
		return
	}
	c.recordReconcileEvents(ctx, item, updated, lifecycleEvents)
}

func (c *Controller) lockFor(id string) *sync.Mutex {
	c.locksMu.Lock()
	defer c.locksMu.Unlock()
	lock := c.locks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		c.locks[id] = lock
	}
	return lock
}

func (c *Controller) recordReconcileEvents(ctx context.Context, before domain.GameServer, after domain.GameServer, lifecycleEvents []LifecycleEvent) {
	activityStore, ok := c.store.(activityControllerStore)
	if !ok {
		return
	}
	operationID := uuid.NewString()
	events := reconciliationLifecycleActivityEvents(after, lifecycleEvents, time.Now(), operationID)
	events = append(events, reconciliationActivityEvents(before, after, time.Now(), lifecycleEvents, operationID)...)
	for _, event := range events {
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		if err := activityStore.CreateActivity(ctx, &event); err != nil {
			c.logger.Warn("failed to record reconciliation activity", "server", after.ID, "type", event.Type, "error", err)
		}
	}
}

func reconciliationLifecycleActivityEvents(server domain.GameServer, lifecycleEvents []LifecycleEvent, now time.Time, operationID string) []domain.ActivityEvent {
	events := make([]domain.ActivityEvent, 0, len(lifecycleEvents))
	for _, item := range lifecycleEvents {
		occurredAt := item.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = now
		}
		payload := map[string]any{"operationId": operationID}
		for key, value := range item.Payload {
			payload[key] = value
		}
		events = append(events, newReconciliationActivityWithPayload(server, item.Type, item.Message, occurredAt, payload))
	}
	return events
}

func reconciliationActivityEvents(before domain.GameServer, after domain.GameServer, now time.Time, lifecycleEvents []LifecycleEvent, operationID string) []domain.ActivityEvent {
	events := make([]domain.ActivityEvent, 0, 3)
	lifecycle := newLifecycleEventSet(lifecycleEvents)
	if before.Status.RuntimeID != after.Status.RuntimeID {
		if before.Status.RuntimeID != "" && !lifecycle.hasPrefix("server.container.remove.") {
			events = append(events, newReconciliationActivity(after, "server.runtime.removed", "Removed runtime workload for server "+after.Name, now, operationID))
		}
		if after.Status.RuntimeID != "" && !lifecycle.hasPrefix("server.container.create.") {
			events = append(events, newReconciliationActivity(after, "server.runtime.created", "Created runtime workload for server "+after.Name, now, operationID))
		}
	}
	if after.Status.Phase == domain.PhaseFailed && (before.Status.Phase != domain.PhaseFailed || before.Status.LastError != after.Status.LastError || before.Status.ObservedGeneration != after.Status.ObservedGeneration) {
		events = append(events, newReconciliationActivity(after, "server.reconcile.failed", after.Name+": "+after.Status.LastError, now, operationID))
		return events
	}
	if after.Status.Phase == domain.PhaseRunning && !lifecycle.has("server.container.start.succeeded") && (before.Status.Phase != domain.PhaseRunning || before.Status.AppliedGeneration != after.Status.AppliedGeneration || before.Status.ActualState != domain.ActualRunning) {
		events = append(events, newReconciliationActivity(after, "server.started", "Started server "+after.Name, now, operationID))
	}
	if after.Status.Phase == domain.PhaseStopped && !lifecycle.has("server.container.stop.succeeded") && !isInitialStoppedReconcile(before, after) && (before.Status.Phase != domain.PhaseStopped || before.Status.ActualState != after.Status.ActualState || before.Status.ObservedGeneration != after.Status.ObservedGeneration) {
		events = append(events, newReconciliationActivity(after, "server.stopped", "Stopped server "+after.Name, now, operationID))
	}
	if after.Status.Phase == domain.PhaseDeleted && before.Status.Phase != domain.PhaseDeleted {
		events = append(events, newReconciliationActivity(after, "server.deleted", "Deleted server "+after.Name, now, operationID))
	}
	return events
}

type lifecycleEventSet map[string]struct{}

func (items lifecycleEventSet) has(eventType string) bool {
	_, ok := items[eventType]
	return ok
}

func (items lifecycleEventSet) hasPrefix(prefix string) bool {
	for item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func newLifecycleEventSet(items []LifecycleEvent) lifecycleEventSet {
	result := make(lifecycleEventSet, len(items))
	for _, item := range items {
		result[item.Type] = struct{}{}
	}
	return result
}

func isInitialStoppedReconcile(before domain.GameServer, after domain.GameServer) bool {
	return before.Spec.DesiredState == domain.DesiredStopped &&
		after.Spec.DesiredState == domain.DesiredStopped &&
		before.Status.Phase == domain.PhasePending &&
		before.Status.ActualState == domain.ActualMissing &&
		before.Status.RuntimeID == "" &&
		after.Status.RuntimeID == "" &&
		after.Status.ActualState == domain.ActualMissing
}

func newReconciliationActivity(server domain.GameServer, eventType string, message string, now time.Time, operationID string) domain.ActivityEvent {
	return newReconciliationActivityWithPayload(server, eventType, message, now, map[string]any{"operationId": operationID})
}

func newReconciliationActivityWithPayload(server domain.GameServer, eventType string, message string, now time.Time, extraPayload map[string]any) domain.ActivityEvent {
	payload := map[string]any{
		"serverId":      server.ID,
		"serverName":    server.Name,
		"gameKey":       server.GameKey,
		"providerKey":   server.ProviderKey,
		"desiredState":  server.Spec.DesiredState,
		"generation":    server.Spec.Generation,
		"runtimePhase":  server.Status.Phase,
		"runtimeId":     server.Status.RuntimeID,
		"runtimeStatus": server.Status.ActualState,
		"lastError":     server.Status.LastError,
	}
	for key, value := range extraPayload {
		payload[key] = value
	}
	return domain.ActivityEvent{
		ID:         uuid.NewString(),
		InstanceID: server.ID,
		Type:       eventType,
		Message:    message,
		Payload:    payload,
		CreatedAt:  now,
	}
}
