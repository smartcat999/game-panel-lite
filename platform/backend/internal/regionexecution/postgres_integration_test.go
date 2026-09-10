package regionexecution

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestPostgresRegionalDurabilityAndConcurrentReservation(t *testing.T) {
	database := openRegionPostgresTestDatabase(t)
	ctx := context.Background()
	now := phase4Now()
	region := NewPostgres(database, "reg_test")

	firstDesired := desiredEvent("evt_durable", "lin_durable")
	firstDesired.CPUUnits, firstDesired.MemoryMegabytes = 500, 512
	first, changed, err := region.ReceiveDesired(ctx, firstDesired, now)
	if err != nil || !changed {
		t.Fatalf("first ingress changed=%v error=%v", changed, err)
	}
	restarted := NewPostgres(database, "reg_test")
	repeated, changed, err := restarted.ReceiveDesired(ctx, firstDesired, now.Add(time.Minute))
	if err != nil || changed || repeated.ID != first.ID {
		t.Fatalf("durable redelivery id=%s changed=%v error=%v", repeated.ID, changed, err)
	}

	secondDesired := desiredEvent("evt_compete", "lin_compete")
	secondDesired.CPUUnits, secondDesired.MemoryMegabytes = 500, 512
	second, _, err := region.ReceiveDesired(ctx, secondDesired, now)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, deploymentID := range []contract.RegionalDeploymentID{first.ID, second.ID} {
		wait.Add(1)
		go func(id contract.RegionalDeploymentID) {
			defer wait.Done()
			<-start
			_, err := region.Schedule(ctx, id, now)
			results <- err
		}(deploymentID)
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
		t.Fatalf("successful concurrent reservations=%d, want 1", succeeded)
	}
	capacity := region.Capacity(ctx)
	if capacity.CPUReserved != 500 || capacity.MemoryReservedMB != 512 {
		t.Fatalf("reserved capacity=%#v", capacity)
	}
	assertRegionTableCount(t, database, "reservations", 1)
	assertRegionTableCount(t, database, "regional_tasks", 1)

	if changed, err := region.ApplyObservation(ctx, Observation{MessageID: "evt_obs_2", RegionalDeploymentID: first.ID, Sequence: 2, State: ObservedRunning, ObservedAt: now}); err != nil || !changed {
		t.Fatalf("new observation changed=%v error=%v", changed, err)
	}
	if changed, err := region.ApplyObservation(ctx, Observation{MessageID: "evt_obs_1", RegionalDeploymentID: first.ID, Sequence: 1, State: ObservedFailed, ObservedAt: now.Add(time.Minute)}); err != nil || changed {
		t.Fatalf("old observation changed=%v error=%v", changed, err)
	}
	if deployment := deploymentFromList(region.Deployments(ctx), first.ID); deployment.ObservedState != ObservedRunning || deployment.ObservationSequence != 2 {
		t.Fatalf("deployment regressed: %#v", deployment)
	}
	assertRegionTableCount(t, database, "regional_outbox", 1)
}

func TestPostgresAssignmentRestartAndBackupCompletionAreDurable(t *testing.T) {
	database := openRegionPostgresTestDatabase(t)
	ctx := context.Background()
	now := phase4Now()
	region := NewPostgres(database, "reg_test")
	desired := desiredEvent("evt_phase5_durable", "lin_phase5")
	desired.CPUUnits, desired.MemoryMegabytes = 500, 512
	deployment, _, err := region.ReceiveDesired(ctx, desired, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := region.Schedule(ctx, deployment.ID, now); err != nil {
		t.Fatal(err)
	}
	initial := region.PollAssignments(ctx, "nod_test", 1, now)
	if len(initial) != 1 {
		t.Fatalf("initial assignments=%#v", initial)
	}
	if _, err := region.ClaimAssignment(ctx, initial[0].ID, "nod_test", now.Add(time.Second), now); err != nil {
		t.Fatal(err)
	}
	restarted := NewPostgres(database, "reg_test")
	if polled := restarted.PollAssignments(ctx, "nod_test", 1, now.Add(500*time.Millisecond)); len(polled) != 0 {
		t.Fatalf("live lease was reclaimed: %#v", polled)
	}
	reclaimed := restarted.PollAssignments(ctx, "nod_test", 1, now.Add(2*time.Second))
	if len(reclaimed) != 1 {
		t.Fatalf("expired lease was not reclaimed: %#v", reclaimed)
	}
	claimed, err := restarted.ClaimAssignment(ctx, reclaimed[0].ID, "nod_test", now.Add(time.Minute), now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if completed, err := restarted.CompleteAssignment(ctx, claimed.ID, "nod_test", claimed.FencingToken, true, now.Add(3*time.Second)); err != nil || !completed {
		t.Fatalf("reclaimed completion=%v error=%v", completed, err)
	}
	backupAssignment, changed, err := restarted.ReceiveBackup(ctx, BackupRequested{MessageID: "evt_backup_requested", BackupRequestID: "bkr_region", LogicalInstanceID: "lin_phase5", RegionID: "reg_test", Kind: "backup", ObjectKey: "regions/reg_test/backup.tar.gz", TransferURL: "https://objects.invalid/signed", RelativePath: "instances/lin_phase5/world"}, now.Add(4*time.Second))
	if err != nil || !changed {
		t.Fatalf("backup assignment=%#v changed=%v error=%v", backupAssignment, changed, err)
	}
	claimedBackup, err := restarted.ClaimAssignment(ctx, backupAssignment.ID, "nod_test", now.Add(time.Minute), now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	result := BackupResult{BackupRequestID: "bkr_region", Sequence: 1, Status: "completed", ObjectKey: "regions/reg_test/backup.tar.gz", SizeBytes: 128, Checksum: "sha", ObservedAt: now.Add(5 * time.Second)}
	if changed, err := restarted.RecordBackupResult(ctx, claimedBackup, result); err != nil || !changed {
		t.Fatalf("backup result changed=%v error=%v", changed, err)
	}
	if changed, err := restarted.RecordBackupResult(ctx, claimedBackup, result); err != nil || changed {
		t.Fatalf("backup result duplicated changed=%v error=%v", changed, err)
	}
	assertRegionTableCount(t, database, "region_backup_results", 1)
}

func openRegionPostgresTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_REGION_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_REGION_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase4_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	database, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	_, filename, _, _ := runtime.Caller(0)
	for _, name := range []string{"0001_region_execution.sql", "0002_node_assignments_and_backups.sql"} {
		migration, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "region", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(migration), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := database.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	games := []byte(`["terraria"]`)
	if _, err := database.Exec(`INSERT INTO nodes (id, region_id, name, state, games, cpu_capacity, memory_capacity_mb, lease_until, last_heartbeat_at) VALUES ('nod_test', 'reg_test', 'Test', 'ready', $1, 500, 512, $2, $3)`, games, phase4Now().Add(time.Hour), phase4Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close(); _, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); admin.Close() })
	return database
}

func deploymentFromList(items []RegionalDeployment, id contract.RegionalDeploymentID) RegionalDeployment {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return RegionalDeployment{}
}
func assertRegionTableCount(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT count(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count=%d, want %d", table, got, want)
	}
}
