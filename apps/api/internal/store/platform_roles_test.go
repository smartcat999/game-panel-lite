package store

import (
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteBackfillsLegacyPlatformRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec(`CREATE TABLE admin_accounts (id text PRIMARY KEY, username text, role text, password_hash text, created_at datetime, updated_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec(`INSERT INTO admin_accounts(id,username,role) VALUES ('legacy-admin','legacy-admin','admin'),('legacy-member','legacy-member','member')`).Error; err != nil {
		t.Fatal(err)
	}
	pool, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	admin, err := db.GetAdminAccount(t.Context(), "legacy-admin")
	if err != nil || admin.PlatformRole != domain.PlatformRoleAdmin {
		t.Fatalf("admin platform role: %+v %v", admin, err)
	}
	member, err := db.GetAdminAccount(t.Context(), "legacy-member")
	if err != nil || member.PlatformRole != domain.PlatformRoleUser {
		t.Fatalf("member platform role: %+v %v", member, err)
	}
	count, err := db.CountAdminRoleAccounts(t.Context())
	if err != nil || count != 1 {
		t.Fatalf("platform administrator count: %d %v", count, err)
	}
}

func TestExplicitPlatformRoleOverridesLegacyAccountRole(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "explicit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account := domain.AdminAccount{ID: "workspace-admin", Username: "workspace-admin", Role: domain.RoleAdmin, PlatformRole: domain.PlatformRoleUser}
	if err := db.CreateAdminAccount(t.Context(), &account); err != nil {
		t.Fatal(err)
	}
	account.Username = "renamed-workspace-admin"
	if err := db.SaveAdminAccount(t.Context(), &account); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetAdminAccount(t.Context(), account.ID)
	if err != nil || stored.PlatformRole != domain.PlatformRoleUser || domain.IsPlatformAdmin(stored) {
		t.Fatalf("explicit platform role lost: %+v %v", stored, err)
	}
}
