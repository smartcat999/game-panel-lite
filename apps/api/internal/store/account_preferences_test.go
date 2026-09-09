package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestAccountPreferencesAreScopedByAccount(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "preferences.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, account := range []domain.AdminAccount{
		{ID: "account-a", Username: "account_a", PasswordHash: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "account-b", Username: "account_b", PasswordHash: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	} {
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
	}

	defaults, err := db.GetAccountPreferences(ctx, "account-a")
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Locale != "zh" || defaults.Theme != "system" {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	if err := db.SaveAccountPreferences(ctx, domain.AccountPreferences{AccountID: "account-a", Locale: "en", Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	accountA, err := db.GetAccountPreferences(ctx, "account-a")
	if err != nil {
		t.Fatal(err)
	}
	accountB, err := db.GetAccountPreferences(ctx, "account-b")
	if err != nil {
		t.Fatal(err)
	}
	if accountA.Locale != "en" || accountA.Theme != "dark" {
		t.Fatalf("unexpected saved preferences: %+v", accountA)
	}
	if accountB.Locale != "zh" || accountB.Theme != "system" {
		t.Fatalf("preferences leaked between accounts: %+v", accountB)
	}
}
