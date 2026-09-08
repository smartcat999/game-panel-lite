package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestCreditConsistency(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "credits.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testCreditConsistency(t, db)
}

func testCreditConsistency(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "credit-consistency", Slug: "credit-consistency"}
	if err := db.CreateOrganization(ctx, &org, "credit-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Model(&domain.Organization{}).Where("id = ?", org.ID).Update("credits", 0).Error; err != nil {
		t.Fatal(err)
	}
	run := func(n int, operation func() error) []error {
		var wg sync.WaitGroup
		results := make([]error, n)
		for i := range results {
			wg.Add(1)
			go func(i int) { defer wg.Done(); results[i] = operation() }(i)
		}
		wg.Wait()
		return results
	}
	assertBalance := func(want int64, rows int) {
		t.Helper()
		balance, err := db.GetOrganizationCredits(ctx, org.ID)
		if err != nil || balance != want {
			t.Fatalf("balance=%d want=%d err=%v", balance, want, err)
		}
		ledger, err := db.ListCreditTransactions(ctx, org.ID, 100)
		if err != nil || len(ledger) != rows {
			t.Fatalf("ledger rows=%d want=%d err=%v", len(ledger), rows, err)
		}
		var sum int64
		for _, row := range ledger {
			sum += row.Amount
		}
		if sum != balance {
			t.Fatalf("ledger sum=%d balance=%d", sum, balance)
		}
	}
	for _, err := range run(20, func() error { _, err := db.TopUpCredits(ctx, org.ID, 1, "fund", "admin"); return err }) {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertBalance(20, 20)
	accepted := 0
	for _, err := range run(30, func() error { _, err := db.DeductCredits(ctx, org.ID, 1, "test", "spend", "credit-owner"); return err }) {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrInsufficientCredits) {
			t.Fatal(err)
		}
	}
	if accepted != 20 {
		t.Fatalf("accepted %d deductions, want 20", accepted)
	}
	assertBalance(0, 40)
	quota := domain.TenantQuota{OrganizationID: org.ID, MaxServers: 1, MaxCPUCores: 1, MaxMemoryMB: 1024, MaxStorageGB: 10}
	if err := db.UpdateTenantQuota(ctx, quota); err != nil {
		t.Fatal(err)
	}
	server := func(id string) domain.GameServer {
		return domain.GameServer{ID: id, OrganizationID: org.ID, Spec: domain.ServerSpec{Generation: 1, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024}}}
	}
	empty := server("credit-empty")
	if err := db.CreateChargedGameServer(ctx, "credit-owner", "credit-owner", &empty, 10); !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("empty balance: %v", err)
	}
	var count int64
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", empty.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rolled back server count=%d err=%v", count, err)
	}
	assertBalance(0, 40)
	if _, err := db.TopUpCredits(ctx, org.ID, 20, "fund", "admin"); err != nil {
		t.Fatal(err)
	}
	denied := server("credit-denied")
	if err := db.CreateChargedGameServer(ctx, "outsider", "outsider", &denied, 10); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("authorization: %v", err)
	}
	assertBalance(20, 41)
	for i := 0; i < 2; i++ {
		instance := server(fmt.Sprintf("credit-server-%d", i))
		err := db.CreateChargedGameServer(ctx, "credit-owner", "credit-owner", &instance, 10)
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i == 1 && !errors.Is(err, ErrQuotaExceeded) {
			t.Fatalf("quota rejection: %v", err)
		}
	}
	assertBalance(10, 42)
	if record, err := db.TopUpCredits(ctx, org.ID, math.MaxInt64, "overflow", "admin"); !errors.Is(err, ErrCreditOverflow) || record != nil {
		t.Fatalf("overflow record=%v err=%v", record, err)
	}
	assertBalance(10, 42)
}
