package backupcontrol

import (
	"context"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

func TestBackupLifecycleIsIdempotentSequencedAndRegionBound(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	messages := messaging.New()
	module := New(messages)
	command := CreateCommand{Identity: contract.CommandIdentity{IdempotencyKey: "backup-one"}, WorkspaceID: "ws_test", LogicalInstanceID: "lin_test", RegionID: "reg_test", Kind: KindBackup}
	request, err := module.Create(context.Background(), command, now)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := module.Create(context.Background(), command, now.Add(time.Minute))
	if err != nil || repeated.ID != request.ID || messages.Count("backup.requested.v1") != 1 {
		t.Fatalf("idempotent create=%#v events=%d error=%v", repeated, messages.Count("backup.requested.v1"), err)
	}
	changed, err := module.ApplyObservation(context.Background(), Observation{MessageID: "evt_done", BackupRequestID: request.ID, Sequence: 2, Status: StatusCompleted, ObjectKey: request.ObjectKey, SizeBytes: 42, Checksum: "sha", ObservedAt: now.Add(time.Minute)})
	if err != nil || !changed {
		t.Fatalf("completion changed=%v error=%v", changed, err)
	}
	changed, err = module.ApplyObservation(context.Background(), Observation{MessageID: "evt_old", BackupRequestID: request.ID, Sequence: 1, Status: StatusFailed, ObservedAt: now.Add(2 * time.Minute)})
	if err != nil || changed || module.List(context.Background(), "ws_test")[0].Status != StatusCompleted {
		t.Fatalf("stale observation changed=%v requests=%#v error=%v", changed, module.List(context.Background(), "ws_test"), err)
	}
	if _, err := module.Create(context.Background(), CreateCommand{Identity: contract.CommandIdentity{IdempotencyKey: "restore-wrong-region"}, WorkspaceID: "ws_test", LogicalInstanceID: "lin_test", RegionID: "reg_other", Kind: KindRestore, SourceBackupRequestID: request.ID}, now); err != ErrCrossRegionRestore {
		t.Fatalf("cross-region restore error=%v", err)
	}
	restore, err := module.Create(context.Background(), CreateCommand{Identity: contract.CommandIdentity{IdempotencyKey: "restore-one"}, WorkspaceID: "ws_test", LogicalInstanceID: "lin_test", RegionID: "reg_test", Kind: KindRestore, SourceBackupRequestID: request.ID}, now)
	if err != nil || restore.ObjectKey != request.ObjectKey {
		t.Fatalf("restore=%#v error=%v", restore, err)
	}
}
