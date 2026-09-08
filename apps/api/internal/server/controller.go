package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gateway"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type ControllerStore interface {
	ListGameServers(context.Context) ([]domain.GameServer, error)
	SaveReconciledGameServer(context.Context, domain.GameServer, domain.GameServer) error
}

type deletingControllerStore interface {
	DeleteGameServer(context.Context, string) error
}

type activityControllerStore interface {
	CreateActivity(context.Context, *domain.ActivityEvent) error
}

type assignmentControllerStore interface {
	PublishWorkloadAssignment(context.Context, domain.GameServer, *domain.WorkloadAssignment) error
	GetWorkloadAssignmentByServer(context.Context, string) (domain.WorkloadAssignment, error)
	DeleteWorkloadAssignment(context.Context, string) error
	GetWorkloadObservation(context.Context, string) (domain.WorkloadObservation, error)
}

type pendingPlacementStore interface {
	AssignPendingGameServer(context.Context, domain.GameServer, domain.GameServer) error
}

type nodeStatusControllerStore interface {
	GetComputeNode(context.Context, string) (domain.ComputeNode, error)
}

type NodeScheduler interface {
	Schedule(context.Context, domain.GameServer) (domain.ComputeNode, error)
}

type Controller struct {
	store           ControllerStore
	reconciler      *Reconciler
	gateway         *gateway.StreamGateway
	logger          *slog.Logger
	interval        time.Duration
	dataRoot        string
	scheduler       NodeScheduler
	recoveryTimeout time.Duration
	locksMu         sync.Mutex
	locks           map[string]*sync.Mutex
}

func NewController(store ControllerStore, reconciler *Reconciler, logger *slog.Logger) *Controller {
	if reconciler == nil {
		reconciler = NewReconciler()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Controller{
		store:           store,
		reconciler:      reconciler,
		logger:          logger,
		interval:        3 * time.Second,
		recoveryTimeout: 2 * time.Minute,
		locks:           map[string]*sync.Mutex{},
	}
}

func (c *Controller) WithScheduler(sched NodeScheduler) *Controller {
	c.scheduler = sched
	return c
}

func (c *Controller) WithRecoveryTimeout(timeout time.Duration) *Controller {
	if timeout > 0 {
		c.recoveryTimeout = timeout
	}
	return c
}

func (c *Controller) WithDataRoot(dataRoot string) *Controller {
	c.dataRoot = dataRoot
	return c
}

func (c *Controller) WithGateway(gw *gateway.StreamGateway) *Controller {
	c.gateway = gw
	return c
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
		if c.gateway != nil {
			listenPort := item.Spec.Network.HostPort
			if listenPort <= 0 {
				listenPort = item.Spec.Network.Port
			}
			if item.Status.Phase == domain.PhaseRunning && listenPort > 0 {
				_ = c.gateway.RegisterForward(gateway.ForwardRule{
					ID:         item.ID,
					NodeID:     item.NodeID,
					ListenPort: listenPort,
					TargetPort: listenPort,
				})
			} else if item.Status.Phase == domain.PhaseStopped || item.Status.Phase == domain.PhaseFailed || item.Status.Phase == domain.PhaseDeleted {
				c.gateway.UnregisterForward(item.ID)
			}
		}

		c.reconcileOne(ctx, item)
	}
}

func (c *Controller) reconcileOne(ctx context.Context, item domain.GameServer) {
	lock := c.lockFor(item.ID)
	lock.Lock()
	defer lock.Unlock()

	// All workloads converge declaratively through durable desired assignments
	// and worker observations (kube-apiserver <-> kubelet architecture).
	c.reconcileRemote(ctx, item)
}

