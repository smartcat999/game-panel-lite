package regionstatusingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

type projectionFunc func(context.Context, string, regionstatus.Snapshot) error

func (f projectionFunc) RecordRegionStatus(ctx context.Context, source string, snapshot regionstatus.Snapshot) error {
	return f(ctx, source, snapshot)
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
	})}
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
