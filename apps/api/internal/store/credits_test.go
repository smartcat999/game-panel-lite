package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestCreditsTopUpAndDeduct(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "credits.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	org := domain.Organization{
		ID:        "org-test-credits",
		Name:      "Test Org",
		Slug:      "test-org",
		Plan:      "starter",
		Credits:   50,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateOrganization(ctx, &org, "user-owner"); err != nil {
		t.Fatal(err)
	}

	// 1. Check initial credits
	bal, err := db.GetOrganizationCredits(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 50 {
		t.Fatalf("expected 50 credits, got %d", bal)
	}

	// 2. Top-up 100 credits
	txTopup, err := db.TopUpCredits(ctx, org.ID, 100, "管理员手动充值测试", "admin-user")
	if err != nil {
		t.Fatal(err)
	}
	if txTopup.Amount != 100 || txTopup.BalanceAfter != 150 {
		t.Fatalf("expected balance 150, got %+v", txTopup)
	}

	// 3. Deduct 30 credits (e.g. server creation)
	txDeduct, err := db.DeductCredits(ctx, org.ID, 30, "server_create", "创建幻兽帕鲁服务器", "user-owner")
	if err != nil {
		t.Fatal(err)
	}
	if txDeduct.Amount != -30 || txDeduct.BalanceAfter != 120 {
		t.Fatalf("expected balance 120, got %+v", txDeduct)
	}

	// 4. Overdraft attempt (try to deduct 500 when balance is 120)
	_, err = db.DeductCredits(ctx, org.ID, 500, "server_create", "超额扣减", "user-owner")
	if err == nil {
		t.Fatalf("expected insufficient credits error, got nil")
	}

	// 5. Check transactions list
	list, err := db.ListCreditTransactions(ctx, org.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(list))
	}
}

func TestOAuthIdentityAndUserCreation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "oauth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	identity := domain.OAuthIdentity{
		Provider:       "github",
		ProviderUserID: "987654",
		Email:          "gamer@github.com",
		Name:           "GitHub Gamer",
		AvatarURL:      "https://avatars.githubusercontent.com/u/987654",
	}

	account, org, err := db.CreateUserWithOAuth(ctx, &identity, "github_gamer")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil || org == nil {
		t.Fatalf("expected account and org created")
	}
	if org.Credits != 100 {
		t.Fatalf("expected 100 starter credits, got %d", org.Credits)
	}

	// Find by provider & provider_user_id
	found, err := db.FindOAuthIdentity(ctx, "github", "987654")
	if err != nil {
		t.Fatal(err)
	}
	if found.UserID != account.ID || found.Email != "gamer@github.com" {
		t.Fatalf("found mismatch: %+v", found)
	}
}
