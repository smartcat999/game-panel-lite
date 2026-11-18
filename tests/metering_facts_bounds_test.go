package tests

import (
	"errors"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	meteringmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering"
)

func TestMeteringFactsMultiStopSameGenerationAndSkewBounds(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-multi-stop", "1000m")
	t3 := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Millisecond)
	t2 := t3.Add(-10 * time.Minute)
	t1 := t2.Add(-10 * time.Minute)
	t0 := t1.Add(-10 * time.Minute)

	// Cycle 1: Running -> Stopped in generation 1 without explicit TransitionID.
	cur, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildRunningObservation(inst, t0, t0, "", nil))
	if err != nil {
		t.Fatalf("cycle 1 Running: %v", err)
	}
	cur, err = env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildPhaseObservation(cur, corev1.InstancePhaseStopped, &t0, t1, "", nil))
	if err != nil {
		t.Fatalf("cycle 1 Stopped: %v", err)
	}
	// Cycle 2: Restart and second Stopped in the same Generation=1 with distinct StartedAt.
	cur, err = env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildRunningObservation(cur, t2, t2, "", nil))
	if err != nil {
		t.Fatalf("cycle 2 Running: %v", err)
	}
	cur, err = env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildPhaseObservation(cur, corev1.InstancePhaseStopped, &t2, t3, "", nil))
	if err != nil {
		t.Fatalf("cycle 2 Stopped: %v", err)
	}
	if got := countExecutionFactRows(t, env.ctx, env.reg, cur.Metadata.UID, meteringmodel.FactInstanceRunning); got != 2 {
		t.Fatalf("expected 2 Running facts across two cycles, got %d", got)
	}
	if got := countExecutionFactRows(t, env.ctx, env.reg, cur.Metadata.UID, meteringmodel.FactInstanceStopped); got != 2 {
		t.Fatalf("expected 2 Stopped facts across two cycles, got %d", got)
	}
	futureObs := buildRunningObservation(cur, time.Now().UTC(), time.Now().UTC().Add(5*time.Minute), "future-skew", nil)
	if _, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, futureObs); !errors.Is(err, models.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for +5m future ObservedAt, got %v", err)
	}
}

func TestMeteringFactsOutOfOrderPriorResolution(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-out-of-order", "1000m")
	t1 := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
	t2 := t1.Add(30 * time.Minute)
	t3 := t1.Add(2 * time.Hour)

	cur, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildRunningObservation(inst, t1, t1, "", nil))
	if err != nil {
		t.Fatalf("Running at t1: %v", err)
	}
	curStopped, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, buildPhaseObservation(cur, corev1.InstancePhaseStopped, &t1, t3, "", nil))
	if err != nil {
		t.Fatalf("Stopped at t3: %v", err)
	}
	dupFacts, err := meteringmodel.PrepareObservationFacts(env.ctx, env.reg, curStopped, buildRunningObservation(curStopped, t1, t2, "", nil), t2, "")
	if err != nil || len(dupFacts) != 0 {
		t.Fatalf("expected out-of-order Running at t2 after t1 Running to emit 0 facts, got len=%d err=%v fact=%+v", len(dupFacts), err, dupFacts)
	}

	approved4c := approveResizeToCPU(t, env, curStopped, "4000m")
	resizeObs := buildRunningObservation(approved4c, t1, t2.Add(30*time.Minute), "", nil)
	resizeFacts, err := meteringmodel.PrepareObservationFacts(env.ctx, env.reg, approved4c, resizeObs, *resizeObs.Status.ObservedAt, "")
	if err != nil || len(resizeFacts) != 1 || resizeFacts[0].Entry.FactType != meteringmodel.FactInstanceResized {
		t.Fatalf("expected out-of-order Running at t2 after 4000m resize to emit Resized, got facts=%+v err=%v", resizeFacts, err)
	}
}
