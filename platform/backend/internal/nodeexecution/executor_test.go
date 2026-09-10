package nodeexecution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

func TestExecutorUploadsAndRecordsBackupBeforeTerminalAssignment(t *testing.T) {
	region, now := assignedRegion(t)
	assignment, changed, err := region.ReceiveBackup(context.Background(), regionexecution.BackupRequested{MessageID: "evt_backup", BackupRequestID: "bkr_test", LogicalInstanceID: "lin_test", RegionID: "reg_test", Kind: "backup", ObjectKey: "regions/reg_test/backup.tar.gz", TransferURL: "memory://backup", RelativePath: "instances/lin_test/world"}, now)
	if err != nil || !changed {
		t.Fatalf("receive changed=%v error=%v", changed, err)
	}
	rootPath := t.TempDir()
	root, _ := NewScopedRoot(rootPath)
	if err := os.MkdirAll(filepath.Join(rootPath, "instances/lin_test/world"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "instances/lin_test/world/world.wld"), []byte("world"), 0o640); err != nil {
		t.Fatal(err)
	}
	transfer := &memoryTransfer{}
	executor := Executor{Root: root, Transfer: transfer, Results: region, Clock: func() time.Time { return now.Add(time.Second) }}
	agent := Agent{NodeID: assignment.NodeID, BatchSize: 2, ClaimTTL: time.Minute, Store: region, Reconcile: executor.Reconcile}
	processed, err := agent.RunOnce(context.Background(), now)
	if err != nil || processed != 2 || transfer.uploads != 1 {
		t.Fatalf("processed=%d uploads=%d error=%v", processed, transfer.uploads, err)
	}
	results := region.BackupResults(context.Background())
	if len(results) != 1 || results[0].Status != "completed" || results[0].SizeBytes == 0 {
		t.Fatalf("results=%#v", results)
	}
	processed, err = agent.RunOnce(context.Background(), now.Add(2*time.Second))
	if err != nil || processed != 0 || transfer.uploads != 1 {
		t.Fatalf("terminal effect duplicated processed=%d uploads=%d error=%v", processed, transfer.uploads, err)
	}
}