func (c *Controller) reconcileRemote(ctx context.Context, item domain.GameServer) {
	assignments, ok := c.store.(assignmentControllerStore)
	if !ok || c.reconciler == nil || c.reconciler.Builder() == nil {
		return
	}
	before := item
	now := time.Now().UTC()

	if item.NodeID == "" {
		if item.Spec.DesiredState == domain.DesiredDeleted {
			if deletingStore, deleteOK := c.store.(deletingControllerStore); deleteOK {
				if err := cleanupOwnedResources(ctx, c.store, item, c.dataRoot); err != nil {
					c.logger.Warn("failed to clean unassigned server resources", "server", item.ID, "error", err)
					return
				}
				_ = assignments.DeleteWorkloadAssignment(ctx, item.ID)
				if err := deletingStore.DeleteGameServer(ctx, item.ID); err != nil {
					c.logger.Warn("failed to delete unassigned server record", "server", item.ID, "error", err)
				}
				c.recordReconcileEvents(ctx, before, item, nil)
				return
			}
		}
		if item.Spec.DesiredState == domain.DesiredStopped {
			if item.Status.Phase != domain.PhaseStopped || item.Status.ActualState != domain.ActualStopped {
				item.Status.Phase = domain.PhaseStopped
				item.Status.ActualState = domain.ActualStopped
				item.Status.ObservedGeneration = item.Spec.Generation
				item.Status.AppliedGeneration = item.Spec.Generation
				item.Status.LastReconcileAt = now
				_ = c.store.SaveReconciledGameServer(ctx, before, item)
			}
			return
		}
		if item.Spec.DesiredState == domain.DesiredRunning && c.scheduler != nil {
			chosenNode, err := c.scheduler.Schedule(ctx, item)
			if err == nil {
				item.NodeID = chosenNode.ID
				item.Spec.Generation++
				item.Status.Phase = domain.PhasePending
				item.Status.ActualState = domain.ActualUnknown
				item.Status.LastReconcileAt = now
				item.Status.Conditions = upsertServerCondition(item.Status.Conditions, domain.ServerCondition{
					Type:               "Scheduled",
					Status:             "True",
					Reason:             "NodeAssigned",
					Message:            "assigned to node " + chosenNode.Name,
					ObservedGeneration: item.Spec.Generation,
					LastTransitionAt:   now,
				})
				placement, supported := c.store.(pendingPlacementStore)
				if !supported {
					c.logger.Error("store does not support atomic initial placement", "server", item.ID)
					return
				}
				if err := placement.AssignPendingGameServer(ctx, before, item); err != nil {
					c.logger.Warn("initial node allocation was not committed", "server", item.ID, "node", item.NodeID, "error", err)
					return
				}
				c.recordReconcileEvents(ctx, before, item, nil)
				before = item
			} else {
				item.Status.Conditions = upsertServerCondition(item.Status.Conditions, domain.ServerCondition{
					Type:               "Scheduled",
					Status:             "False",
					Reason:             "Unschedulable",
					Message:            err.Error(),
					ObservedGeneration: item.Spec.Generation,
					LastTransitionAt:   now,
				})
				item.Status.Phase = domain.PhasePending
				item.Status.ActualState = domain.ActualUnknown
				item.Status.LastReconcileAt = now
				_ = c.store.SaveReconciledGameServer(ctx, before, item)
				return
			}
		} else {
			if item.Status.Phase != domain.PhasePending {
				item.Status.Phase = domain.PhasePending
				item.Status.ActualState = domain.ActualUnknown
				item.Status.LastReconcileAt = now
				_ = c.store.SaveReconciledGameServer(ctx, before, item)
			}
			return
		}
	}

	current, currentErr := assignments.GetWorkloadAssignmentByServer(ctx, item.ID)
	assignmentUID := current.UID
	assignmentID := current.ID
	createdAt := current.CreatedAt
	if currentErr != nil || current.NodeID != item.NodeID || assignmentUID == "" {
		assignmentUID = uuid.NewString()
		assignmentID = uuid.NewString()
		createdAt = now
	}

	workloadSpec := current.Spec
	if item.Spec.DesiredState != domain.DesiredDeleted || currentErr != nil {
		built, err := c.reconciler.Builder().BuildWorkloadSpec(ctx, item)
		if err != nil {
			item.Status.LastError = err.Error()
			item.Status.ActualState = domain.ActualUnknown
			setPhase(&item.Status, domain.PhaseFailed, now)
			_ = c.store.SaveReconciledGameServer(ctx, before, item)
			return
		}
		workloadSpec = built
	}
	assignment := domain.WorkloadAssignment{
		ID:           assignmentID,
		UID:          assignmentUID,
		ServerID:     item.ID,
		NodeID:       item.NodeID,
		Generation:   item.Spec.Generation,
		DesiredState: item.Spec.DesiredState,
		Spec:         workloadSpec,
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	}
	if item.Spec.DesiredState == domain.DesiredDeleted {
		assignment.DeletionTimestamp = &now
	}
	if err := assignments.PublishWorkloadAssignment(ctx, before, &assignment); err != nil {
		c.logger.Warn("failed to persist remote workload assignment", "server", item.ID, "node", item.NodeID, "error", err)
		return
	}
	if nodes, nodeOK := c.store.(nodeStatusControllerStore); nodeOK {
		node, err := nodes.GetComputeNode(ctx, assignment.NodeID)
		isOffline := err != nil || node.Status == "offline" || node.LastHeartbeat.IsZero() || now.Sub(node.LastHeartbeat) > 45*time.Second
		if isOffline {
			staleDuration := 46 * time.Second
			if !node.LastHeartbeat.IsZero() {
				staleDuration = now.Sub(node.LastHeartbeat)
			}
			if item.Spec.DesiredState == domain.DesiredRunning && staleDuration >= c.recoveryTimeout {
				// A missing heartbeat does not prove the old game process has
				// stopped. Preserve its placement and assignment until fencing
				// and a recoverable world checkpoint have been established.
				item.Status.Conditions = upsertServerCondition(item.Status.Conditions, domain.ServerCondition{
					Type:               "RecoveryReady",
					Status:             "False",
					Reason:             "FencingAndCheckpointRequired",
					Message:            "source node is unreachable; recovery requires confirmed source isolation and a recoverable world checkpoint",
					ObservedGeneration: item.Spec.Generation,
					LastTransitionAt:   now,
				})
			}

			item.Status.ActualState = domain.ActualUnknown
			item.Status.LastReconcileAt = now
			item.Status.Conditions = upsertServerCondition(item.Status.Conditions, domain.ServerCondition{
				Type:               "AgentReachable",
				Status:             "Unknown",
				Reason:             "HeartbeatStale",
				Message:            "worker heartbeat is stale; runtime state cannot be confirmed",
				ObservedGeneration: item.Spec.Generation,
				LastTransitionAt:   now,
			})
			setPhase(&item.Status, domain.PhaseReconciling, now)
			_ = c.store.SaveReconciledGameServer(ctx, before, item)
			c.recordReconcileEvents(ctx, before, item, nil)
			return
		}
	}

	observation, err := assignments.GetWorkloadObservation(ctx, assignment.UID)
	item.Status.LastReconcileAt = now
	if err != nil || observation.AssignmentUID != assignment.UID || observation.NodeID != assignment.NodeID {
		item.Status.ActualState = domain.ActualUnknown
		setPhase(&item.Status, domain.PhaseReconciling, now)
		_ = c.store.SaveReconciledGameServer(ctx, before, item)
		c.recordReconcileEvents(ctx, before, item, nil)
		return
	}
	item.Status.RuntimeID = observation.RuntimeID
	item.Status.ActualState = observation.ActualState
	item.Status.ObservedGeneration = observation.ObservedGeneration
	item.Status.Conditions = observation.Conditions
	item.Status.LastError = observation.LastError
	if observation.LastError != "" {
		setPhase(&item.Status, domain.PhaseFailed, now)
		_ = c.store.SaveReconciledGameServer(ctx, before, item)
		c.recordReconcileEvents(ctx, before, item, nil)
		return
	}
	if observation.ObservedGeneration < assignment.Generation {
		setPhase(&item.Status, domain.PhaseReconciling, now)
		_ = c.store.SaveReconciledGameServer(ctx, before, item)
		c.recordReconcileEvents(ctx, before, item, nil)
		return
	}
	item.Status.AppliedGeneration = observation.ObservedGeneration
	switch assignment.DesiredState {
	case domain.DesiredRunning:
		if observation.ActualState == domain.ActualRunning {
			setPhase(&item.Status, domain.PhaseRunning, now)
		} else {
			setPhase(&item.Status, domain.PhaseReconciling, now)
		}
	case domain.DesiredStopped:
		if observation.ActualState == domain.ActualStopped || observation.ActualState == domain.ActualMissing {
			setPhase(&item.Status, domain.PhaseStopped, now)
		} else {
			setPhase(&item.Status, domain.PhaseReconciling, now)
		}
	case domain.DesiredDeleted:
		if observation.ActualState == domain.ActualMissing {
			if deletingStore, deleteOK := c.store.(deletingControllerStore); deleteOK {
				if err := cleanupOwnedResources(ctx, c.store, item, c.dataRoot); err != nil {
					c.logger.Warn("failed to clean remote server resources", "server", item.ID, "error", err)
					return
				}
				if err := assignments.DeleteWorkloadAssignment(ctx, item.ID); err != nil {
					c.logger.Warn("failed to delete remote workload assignment", "server", item.ID, "error", err)
					return
				}
				if err := deletingStore.DeleteGameServer(ctx, item.ID); err != nil {
					c.logger.Warn("failed to delete remote server record", "server", item.ID, "error", err)
				}
				c.recordReconcileEvents(ctx, before, item, nil)
				return
			}
		}
		setPhase(&item.Status, domain.PhaseDeleting, now)
	}
	_ = c.store.SaveReconciledGameServer(ctx, before, item)
	c.recordReconcileEvents(ctx, before, item, nil)
}

