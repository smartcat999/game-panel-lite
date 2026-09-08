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

func TestAgentNodeReports(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testAgentNodeReports(t, db)
}
func testAgentNodeReports(t *testing.T, db *Store) {
	for _, kind := range []string{"heartbeat", "registration"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			before := domain.ComputeNode{ID: "report-" + kind, Token: "original-" + kind, Name: "old-name", Host: "old-host", PublicIP: "old-ip", CPUCores: 2, CPUUsagePercent: 3, LastHeartbeat: time.Now().Add(-time.Minute)}
			if err := db.CreateComputeNode(ctx, &before); err != nil {
				t.Fatal(err)
			}
			report := before
			report.Status = "online"
			report.LastHeartbeat = time.Now()
			report.UpdatedAt = report.LastHeartbeat
			report.WorkloadCapabilities = []string{workload.ArtifactCapability}
			report.CPUUsagePercent = 8
			report.CPUCores = 4
			report.RuntimeArchitecture = "arm64"
			apply := db.SaveAgentHeartbeat
			if kind == "registration" {
				apply = db.SaveAgentRegistration
			}
			// Configuration edited after the Agent read must survive its report.
			if err := db.db.Model(&domain.ComputeNode{}).Where("id = ?", before.ID).Updates(map[string]any{"name": "new-name", "host": "new-host", "public_ip": "new-ip"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := apply(ctx, before, report); err != nil {
				t.Fatal(err)
			}
			current, err := db.GetComputeNode(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.RuntimeArchitecture != "arm64" || current.Name != "new-name" || current.Host != "new-host" || current.PublicIP != "new-ip" || current.Token != before.Token || len(current.WorkloadCapabilities) != 1 {
				t.Fatalf("report overwrote configuration: %+v", current)
			}
			if kind == "heartbeat" && (current.CPUCores != 2 || current.CPUUsagePercent != 8) || kind == "registration" && (current.CPUCores != 4 || current.CPUUsagePercent != 3) {
				t.Fatalf("report crossed measurement ownership: %+v", current)
			}
			// A delayed older report cannot restore old capabilities or liveness.
			stale := report
			stale.LastHeartbeat = report.LastHeartbeat.Add(-time.Second)
			stale.WorkloadCapabilities = nil
			if err := apply(ctx, before, stale); !errors.Is(err, ErrReconciliationSuperseded) {
				t.Fatalf("older report accepted: %v", err)
			}
			if err := db.db.Model(&domain.ComputeNode{}).Where("id = ?", before.ID).UpdateColumn("token", "rotated").Error; err != nil {
				t.Fatal(err)
			}
			report.LastHeartbeat = time.Now()
			if err := apply(ctx, before, report); !errors.Is(err, ErrReconciliationSuperseded) {
				t.Fatalf("rotated token accepted: %v", err)
			}
			current, err = db.GetComputeNode(ctx, before.ID)
			if err != nil || current.Token != "rotated" {
				t.Fatalf("rotation undone: %v", err)
			}
			if err := db.DeleteComputeNode(ctx, before.ID); err != nil {
				t.Fatal(err)
			}
			if err := apply(ctx, before, report); !errors.Is(err, ErrReconciliationSuperseded) {
				t.Fatalf("deleted report accepted: %v", err)
			}
			if _, err := db.GetComputeNode(ctx, before.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("node resurrected: %v", err)
			}
			// Explicitly race a report against revocation; either ordering preserves it.
			before.ID += "-race"
			before.Token = "race-token"
			before.LastHeartbeat = time.Now().Add(-time.Minute)
			if err := db.CreateComputeNode(ctx, &before); err != nil {
				t.Fatal(err)
			}
			report = before
			report.LastHeartbeat = time.Now()
			report.Status = "online"
			start := make(chan struct{})
			reported, revoked := make(chan error, 1), make(chan error, 1)
			go func() { <-start; reported <- apply(ctx, before, report) }()
			go func() {
				<-start
				revoked <- db.db.Model(&domain.ComputeNode{}).Where("id = ?", before.ID).UpdateColumn("token", "revoked-race").Error
			}()
			close(start)
			reportErr, revokeErr := <-reported, <-revoked
			if revokeErr != nil || reportErr != nil && !errors.Is(reportErr, ErrReconciliationSuperseded) {
				t.Fatalf("race failed: %v %v", reportErr, revokeErr)
			}
			current, err = db.GetComputeNode(ctx, before.ID)
			if err != nil || current.Token != "revoked-race" {
				t.Fatalf("race restored credential: %v", err)
			}
		})
	}
}
