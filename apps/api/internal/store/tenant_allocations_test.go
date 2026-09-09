package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantAllocations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantAllocations(t, db)
}

func testTenantAllocations(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "allocations", Slug: "allocations"}
	if err := db.CreateOrganization(ctx, &org, "allocator"); err != nil {
		t.Fatal(err)
	}
	quota := domain.TenantQuota{OrganizationID: org.ID, MaxServers: 2, MaxCPUCores: 2, MaxMemoryMB: 2048, MaxStorageGB: 10}
	if err := db.UpdateTenantQuota(ctx, quota); err != nil {
		t.Fatal(err)
	}
	newServer := func(id string) domain.GameServer {
		return domain.GameServer{ID: id, OrganizationID: org.ID, Spec: domain.ServerSpec{Generation: 1, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024}}}
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			server := newServer(fmt.Sprintf("allocation-%d", i))
			results <- db.CreateAllocatedGameServer(ctx, "allocator", &server)
		}(i)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrQuotaExceeded) {
			t.Fatal(err)
		}
	}
	if accepted != 2 {
		t.Fatalf("accepted %d allocations, want 2", accepted)
	}
	usage, err := db.GetTenantUsage(ctx, org.ID)
	if err != nil || usage.TotalServers != 2 || usage.RunningServers != 0 || usage.UsedCPUCores != 2 || usage.UsedMemoryMB != 2048 {
		t.Fatalf("stopped reservations: %+v %v", usage, err)
	}
	servers, err := db.ListUserGameServers(ctx, "allocator")
	if err != nil {
		t.Fatal(err)
	}
	before := servers[0]
	after := before
	after.Spec.Generation++
	after.Spec.Resources.MemoryLimitMB = 2048
	if err := db.SaveAllocatedGameServer(ctx, "allocator", before, after); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("resize: %v", err)
	}
	if err := db.SaveGameServer(ctx, &after); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("legacy resize bypass: %v", err)
	}

	cpuResize := before
	cpuResize.Spec.Resources.CPULimitCores = 2
	if err := db.SaveAllocatedGameServer(ctx, "allocator", before, cpuResize); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("CPU resize: %v", err)
	}
	quota.MaxMemoryMB = 1024
	if err := db.UpdateTenantQuota(ctx, quota); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("quota reduction: %v", err)
	}
	after = before
	after.Spec.Generation++
	after.Spec.Resources.MemoryLimitMB = 512
	if err := db.SaveAllocatedGameServer(ctx, "allocator", before, after); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAllocatedGameServer(ctx, "allocator", before, before); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale config: %v", err)
	}
	unlimited := after
	unlimited.Spec.Resources.CPULimitCores = 0
	if err := db.SaveAllocatedGameServer(ctx, "allocator", after, unlimited); !errors.Is(err, ErrFiniteResourcesRequired) {
		t.Fatalf("unlimited: %v", err)
	}
	if err := db.DeleteGameServer(ctx, servers[1].ID); err != nil {
		t.Fatal(err)
	}
	var deleted struct {
		DesiredState  string
		IntentVersion int64
	}
	if err := db.db.Table("logical_servers").Select("desired_state", "intent_version").Where("id = ?", servers[1].ID).Take(&deleted).Error; err != nil || deleted.DesiredState != "deleted" || deleted.IntentVersion != 2 {
		t.Fatalf("logical deletion tombstone: %+v %v", deleted, err)
	}
	var revisionCount int64
	if err := db.db.Table("server_revisions").Where("server_id = ?", servers[1].ID).Count(&revisionCount).Error; err != nil || revisionCount != 1 {
		t.Fatalf("immutable revision history: %d %v", revisionCount, err)
	}
	replacement := newServer("allocation-replacement")
	if err := db.CreateAllocatedGameServer(ctx, "allocator", &replacement); err != nil {
		t.Fatal(err)
	}

	stored, err := db.GetGameServer(ctx, after.ID)
	if err != nil || stored.Spec.Resources.MemoryLimitMB != 512 {
		t.Fatalf("accepted resize not preserved: %+v %v", stored, err)
	}
	if err := db.RemoveOrganizationMember(ctx, org.ID, "allocator"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAllocatedGameServer(ctx, "allocator", after, after); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked writer: %v", err)
	}
	if err := db.db.WithContext(ctx).Delete(&domain.TenantQuota{}, "organization_id = ?", org.ID).Error; err != nil {
		t.Fatal(err)
	}
	missingQuota := newServer("missing-quota")
	if err := db.CreateAllocatedGameServer(ctx, "", &missingQuota); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing quota must fail closed: %v", err)
	}

}
