package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func testSchedulerProcess(t *testing.T, ctx context.Context, binary string, global *store.Store, admin *sql.DB, dsn, schema string, o options, key []byte, protector *configprotection.Protector, fingerprint *configprotection.Fingerprinter, spec instances.Specification, plaintext []byte, available *atomic.Bool) {
	t.Helper()
	if err := store.MigrateRegionalPostgres(ctx, dsn, "east"); err != nil {
		t.Fatal(err)
	}
	region, err := store.OpenRegionalPostgres(dsn, "east", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer region.Close()
	spec.Assets = nil
	spec.Configuration = instances.ProtectedConfiguration{}
	_, err = global.CreateEncryptedGlobalServer(ctx, "owner", instances.CreateRequest{OrganizationID: "org", Name: "scheduler-process", RegionID: "east", IdempotencyKey: "scheduler-process", Specification: spec}, plaintext, protector, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := global.ClaimOutbox(ctx, "east", 1, time.Minute)
	if err != nil || len(messages) != 1 {
		t.Fatal("scheduler event", err)
	}
	var event instances.RevisionAvailable
	if err := json.Unmarshal([]byte(messages[0].Payload), &event); err != nil {
		t.Fatal(err)
	}
	if err := region.RecordRevisionNotification(ctx, event); err != nil {
		t.Fatal(err)
	}
	claim, err := region.ClaimRevision(ctx, time.Minute)
	if err != nil || claim == nil {
		t.Fatal(err)
	}
	snapshot, err := global.GetRegionalRevision(ctx, "east", event)
	if err != nil {
		t.Fatal(err)
	}
	if err := region.SaveRevision(ctx, *claim, snapshot); err != nil {
		t.Fatal(err)
	}
	assets, err := region.ClaimAssets(ctx, time.Minute)
	if err != nil || assets == nil {
		t.Fatal(err)
	}
	if err := region.CompleteAssets(ctx, *assets); err != nil {
		t.Fatal(err)
	} // no asset references
	for _, id := range []string{"process-a", "process-b"} {
		if _, err := region.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: id, Name: id, Architecture: "amd64", CPU: 2, MemoryMB: 1024, Schedulable: true}, 0); err != nil {
			t.Fatal(err)
		}
		session, err := region.StartRegionalNodeSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := region.RecordRegionalNodeHeartbeat(ctx, id, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := region.ConfigureRegionalNodeAccess(ctx, regional.NodeAccessPolicy{OrganizationID: "org", NodeIDs: []string{"process-a", "process-b"}, Enabled: true}, 0); err != nil {
		t.Fatal(err)
	}
	keyring, err := json.Marshal(map[string]any{"active": "configuration", "keys": map[string][]byte{"configuration": key}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(path, keyring, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-region", "east", "-control-endpoint", o.endpoint, "-certificate", o.certificate, "-key", o.key, "-server-ca", o.ca, "-configuration-keys", path, "-architecture", "amd64", "-first-port", "32000", "-last-port", "32003", "-request-timeout", "1s", "-task-timeout", "2s", "-lease", "5s", "-retry-delay", "50ms", "-poll-interval", "10ms", "-max-heartbeat-age", "1m"}
	var output bytes.Buffer
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = append(os.Environ(), "GAMEPANEL_REGIONAL_DATABASE_URL="+dsn)
	command.Stdout = &output
	command.Stderr = &output
	available.Store(false)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			_ = command.Process.Kill()
			<-finished
		}
		available.Store(true)
	}()
	wait := func(check func() bool) {
		t.Helper()
		for !check() {
			select {
			case err := <-finished:
				stopped = true
				t.Fatalf("scheduler exited early: %v %s", err, output.String())
			case <-ctx.Done():
				t.Fatal("scheduler timeout")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	var status string
	var attempts, next int64
	read := func() {
		t.Helper()
		if err := admin.QueryRowContext(ctx, "SELECT scheduling_status,scheduling_attempts,scheduling_next_ms FROM "+schema+".regional_deployments WHERE server_id=$1", event.ServerID).Scan(&status, &attempts, &next); err != nil {
			t.Fatal(err)
		}
	}
	wait(func() bool { read(); return attempts > 0 && next > 0 })
	if status != "pending" {
		t.Fatal("offline global completed scheduling")
	}
	available.Store(true)
	wait(func() bool { read(); return status == "reserved" })
	var node, ports string
	if err := admin.QueryRowContext(ctx, "SELECT node_id,ports FROM "+schema+".regional_allocations WHERE server_id=$1 AND status='reserved'", event.ServerID).Scan(&node, &ports); err != nil {
		t.Fatal(err)
	}
	if node != "process-a" || !bytes.Contains([]byte(ports), []byte("32000")) {
		t.Fatalf("invalid process reservation: %s %s", node, ports)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		stopped = true
		if err != nil {
			t.Fatalf("scheduler stop: %v %s", err, output.String())
		}
	case <-ctx.Done():
		t.Fatal("scheduler failed to stop")
	}
	if bytes.Contains(output.Bytes(), []byte("test-through-mtls")) || bytes.Contains(output.Bytes(), keyring) {
		t.Fatal("scheduler logged configuration secrets")
	}
	t.Log("actual regional scheduler process: mutual TLS global intent, durable retry, provider/keyring/policy/port admission and graceful stop verified; Node heartbeats are fixtures")
}
