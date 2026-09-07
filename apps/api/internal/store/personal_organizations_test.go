package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func TestPersonalOrganizations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPersonalOrganizations(t, db)
}

func testPersonalOrganizations(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"personal-a", "personal-b"} {
		account := domain.AdminAccount{ID: id, Username: id, Role: domain.RoleMember, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := db.CreateAccountWithPersonalOrganization(ctx, &account); err != nil {
			t.Fatal(err)
		}
		orgs, err := db.ListUserOrganizations(ctx, id)
		if err != nil || len(orgs) != 1 {
			t.Fatalf("personal workspace: %+v %v", orgs, err)
		}
		member, err := db.GetOrganizationMember(ctx, orgs[0].ID, id)
		if err != nil || member.Role != domain.RoleOwner {
			t.Fatalf("owner: %+v %v", member, err)
		}
		var count int64
		if err := db.db.Model(&domain.TenantQuota{}).Where("organization_id = ?", orgs[0].ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("quota count: %d %v", count, err)
		}
	}
	a, _ := db.ListUserOrganizations(ctx, "personal-a")
	if _, err := db.GetUserOrganization(ctx, "personal-b", a[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant read: %v", err)
	}
	if orgs, err := db.ListUserOrganizations(ctx, ""); err != nil || len(orgs) != 0 {
		t.Fatalf("empty principal: %+v %v", orgs, err)
	}
	if err := db.RemoveOrganizationMember(ctx, a[0].ID, "personal-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserOrganization(ctx, "personal-a", a[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked membership: %v", err)
	}

	rejected := errors.New("quota persistence rejected")
	const callback = "test:reject_personal_quota"
	if err := db.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tenant_quota" {
			tx.AddError(rejected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.db.Callback().Create().Remove(callback)
	account := domain.AdminAccount{ID: "personal-rollback", Username: "personal-rollback", Role: domain.RoleMember}
	if err := db.CreateAccountWithPersonalOrganization(ctx, &account); !errors.Is(err, rejected) {
		t.Fatalf("expected quota failure: %v", err)
	}
	if _, err := db.GetAdminAccount(ctx, account.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial account retained: %v", err)
	}
	var count int64
	if err := db.db.Model(&domain.Organization{}).Where("name = ?", account.Username+"'s workspace").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial workspace retained: %d %v", count, err)
	}
	if err := db.db.Model(&domain.OrganizationMember{}).Where("user_id = ?", account.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial member retained: %d %v", count, err)
	}
}
