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

func TestNodeAllocations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "node-allocations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testNodeAllocations(t, db)
}

func testNodeAllocations(t *testing.T, db *Store) {
	ctx := context.Background()
	for _, mode := range []string{"capacity", "port"} {
		t.Run(mode, func(t *testing.T) {
			node := domain.ComputeNode{ID: "reservation-" + mode, CPUCores: 2, MemoryTotalMB: 1024}
			expected := 2
			if mode == "port" {
				node.CPUCores = 8
				node.MemoryTotalMB = 8192
				expected = 1
			}
			if err := db.CreateComputeNode(ctx, &node); err != nil {
				t.Fatal(err)
			}
			var candidates []domain.GameServer
			for i := 0; i < 8; i++ {
				org := domain.Organization{ID: fmt.Sprintf("reservation-%s-%d", mode, i), Slug: fmt.Sprintf("reservation-%s-%d", mode, i)}
				if err := db.CreateOrganization(ctx, &org, "reservation-owner"); err != nil {
					t.Fatal(err)
				}
				quota := domain.TenantQuota{OrganizationID: org.ID, MaxServers: 8, MaxCPUCores: 8, MaxMemoryMB: 8192, MaxStorageGB: 10}
				if err := db.UpdateTenantQuota(ctx, quota); err != nil {
					t.Fatal(err)
				}
				port := 7000 + i
				if mode == "port" {
					port = 7777
				}
				candidates = append(candidates, domain.GameServer{ID: org.ID, OrganizationID: org.ID, NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}, Network: domain.ServerNetworkSpec{HostPort: port}}})
			}
			var wg sync.WaitGroup
			results := make(chan error, len(candidates))
			for _, candidate := range candidates {
				wg.Add(1)
				go func(instance domain.GameServer) {
					defer wg.Done()
					results <- db.CreateAllocatedGameServer(ctx, "reservation-owner", &instance)
				}(candidate)
			}
			wg.Wait()
			close(results)
			accepted := 0
			for err := range results {
				if err == nil {
					accepted++
				} else if !errors.Is(err, ErrNodeAllocationUnavailable) {
					t.Fatal(err)
				}
			}
			if accepted != expected {
				t.Fatalf("accepted %d want %d", accepted, expected)
			}
			var instances []domain.GameServer
			if err := db.db.Where("node_id = ?", node.ID).Find(&instances).Error; err != nil {
				t.Fatal(err)
			}
			if len(instances) != expected {
				t.Fatalf("persisted %d want %d", len(instances), expected)
			}
			if mode == "capacity" {
				before := instances[0]
				after := before
				after.Spec.Generation++
				after.Spec.Resources.MemoryLimitMB = 1024
				if err := db.SaveAllocatedGameServer(ctx, "reservation-owner", before, after); !errors.Is(err, ErrNodeAllocationUnavailable) {
					t.Fatalf("tenant resize overcommitted node: %v", err)
				}
				if err := db.SaveGameServer(ctx, &after); !errors.Is(err, ErrNodeAllocationUnavailable) {
					t.Fatalf("store resize overcommitted node: %v", err)
				}
				saved, err := db.GetGameServer(ctx, before.ID)
				if err != nil || saved.Spec.Resources != before.Spec.Resources || saved.Spec.Generation != before.Spec.Generation {
					t.Fatalf("failed resize changed instance: %+v %v", saved, err)
				}
			}
			// A deletion request must not release capacity or its still-bound port.
			for _, instance := range instances {
				instance.Spec.DesiredState = domain.DesiredDeleted
				if err := db.SaveGameServer(ctx, &instance); err != nil {
					t.Fatal(err)
				}
			}
			next := candidates[0]
			next.ID += "-extra"
			if mode == "capacity" {
				next.Spec.Network.HostPort = 9999
			}
			if err := db.CreateAllocatedGameServer(ctx, "reservation-owner", &next); !errors.Is(err, ErrNodeAllocationUnavailable) {
				t.Fatalf("released capacity before runtime deletion: %v", err)
			}
		})
	}
	t.Run("resize-versus-create", func(t *testing.T) {
		node := domain.ComputeNode{ID: "reservation-resize", CPUCores: 2, MemoryTotalMB: 4096}
		if err := db.CreateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
		makeInstance := func(id string) domain.GameServer {
			org := domain.Organization{ID: id, Slug: id}
			if err := db.CreateOrganization(ctx, &org, "reservation-owner"); err != nil {
				t.Fatal(err)
			}
			if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: id, MaxServers: 4, MaxCPUCores: 4, MaxMemoryMB: 8192, MaxStorageGB: 10}); err != nil {
				t.Fatal(err)
			}
			return domain.GameServer{ID: id, OrganizationID: id, NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
		}
		before := makeInstance("resize-existing")
		candidate := makeInstance("resize-new")
		if err := db.CreateAllocatedGameServer(ctx, "reservation-owner", &before); err != nil {
			t.Fatal(err)
		}
		after := before
		after.Spec.Generation++
		after.Spec.Resources.CPULimitCores = 2
		results := make(chan error, 2)
		go func() { results <- db.SaveAllocatedGameServer(ctx, "reservation-owner", before, after) }()
		go func() { results <- db.CreateAllocatedGameServer(ctx, "reservation-owner", &candidate) }()
		accepted := 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				accepted++
			} else if !errors.Is(err, ErrNodeAllocationUnavailable) {
				t.Fatal(err)
			}
		}
		if accepted != 1 {
			t.Fatalf("accepted %d competing allocations", accepted)
		}
		var instances []domain.GameServer
		if err := db.db.Where("node_id = ?", node.ID).Find(&instances).Error; err != nil {
			t.Fatal(err)
		}
		cpu := 0.0
		for _, instance := range instances {
			cpu += instance.Spec.Resources.CPULimitCores
		}
		if cpu != 2 {
			t.Fatalf("reserved cpu=%v want 2", cpu)
		}
	})

}