func upsertServerCondition(conditions []domain.ServerCondition, condition domain.ServerCondition) []domain.ServerCondition {
	return workload.SetCondition(conditions, condition)
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
		event.OrganizationID = after.OrganizationID
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
	beforeArt, beforeHasArt := workload.FindCondition(before.Status.Conditions, workload.ConditionArtifactsReady)
	afterArt, afterHasArt := workload.FindCondition(after.Status.Conditions, workload.ConditionArtifactsReady)
	if afterHasArt {
		if afterArt.Status == workload.ConditionStatusTrue {
			if !beforeHasArt || beforeArt.Status != workload.ConditionStatusTrue || before.Status.AppliedGeneration != after.Status.AppliedGeneration {
				events = append(events, newReconciliationActivity(after, "server.artifacts.ready", "Artifacts prepared successfully for server "+after.Name, now, operationID))
			}
		} else if afterArt.Status == workload.ConditionStatusFalse {
			if !beforeHasArt || beforeArt.Status != workload.ConditionStatusFalse || beforeArt.Message != afterArt.Message || before.Status.ObservedGeneration != after.Status.ObservedGeneration {
				events = append(events, newReconciliationActivity(after, "server.artifacts.failed", "Failed to prepare artifacts for server "+after.Name+": "+afterArt.Message, now, operationID))
			}
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
