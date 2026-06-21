package server

import (
	"context"
	"log/slog"
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

	updated, err := c.reconciler.Reconcile(ctx, item)
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
			c.recordReconcileEvents(ctx, item, updated)
			return
		}
	}
	if err := c.store.SaveGameServer(ctx, &updated); err != nil {
		c.logger.Warn("failed to save reconciled server", "server", item.ID, "error", err)
		return
	}
	c.recordReconcileEvents(ctx, item, updated)
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

func (c *Controller) recordReconcileEvents(ctx context.Context, before domain.GameServer, after domain.GameServer) {
	activityStore, ok := c.store.(activityControllerStore)
	if !ok {
		return
	}
	for _, event := range reconciliationActivityEvents(before, after, time.Now()) {
		if err := activityStore.CreateActivity(ctx, &event); err != nil {
			c.logger.Warn("failed to record reconciliation activity", "server", after.ID, "type", event.Type, "error", err)
		}
	}
}

func reconciliationActivityEvents(before domain.GameServer, after domain.GameServer, now time.Time) []domain.ActivityEvent {
	events := make([]domain.ActivityEvent, 0, 3)
	if before.Status.RuntimeID != after.Status.RuntimeID {
		if before.Status.RuntimeID != "" {
			events = append(events, newReconciliationActivity(after, "server.runtime.removed", "Removed runtime workload for server "+after.Name, now))
		}
		if after.Status.RuntimeID != "" {
			events = append(events, newReconciliationActivity(after, "server.runtime.created", "Created runtime workload for server "+after.Name, now))
		}
	}
	if after.Status.Phase == domain.PhaseFailed && (before.Status.Phase != domain.PhaseFailed || before.Status.LastError != after.Status.LastError || before.Status.ObservedGeneration != after.Status.ObservedGeneration) {
		events = append(events, newReconciliationActivity(after, "server.reconcile.failed", after.Name+": "+after.Status.LastError, now))
		return events
	}
	if after.Status.Phase == domain.PhaseRunning && (before.Status.Phase != domain.PhaseRunning || before.Status.AppliedGeneration != after.Status.AppliedGeneration || before.Status.ActualState != domain.ActualRunning) {
		events = append(events, newReconciliationActivity(after, "server.started", "Started server "+after.Name, now))
	}
	if after.Status.Phase == domain.PhaseStopped && (before.Status.Phase != domain.PhaseStopped || before.Status.ActualState != after.Status.ActualState || before.Status.ObservedGeneration != after.Status.ObservedGeneration) {
		events = append(events, newReconciliationActivity(after, "server.stopped", "Stopped server "+after.Name, now))
	}
	if after.Status.Phase == domain.PhaseDeleted && before.Status.Phase != domain.PhaseDeleted {
		events = append(events, newReconciliationActivity(after, "server.deleted", "Deleted server "+after.Name, now))
	}
	return events
}

func newReconciliationActivity(server domain.GameServer, eventType string, message string, now time.Time) domain.ActivityEvent {
	return domain.ActivityEvent{
		ID:         uuid.NewString(),
		InstanceID: server.ID,
		Type:       eventType,
		Message:    message,
		Payload: map[string]any{
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
		},
		CreatedAt: now,
	}
}
