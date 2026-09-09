package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestOrganizationInvitations(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "invitations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create owner user and organization
	owner := domain.AdminAccount{
		ID:        "user-owner",
		Username:  "server_boss",
		Role:      domain.RoleMember,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.CreateAdminAccount(ctx, &owner); err != nil {
		t.Fatal(err)
	}

	org := domain.Organization{
		ID:        "org-squad-1",
		Name:      "Squad Alpha",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateOrganization(ctx, &org, owner.ID); err != nil {
		t.Fatal(err)
	}

	t.Run("create and get summary", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleMember,
			MaxUses:        2,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatalf("failed to create invitation: %v", err)
		}
		if invite.Token == "" {
			t.Fatalf("expected generated token")
		}

		summary, err := db.GetOrganizationInvitationSummary(ctx, invite.Token)
		if err != nil {
			t.Fatalf("failed to get summary: %v", err)
		}
		if summary.OrganizationName != "Squad Alpha" {
			t.Fatalf("expected org name 'Squad Alpha', got %s", summary.OrganizationName)
		}
		if summary.InviterName != "server_boss" {
			t.Fatalf("expected inviter name 'server_boss', got %s", summary.InviterName)
		}
		if summary.IsExpired {
			t.Fatalf("expected invitation not to be expired")
		}
	})

	t.Run("accept invitation creates membership", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleMember,
			MaxUses:        5,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatal(err)
		}

		member, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "friend-1")
		if err != nil {
			t.Fatalf("expected successful accept: %v", err)
		}
		if member.UserID != "friend-1" || member.OrganizationID != org.ID || member.Role != domain.RoleMember {
			t.Fatalf("unexpected member: %+v", member)
		}

		// Verify member can be fetched from store
		fetched, err := db.GetOrganizationMember(ctx, org.ID, "friend-1")
		if err != nil {
			t.Fatalf("failed to get member: %v", err)
		}
		if fetched.Role != domain.RoleMember {
			t.Fatalf("expected role member, got %s", fetched.Role)
		}

		// Idempotent accept
		idempotentMember, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "friend-1")
		if err != nil {
			t.Fatalf("expected idempotent accept: %v", err)
		}
		if idempotentMember.UserID != "friend-1" {
			t.Fatalf("expected friend-1, got %s", idempotentMember.UserID)
		}
	})

	t.Run("expired invitation is rejected", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleViewer,
			MaxUses:        1,
			ExpiresAt:      time.Now().UTC().Add(-1 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatal(err)
		}

		_, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "late-friend")
		if !errors.Is(err, ErrInvitationExpired) {
			t.Fatalf("expected ErrInvitationExpired, got %v", err)
		}
	})

	t.Run("revoked invitation is rejected", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleMember,
			MaxUses:        1,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatal(err)
		}

		if err := db.RevokeOrganizationInvitation(ctx, org.ID, invite.ID); err != nil {
			t.Fatalf("failed to revoke invitation: %v", err)
		}

		_, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "friend-revoked")
		if !errors.Is(err, ErrInvitationRevoked) {
			t.Fatalf("expected ErrInvitationRevoked, got %v", err)
		}
	})

	t.Run("max uses limit enforcement", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleMember,
			MaxUses:        2,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatal(err)
		}

		if _, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "user-a"); err != nil {
			t.Fatalf("user-a accept failed: %v", err)
		}
		if _, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "user-b"); err != nil {
			t.Fatalf("user-b accept failed: %v", err)
		}
		// 3rd user should be rejected
		_, err := db.AcceptOrganizationInvitation(ctx, invite.Token, "user-c")
		if !errors.Is(err, ErrInvitationMaxUses) {
			t.Fatalf("expected ErrInvitationMaxUses, got %v", err)
		}
	})

	t.Run("concurrent accept respects max uses", func(t *testing.T) {
		invite := domain.OrganizationInvitation{
			OrganizationID: org.ID,
			InviterUserID:  owner.ID,
			Role:           domain.RoleMember,
			MaxUses:        3,
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		}
		if err := db.CreateOrganizationInvitation(ctx, &invite); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				userID := "concurrent-user-" + string(rune('a'+idx))
				_, err := db.AcceptOrganizationInvitation(ctx, invite.Token, userID)
				if err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()

		if successCount > 3 {
			t.Fatalf("expected at most 3 successful accepts, got %d", successCount)
		}
	})
}
