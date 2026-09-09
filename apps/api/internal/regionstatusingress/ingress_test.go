package regionstatusingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

type projectionFunc func(context.Context, string, regionstatus.Snapshot) error

func (f projectionFunc) RecordRegionStatus(ctx context.Context, source string, snapshot regionstatus.Snapshot) error {
	return f(ctx, source, snapshot)
}

type deploymentProjectionFunc func(context.Context, string, deploymentstatus.Event) error

func (f deploymentProjectionFunc) RecordDeploymentStatus(ctx context.Context, source string, event deploymentstatus.Event) error {
	return f(ctx, source, event)
}

func TestIngressRejectsUntrustedOrMalformedStatus(t *testing.T) {
	snapshot := regionstatus.Snapshot{SchemaVersion: 1, EventID: "event", RegionID: "east", Sequence: 1, ObservedAtMS: 1}
	payload, _ := json.Marshal(snapshot)
	calls := 0
	ingress := Ingress{SourceRegionID: "east", MaxBytes: 2048, Projection: projectionFunc(func(_ context.Context, source string, got regionstatus.Snapshot) error {
		calls++
		if source != "east" || got != snapshot {
			t.Fatal("projection identity changed")
		}
		return nil
	}), Deployments: deploymentProjectionFunc(func(context.Context, string, deploymentstatus.Event) error { return nil })}
	message := regional.Notification{ID: snapshot.EventID, ContentType: "application/json", Body: payload}
	if err := ingress.Handle(context.Background(), message); err != nil || calls != 1 {
		t.Fatalf("valid status: %v", err)
	}
	for _, mutate := range []func(*regional.Notification){
		func(n *regional.Notification) { n.ID = "other" }, func(n *regional.Notification) { n.ContentType = "text/plain" },
		func(n *regional.Notification) { n.Body = append(n.Body, []byte(" {}")...) },
		func(n *regional.Notification) {
			n.Body = []byte(strings.Replace(string(n.Body), `"regionId":"east"`, `"regionId":"west"`, 1))
		},
		func(n *regional.Notification) {
			n.Body = []byte(strings.Replace(string(n.Body), `"sequence":1`, `"sequence":1,"Sequence":2`, 1))
		},
	} {
		bad := message
		mutate(&bad)
		if !errors.Is(ingress.Handle(context.Background(), bad), regional.ErrInvalidNotification) {
			t.Fatal("invalid status accepted")
		}
	}
	ingress.Projection = projectionFunc(func(context.Context, string, regionstatus.Snapshot) error { return regionstatus.ErrSnapshotConflict })
	if !errors.Is(ingress.Handle(context.Background(), message), regional.ErrNotificationConflict) {
		t.Fatal("projection conflict retried")
	}
}

func TestIngressRoutesDeploymentStatus(t *testing.T) {
	event := deploymentstatus.Event{SchemaVersion: 1, EventID: "deployment-event", RegionID: "east", OrganizationID: "tenant", OperationID: "operation", ServerID: "server", RevisionID: "revision", TaskID: "task", NodeID: "node", PlacementEpoch: 1, SpecGeneration: 1, IntentVersion: 1, Fence: 2, ActualState: "running", Outcome: "succeeded", RuntimeID: "container", ObservedAtMS: 100}
	payload, _ := json.Marshal(event)
	calls := 0
	ingress := Ingress{SourceRegionID: "east", MaxBytes: 2048,
		Projection: projectionFunc(func(context.Context, string, regionstatus.Snapshot) error {
			t.Fatal("deployment routed as aggregate")
			return nil
		}),
		Deployments: deploymentProjectionFunc(func(_ context.Context, source string, got deploymentstatus.Event) error {
			calls++
			if source != "east" || got != event {
				t.Fatal("deployment identity changed")
			}
			return nil
		})}
	message := regional.Notification{ID: event.EventID, ContentType: "application/json", Type: "deployment.status.observed", Body: payload}
	if err := ingress.Handle(context.Background(), message); err != nil || calls != 1 {
		t.Fatal("valid deployment status", err)
	}
	conflict := ingress
	conflict.Deployments = deploymentProjectionFunc(func(context.Context, string, deploymentstatus.Event) error { return deploymentstatus.ErrEventConflict })
	if !errors.Is(conflict.Handle(context.Background(), message), regional.ErrNotificationConflict) {
		t.Fatal("deployment conflict retried")
	}
	bad := message
	bad.Body = []byte(strings.Replace(string(payload), `"fence":2`, `"fence":2,"Fence":3`, 1))
	if !errors.Is(ingress.Handle(context.Background(), bad), regional.ErrInvalidNotification) {
		t.Fatal("duplicate deployment field accepted")
	}
}
