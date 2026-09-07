package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestArtifactReferenceLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "refs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testArtifactReferenceLifecycle(t, db)
}

func testArtifactReferenceLifecycle(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "ref-space", Slug: "ref-space"}
	if err := db.CreateOrganization(ctx, &org, "ref-owner"); err != nil {
		t.Fatal(err)
	}
	target := domain.GameServer{ID: "ref-server", OrganizationID: org.ID, NodeID: "ref-node", ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	node := domain.ComputeNode{ID: target.NodeID, Token: "ref-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	item := domain.ModFile{ID: "ref-dependency", OrganizationID: org.ID, ProviderKey: target.ProviderKey, InstanceID: "unassigned", Source: "upload", FileName: "dep.tmod", ContentHash: strings.Repeat("a", 64), SizeBytes: 4}
	if err := db.CreateOwnedLibraryMod(ctx, "ref-owner", &item); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "ref-assignment", UID: "ref-uid", NodeID: target.NodeID, ServerID: target.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{{ID: item.ID, Path: "Mods/dep.tmod", SHA256: item.ContentHash, SizeBytes: item.SizeBytes, Revision: item.Revision}}}}}
	// Metadata changes between planning and publication invalidate the old plan.
	after := item
	after.ModName = "NewName"
	if err := db.SaveOwnedLibraryMod(ctx, "ref-owner", item, after); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishWorkloadAssignment(ctx, target, &assignment); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("published stale source: %v", err)
	}
	item.ModName = after.ModName
	item.Revision++
	assignment.Spec.Options.Artifacts[0].Revision = item.Revision
	if err := db.PublishWorkloadAssignment(ctx, target, &assignment); err != nil {
		t.Fatal(err)
	}
	leaseReq := ExecutionLeaseRequest{NodeID: node.ID, NodeToken: node.Token, AssignmentUID: assignment.UID, Generation: 1, HolderID: "ref-holder"}
	lease, err := db.AcquireExecutionLease(ctx, leaseReq, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ResolveArtifactForNode(ctx, target.NodeID, assignment.UID, 1, item.ID, leaseReq.HolderID, lease.Fence); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "ref-owner", item); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("deleted transitive reference: %v", err)
	}
	after = item
	after.Description = "edited"
	if err := db.SaveOwnedLibraryMod(ctx, "ref-owner", item, after); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("updated pinned revision: %v", err)
	}
	// New desired state alone cannot release an older persisted manifest.
	target.Spec.Generation++
	target.Spec.DesiredState = domain.DesiredStopped
	if err := db.SaveGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "ref-owner", item); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("desired change released manifest: %v", err)
	}
	assignment.Generation = target.Spec.Generation
	assignment.DesiredState = target.Spec.DesiredState
	assignment.Spec.Options.Artifacts = nil
	if err := db.PublishWorkloadAssignment(ctx, target, &assignment); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "ref-owner", item); err != nil {
		t.Fatal(err)
	}
	// Even a pre-built plan cannot republish bytes after deletion wins the lock.
	target.Spec.Generation++
	target.Spec.DesiredState = domain.DesiredRunning
	if err := db.SaveGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	assignment.Generation = target.Spec.Generation
	assignment.DesiredState = target.Spec.DesiredState
	assignment.Spec.Options.Artifacts = []workload.Artifact{{ID: item.ID, Path: "Mods/dep.tmod", SHA256: item.ContentHash, SizeBytes: item.SizeBytes, Revision: item.Revision}}
	if err := db.PublishWorkloadAssignment(ctx, target, &assignment); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("published deleted dependency: %v", err)
	}
	for i := 0; i < 3; i++ {
		candidate := item
		candidate.ID = fmt.Sprintf("ref-race-%d", i)
		candidate.FileName = candidate.ID + ".tmod"
		candidate.Revision = 0
		if err := db.CreateOwnedLibraryMod(ctx, "ref-owner", &candidate); err != nil {
			t.Fatal(err)
		}
		instance := target
		instance.ID = fmt.Sprintf("ref-race-server-%d", i)
		if err := db.CreateGameServer(ctx, &instance); err != nil {
			t.Fatal(err)
		}
		task := domain.WorkloadAssignment{ID: instance.ID, UID: instance.ID, ServerID: instance.ID, NodeID: instance.NodeID, Generation: instance.Spec.Generation, DesiredState: instance.Spec.DesiredState, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{{ID: candidate.ID, Path: "Mods/dep.tmod", SHA256: candidate.ContentHash, SizeBytes: candidate.SizeBytes, Revision: candidate.Revision}}}}}
		start := make(chan struct{})
		published, deleted := make(chan error, 1), make(chan error, 1)
		go func() { <-start; published <- db.PublishWorkloadAssignment(ctx, instance, &task) }()
		go func() { <-start; deleted <- db.DeleteOwnedLibraryMod(ctx, "ref-owner", candidate) }()
		close(start)
		publishErr, deleteErr := <-published, <-deleted
		if publishErr == nil {
			if !errors.Is(deleteErr, ErrInvalidModLibrary) {
				t.Fatalf("published source deleted: %v", deleteErr)
			}
		} else if deleteErr != nil || !errors.Is(publishErr, ErrInvalidModLibrary) {
			t.Fatalf("publication/delete race: %v / %v", publishErr, deleteErr)
		}
	}

}
