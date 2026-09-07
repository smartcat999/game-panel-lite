package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestNodeArtifactAuthorization(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "artifact-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testNodeArtifactAuthorization(t, db)
}
func testNodeArtifactAuthorization(t *testing.T, db *Store) {
	ctx := context.Background()
	target := domain.GameServer{ID: "artifact-auth-server", OrganizationID: "artifact-auth-space", NodeID: "artifact-auth-node", ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	org := domain.Organization{ID: target.OrganizationID, Slug: target.OrganizationID}
	if err := db.CreateOrganization(ctx, &org, "artifact-auth-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	item := domain.ModFile{ID: "artifact-auth-source", OrganizationID: target.OrganizationID, InstanceID: "unassigned", ProviderKey: target.ProviderKey, FileName: "source.tmod", Source: "upload", ContentHash: strings.Repeat("a", 64), SizeBytes: 10}
	if err := db.CreateMod(ctx, &item); err != nil {
		t.Fatal(err)
	}
	ref := workload.Artifact{ID: item.ID, Path: "Mods/source.tmod", SHA256: item.ContentHash, SizeBytes: item.SizeBytes}
	assignment := domain.WorkloadAssignment{ID: "artifact-auth-assignment", UID: "artifact-auth-uid", ServerID: target.ID, NodeID: target.NodeID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{ref}}}}
	if err := db.PublishWorkloadAssignment(ctx, target, &assignment); err != nil {
		t.Fatal(err)
	}
	resolve := func(node, uid string, generation int, id string) error {
		_, _, err := db.ResolveArtifactForNode(ctx, node, uid, generation, id)
		return err
	}
	if err := resolve(target.NodeID, assignment.UID, 1, item.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		node, uid  string
		generation int
		id         string
	}{{"foreign", assignment.UID, 1, item.ID}, {target.NodeID, "missing", 1, item.ID}, {target.NodeID, assignment.UID, 2, item.ID}, {target.NodeID, assignment.UID, 1, "missing"}} {
		if err := resolve(tc.node, tc.uid, tc.generation, tc.id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized request: %v", err)
		}
	}
	for _, field := range []string{"organization_id", "provider_key", "source", "content_hash", "size_bytes"} {
		value := any("foreign")
		if field == "size_bytes" {
			value = int64(11)
		}
		if err := db.db.Model(&domain.ModFile{}).Where("id = ?", item.ID).UpdateColumn(field, value).Error; err != nil {
			t.Fatal(err)
		}
		if err := resolve(target.NodeID, assignment.UID, 1, item.ID); err == nil {
			t.Fatalf("changed source %s authorized", field)
		}
		if err := db.SaveMod(ctx, &item); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", target.ID).UpdateColumn("organization_id", "another-space").Error; err != nil {
		t.Fatal(err)
	}
	if err := resolve(target.NodeID, assignment.UID, 1, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moved workspace: %v", err)
	}
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", target.ID).UpdateColumn("organization_id", target.OrganizationID).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range []domain.ServerDesiredState{domain.DesiredStopped, domain.DesiredDeleted} {
		if err := db.db.Model(&domain.WorkloadAssignment{}).Where("uid = ?", assignment.UID).UpdateColumn("desired_state", state).Error; err != nil {
			t.Fatal(err)
		}
		if err := resolve(target.NodeID, assignment.UID, 1, item.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("non-running assignment: %v", err)
		}
	}
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("uid = ?", assignment.UID).UpdateColumn("desired_state", domain.DesiredRunning).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", target.ID).UpdateColumn("node_id", "another-node").Error; err != nil {
		t.Fatal(err)
	}
	if err := resolve(target.NodeID, assignment.UID, 1, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moved node: %v", err)
	}
}
