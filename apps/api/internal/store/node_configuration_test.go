package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestNodeConfiguration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "configuration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testNodeConfiguration(t, db)
}

func testNodeConfiguration(t *testing.T, db *Store) {
	ctx := context.Background()
	before := domain.ComputeNode{ID: "configuration-node", Token: "configuration-token", Name: "old", Region: "old-region", Host: "old-host", LastHeartbeat: time.Now().UTC().Add(-time.Minute)}
	if err := db.CreateComputeNode(ctx, &before); err != nil {
		t.Fatal(err)
	}
	report := before
	report.LastHeartbeat = time.Now().UTC().Truncate(time.Microsecond)
	report.Status = "online"
	report.CPUUsagePercent = 37
	report.WorkloadCapabilities = []string{workload.ArtifactCapability}
	name, host, empty := "new-name", "new-host", ""
	// Both independent edits and the Agent report may arrive in either order.
	start := make(chan struct{})
	results := make(chan error, 3)
	go func() {
		<-start
		_, err := db.UpdateNodeConfiguration(ctx, before.ID, NodeConfigurationPatch{Name: &name, Region: &empty})
		results <- err
	}()
	go func() {
		<-start
		_, err := db.UpdateNodeConfiguration(ctx, before.ID, NodeConfigurationPatch{Host: &host})
		results <- err
	}()
	go func() { <-start; results <- db.SaveAgentHeartbeat(ctx, before, report) }()
	close(start)
	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	current, err := db.GetComputeNode(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != name || current.Host != host || current.Region != "" || current.Token != before.Token || current.CPUUsagePercent != 37 || len(current.WorkloadCapabilities) != 1 || !current.LastHeartbeat.Equal(report.LastHeartbeat) {
		t.Fatalf("configuration/report fields lost: %+v", current)
	}
	if err := db.db.Model(&domain.ComputeNode{}).Where("id = ?", before.ID).UpdateColumn("token", "configuration-rotated").Error; err != nil {
		t.Fatal(err)
	}
	current, err = db.UpdateNodeConfiguration(ctx, before.ID, NodeConfigurationPatch{Name: &name})
	if err != nil || current.Token != "configuration-rotated" {
		t.Fatalf("token rotation lost: %v", err)
	}
	if err := db.DeleteComputeNode(ctx, before.ID); err != nil {
		t.Fatal(err)
	}
	for _, patch := range []NodeConfigurationPatch{{Name: &name}, {}} {
		if _, err := db.UpdateNodeConfiguration(ctx, before.ID, patch); !errors.Is(err, ErrNotFound) {
			t.Fatalf("deleted node accepted: %v", err)
		}
	}
	if _, err := db.GetComputeNode(ctx, before.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("node recreated: %v", err)
	}
}
