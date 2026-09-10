package regionexecution

import (
	"context"
	"sync"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestDuplicateAndOutOfOrderEventsCannotRegressDeployment(t *testing.T) {
	module := newTestRegion(4000, 4096)
	desired := desiredEvent("evt_desired_1", "lin_one")
	deployment, changed, err := module.ReceiveDesired(context.Background(), desired, phase4Now())
	if err != nil || !changed {
		t.Fatalf("ReceiveDesired changed=%v error=%v", changed, err)
	}
	if _, changed, err := module.ReceiveDesired(context.Background(), desired, phase4Now()); err != nil || changed {
		t.Fatalf("duplicate desired changed=%v error=%v", changed, err)
	}
	newerPlacement := desired
	newerPlacement.MessageID = "evt_desired_2"
	newerPlacement.PlacementVersion = 2
	if _, changed, err := module.ReceiveDesired(context.Background(), newerPlacement, phase4Now()); err != nil || !changed {
		t.Fatalf("new placement changed=%v error=%v", changed, err)
	}
	olderPlacement := desired
	olderPlacement.MessageID = "evt_desired_old"
	if _, changed, err := module.ReceiveDesired(context.Background(), olderPlacement, phase4Now().Add(time.Minute)); err != nil || changed {
		t.Fatalf("old placement changed=%v error=%v", changed, err)
	}
	if changed, err := module.ApplyObservation(context.Background(), Observation{MessageID: "evt_observed_2", RegionalDeploymentID: deployment.ID, Sequence: 2, State: ObservedRunning, ObservedAt: phase4Now()}); err != nil || !changed {
		t.Fatalf("new observation changed=%v error=%v", changed, err)
	}
	if changed, err := module.ApplyObservation(context.Background(), Observation{MessageID: "evt_observed_1", RegionalDeploymentID: deployment.ID, Sequence: 1, State: ObservedFailed, ObservedAt: phase4Now().Add(time.Minute)}); err != nil || changed {
		t.Fatalf("old observation changed=%v error=%v", changed, err)
	}
	got := module.Deployments(context.Background())[0]
	if got.PlacementVersion != 2 || got.ObservedState != ObservedRunning || got.ObservationSequence != 2 {
		t.Fatalf("deployment regressed: %#v", got)
	}
	if module.OutboxCount() != 1 {
		t.Fatalf("outbox count=%d, want 1", module.OutboxCount())
	}
}

func TestConcurrentSchedulingCannotReserveSameCapacity(t *testing.T) {
	module := newTestRegion(1000, 1024)
	first, _, _ := module.ReceiveDesired(context.Background(), desiredEvent("evt_desired_a", "lin_a"), phase4Now())
	second, _, _ := module.ReceiveDesired(context.Background(), desiredEvent("evt_desired_b", "lin_b"), phase4Now())
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, id := range []contract.RegionalDeploymentID{first.ID, second.ID} {
		wait.Add(1)
		go func(deploymentID contract.RegionalDeploymentID) {
			defer wait.Done()
			<-start
			_, err := module.Schedule(context.Background(), deploymentID, phase4Now())
			results <- err
		}(id)
	}
	close(start)
	wait.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful schedules=%d, want 1", succeeded)
	}
	capacity := module.Capacity(context.Background())
	if capacity.CPUReserved != 1000 || capacity.MemoryReservedMB != 1024 {
		t.Fatalf("capacity over/under reserved: %#v", capacity)
	}
}

func TestRegionReconcilesExistingDeploymentWithoutGlobalPlane(t *testing.T) {
	module := newTestRegion(2000, 4096)
	if _, _, err := module.ReceiveDesired(context.Background(), desiredEvent("evt_global_before_outage", "lin_outage"), phase4Now()); err != nil {
		t.Fatal(err)
	}
	// No global dependency is consulted after durable ingress has completed.
	module.Reconcile(context.Background(), phase4Now().Add(time.Minute))
	module.Reconcile(context.Background(), phase4Now().Add(2*time.Minute))
	if len(module.Tasks(context.Background())) != 1 {
		t.Fatalf("tasks=%d, want one durable reconcile task", len(module.Tasks(context.Background())))
	}
	if module.Deployments(context.Background())[0].NodeID == "" {
		t.Fatal("existing deployment was not scheduled during global outage")
	}
}

