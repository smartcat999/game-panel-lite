package nodeexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

func TestAgentPollingIsBoundedAndTerminalEffectIsNotDuplicated(t *testing.T) {
	region, now := assignedRegion(t)
	calls := 0
	agent := Agent{NodeID: "nod_test", BatchSize: 1000, ClaimTTL: time.Minute, Store: region, Reconcile: func(context.Context, regionexecution.WorkAssignment) error { calls++; return nil }}
	processed, err := agent.RunOnce(context.Background(), now)
	if err != nil || processed != 1 {
		t.Fatalf("first run processed=%d error=%v", processed, err)
	}
	processed, err = agent.RunOnce(context.Background(), now.Add(time.Second))
	if err != nil || processed != 0 || calls != 1 {
		t.Fatalf("redelivery processed=%d calls=%d error=%v", processed, calls, err)
	}
	metrics := agent.Metrics()
	if metrics.Polls != 2 || metrics.Claimed != 1 || metrics.Succeeded != 1 {
		t.Fatalf("metrics=%#v", metrics)
	}
}

func TestRestartReclaimsExpiredAssignmentLease(t *testing.T) {
	region, now := assignedRegion(t)
	assignment := region.PollAssignments(context.Background(), "nod_test", 1, now)[0]
	if _, err := region.ClaimAssignment(context.Background(), assignment.ID, "nod_test", now.Add(time.Second), now); err != nil {
		t.Fatal(err)
	}
	calls := 0
	restarted := Agent{NodeID: "nod_test", BatchSize: 1, ClaimTTL: time.Minute, Store: region, Reconcile: func(context.Context, regionexecution.WorkAssignment) error { calls++; return nil }}
	processed, err := restarted.RunOnce(context.Background(), now.Add(2*time.Second))
	if err != nil || processed != 1 || calls != 1 {
		t.Fatalf("restart processed=%d calls=%d error=%v", processed, calls, err)
	}
}

func TestStaleNodeOrFencingTokenCannotCompleteAssignment(t *testing.T) {
	region, now := assignedRegion(t)
	assignment := region.PollAssignments(context.Background(), "nod_test", 1, now)[0]
	claimed, err := region.ClaimAssignment(context.Background(), assignment.ID, "nod_test", now.Add(time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := region.OverridePlacement(context.Background(), "usr_operator", claimed.RegionalDeploymentID, "nod_other", "replace host", now); err != nil {
		t.Fatal(err)
	}
	if completed, err := region.CompleteAssignment(context.Background(), claimed.ID, "nod_test", claimed.FencingToken, true, now.Add(time.Second)); err == nil || completed {
		t.Fatalf("stale completion completed=%v error=%v", completed, err)
	}

	region2, now2 := assignedRegion(t)
	second := region2.PollAssignments(context.Background(), "nod_test", 1, now2)[0]
	claimed2, _ := region2.ClaimAssignment(context.Background(), second.ID, "nod_test", now2.Add(time.Minute), now2)
	if err := region2.RenewLease(context.Background(), "nod_test", now2.Add(-time.Second), now2); err != nil {
		t.Fatal(err)
	}
	if completed, err := region2.CompleteAssignment(context.Background(), claimed2.ID, "nod_test", claimed2.FencingToken, true, now2); err == nil || completed {
		t.Fatalf("stale Node completed=%v error=%v", completed, err)
	}
}

func TestBackoffIsExponentiallyBoundedAndFailuresAreObservable(t *testing.T) {
	region, now := assignedRegion(t)
	agent := Agent{NodeID: "nod_test", BatchSize: 1, ClaimTTL: time.Minute, BackoffBase: 100 * time.Millisecond, BackoffMax: 800 * time.Millisecond, Store: region, Reconcile: func(context.Context, regionexecution.WorkAssignment) error { return errors.New("runtime failed") }}
	if agent.Backoff(0) != 100*time.Millisecond || agent.Backoff(10) != 800*time.Millisecond {
		t.Fatalf("unbounded backoff: first=%s max=%s", agent.Backoff(0), agent.Backoff(10))
	}
	if _, err := agent.RunOnce(context.Background(), now); err == nil {
		t.Fatal("reconciliation failure was hidden")
	}
	if metrics := agent.Metrics(); metrics.ReconciliationFailures != 1 || metrics.Failed != 1 {
		t.Fatalf("metrics=%#v", metrics)
	}
	if monitoring := region.Monitoring(context.Background()); monitoring.ReconciliationFailures != 1 || monitoring.TaskLatencyMilliseconds < 0 {
		t.Fatalf("region monitoring=%#v", monitoring)
	}
}

func assignedRegion(t *testing.T) (*regionexecution.Module, time.Time) {
	t.Helper()
	now := time.Date(2026, time.September, 10, 3, 0, 0, 0, time.UTC)
	region := regionexecution.New("reg_test", []regionexecution.Node{
		{ID: "nod_test", RegionID: "reg_test", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 4000, MemoryCapacityMB: 8192, LeaseUntil: now.Add(time.Hour)},
		{ID: "nod_other", RegionID: "reg_test", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 8000, MemoryCapacityMB: 16384, LeaseUntil: now.Add(time.Hour)},
	})
	deployment, _, err := region.ReceiveDesired(context.Background(), regionexecution.DesiredDeployment{MessageID: "evt_assignment", WorkspaceID: "ws_test", LogicalInstanceID: "lin_test", RegionID: "reg_test", PlacementVersion: 1, InstanceRevisionID: "rev_test", DesiredState: "running", GameKey: "terraria", GameVersion: "1.4.5.6", Configuration: map[string]any{"worldName": "Assigned World"}, CPUUnits: 1000, MemoryMegabytes: 1024}, now)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := region.Schedule(context.Background(), deployment.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.NodeID != "nod_test" {
		t.Fatalf("test scheduler selected %s", reservation.NodeID)
	}
	return region, now
}
