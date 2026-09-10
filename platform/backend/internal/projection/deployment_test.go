package projection

import (
	"context"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

func TestProjectionRejectsDuplicateAndOutOfOrderObservations(t *testing.T) {
	instances := instancecontrol.New()
	consumer := New(messaging.New(), instances)
	now := time.Date(2026, time.September, 10, 2, 0, 0, 0, time.UTC)
	newer := DeploymentObserved{MessageID: "evt_newer", LogicalInstanceID: "lin_test", RegionalDeploymentID: "rdp_test", RegionID: "reg_test", Sequence: 2, ObservedState: "running", ObservedAt: now}
	if handled, err := consumer.Consume(context.Background(), newer, now); err != nil || !handled {
		t.Fatalf("newer handled=%v error=%v", handled, err)
	}
	if handled, err := consumer.Consume(context.Background(), newer, now); err != nil || handled {
		t.Fatalf("duplicate handled=%v error=%v", handled, err)
	}
	older := newer
	older.MessageID = "evt_older"
	older.Sequence = 1
	older.ObservedState = "failed"
	if handled, err := consumer.Consume(context.Background(), older, now); err != nil || !handled {
		t.Fatalf("older envelope handled=%v error=%v", handled, err)
	}
	summary, ok := instances.DeploymentSummary(context.Background(), "lin_test")
	if !ok || summary.Sequence != 2 || summary.ObservedState != "running" {
		t.Fatalf("projection regressed: %#v", summary)
	}
}