func TestSchedulerRecordsExplicitFilteringReasonAndLeaseRenewal(t *testing.T) {
	now := phase4Now()
	module := New("reg_test", []Node{{ID: "nod_expired", RegionID: "reg_test", State: NodeStale, Games: []string{"terraria"}, CPUCapacity: 2000, MemoryCapacityMB: 4096, LeaseUntil: now.Add(-time.Minute)}})
	deployment, _, _ := module.ReceiveDesired(context.Background(), desiredEvent("evt_filter", "lin_filter"), now)
	if _, err := module.Schedule(context.Background(), deployment.ID, now); err == nil {
		t.Fatal("expired Node was scheduled")
	}
	if got := module.Deployments(context.Background())[0].UnschedulableReason; got != UnschedulableNoReadyNodes {
		t.Fatalf("reason=%s", got)
	}
	if err := module.RenewLease(context.Background(), "nod_expired", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Schedule(context.Background(), deployment.ID, now); err != nil {
		t.Fatal(err)
	}
	if got := module.Nodes(context.Background())[0].State; got != NodeReady {
		t.Fatalf("renewed Node state=%s", got)
	}

	incompatible := New("reg_test", []Node{{ID: "nod_other", RegionID: "reg_test", State: NodeReady, Games: []string{"other"}, CPUCapacity: 2000, MemoryCapacityMB: 4096, LeaseUntil: now.Add(time.Hour)}})
	other, _, _ := incompatible.ReceiveDesired(context.Background(), desiredEvent("evt_game", "lin_game"), now)
	if _, err := incompatible.Schedule(context.Background(), other.ID, now); err == nil {
		t.Fatal("incompatible Node was scheduled")
	}
	if got := incompatible.Deployments(context.Background())[0].UnschedulableReason; got != UnschedulableIncompatibleGame {
		t.Fatalf("reason=%s", got)
	}

	full := newTestRegion(500, 512)
	large, _, _ := full.ReceiveDesired(context.Background(), desiredEvent("evt_capacity", "lin_capacity"), now)
	if _, err := full.Schedule(context.Background(), large.ID, now); err == nil {
		t.Fatal("undersized Node was scheduled")
	}
	if got := full.Deployments(context.Background())[0].UnschedulableReason; got != UnschedulableInsufficientCapacity {
		t.Fatalf("reason=%s", got)
	}
}

func TestPlacementOverrideHasNoFallbackAndRecordsAudit(t *testing.T) {
	now := phase4Now()
	module := New("reg_test", []Node{
		{ID: "nod_one", RegionID: "reg_test", Name: "One", State: NodeReady, Games: []string{"terraria"}, CPUCapacity: 2000, MemoryCapacityMB: 4096, LeaseUntil: now.Add(time.Hour)},
		{ID: "nod_full", RegionID: "reg_test", Name: "Full", State: NodeReady, Games: []string{"terraria"}, CPUCapacity: 500, MemoryCapacityMB: 512, LeaseUntil: now.Add(time.Hour)},
	})
	deployment, _, _ := module.ReceiveDesired(context.Background(), desiredEvent("evt_override", "lin_override"), now)
	initial, err := module.Schedule(context.Background(), deployment.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.OverridePlacement(context.Background(), "usr_operator", deployment.ID, "nod_full", "maintenance move", now); err != ErrOverrideUnavailable {
		t.Fatalf("override error=%v", err)
	}
	if got := module.Deployments(context.Background())[0].NodeID; got != initial.NodeID {
		t.Fatalf("failed override fell back or moved deployment to %s", got)
	}
	if _, err := module.OverridePlacement(context.Background(), "usr_operator", deployment.ID, "nod_one", "", now); err != ErrOverrideReason {
		t.Fatalf("empty reason error=%v", err)
	}
	if len(module.Audits(context.Background())) != 0 {
		t.Fatal("failed overrides wrote audit")
	}
}

func newTestRegion(cpu, memory int) *Module {
	now := phase4Now()
	return New("reg_test", []Node{{ID: "nod_test", RegionID: "reg_test", Name: "Test", State: NodeReady, Games: []string{"terraria"}, CPUCapacity: cpu, MemoryCapacityMB: memory, LeaseUntil: now.Add(time.Hour), LastHeartbeatAt: now}})
}

func desiredEvent(messageID contract.EventID, instanceID contract.LogicalInstanceID) DesiredDeployment {
	return DesiredDeployment{MessageID: messageID, WorkspaceID: "ws_test", LogicalInstanceID: instanceID, RegionID: "reg_test", PlacementVersion: 1, InstanceRevisionID: "rev_test", DesiredState: "running", GameKey: "terraria", CPUUnits: 1000, MemoryMegabytes: 1024}
}

func phase4Now() time.Time { return time.Date(2026, time.September, 10, 1, 0, 0, 0, time.UTC) }
