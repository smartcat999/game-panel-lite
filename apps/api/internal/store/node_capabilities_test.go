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

func TestRemoteInstallationCapabilities(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "node-cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testRemoteInstallationCapabilities(t, db)
}
func testRemoteInstallationCapabilities(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "cap-space", Slug: "cap-space"}
	if err := db.CreateOrganization(ctx, &org, "cap-owner"); err != nil {
		t.Fatal(err)
	}
	node := domain.ComputeNode{ID: "cap-node", Status: "online", LastHeartbeat: time.Now(), WorkloadCapabilities: []string{workload.ArtifactCapability}}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	before := domain.GameServer{ID: "cap-server", NodeID: node.ID, OrganizationID: org.ID, ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
	if err := db.CreateGameServer(ctx, &before); err != nil {
		t.Fatal(err)
	}
	source := domain.ModFile{ID: "cap-source", OrganizationID: org.ID, InstanceID: "unassigned", ProviderKey: before.ProviderKey, FileName: "cap.tmod", Source: "upload"}
	if err := db.CreateOwnedLibraryMod(ctx, "cap-owner", &source); err != nil {
		t.Fatal(err)
	}
	after := before
	after.Spec.Generation++
	after.Spec.ModIDs = []string{source.ID}
	for _, mode := range []string{"legacy", "stale", "offline", "future", "lookalike", "ready"} {
		node.Status = "online"
		node.LastHeartbeat = time.Now()
		node.WorkloadCapabilities = []string{workload.ArtifactCapability}
		switch mode {
		case "legacy":
			node.WorkloadCapabilities = nil
		case "stale":
			node.LastHeartbeat = time.Now().Add(-time.Minute)
		case "offline":
			node.Status = "offline"
		case "future":
			node.LastHeartbeat = time.Now().Add(time.Hour)
		case "lookalike":
			node.WorkloadCapabilities = []string{"artifacts-v10"}
		}
		if err := db.UpdateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
		ready, err := db.RemoteArtifactsAvailable(ctx, node.ID)
		if err != nil || ready != (mode == "ready") {
			t.Fatalf("%s readiness: %v %v", mode, ready, err)
		}
		err = db.SaveModInstallationIntent(ctx, "cap-owner", before, after, source)
		if mode == "ready" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatalf("%s commit accepted: %v", mode, err)
		}
	}
}
