package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestCredentialRotation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testCredentialRotation(t, db)
}
func testCredentialRotation(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	account := domain.AdminAccount{ID: "credential-account", Username: "credential-account", Role: domain.RoleMember, PasswordHash: "old-hash"}
	if err := db.CreateAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"credential-session-a", "credential-session-b"} {
		session := domain.Session{ID: id, AccountID: account.ID, TokenHash: id, ExpiresAt: time.Now().Add(time.Hour)}
		if err := db.CreateSessionForPassword(ctx, &session, account.PasswordHash); err != nil {
			t.Fatal(err)
		}
	}
	replacement := domain.Session{ID: "credential-replacement", AccountID: account.ID, TokenHash: "credential-replacement", ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.RotatePassword(ctx, account.ID, "old-hash", "new-hash", &replacement); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"credential-session-a", "credential-session-b"} {
		if _, err := db.GetSessionByTokenHash(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("old session: %v", err)
		}
	}
	stale := domain.Session{ID: "credential-stale", AccountID: account.ID, TokenHash: "credential-stale"}
	if err := db.CreateSessionForPassword(ctx, &stale, "old-hash"); !errors.Is(err, ErrCredentialsChanged) {
		t.Fatalf("stale login: %v", err)
	}
	if err := db.RotatePassword(ctx, account.ID, "old-hash", "lost-update", nil); !errors.Is(err, ErrCredentialsChanged) {
		t.Fatalf("stale rotation: %v", err)
	}
	if err := db.UpdateAccountRole(ctx, account.ID, domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetAdminAccount(ctx, account.ID)
	if err != nil || got.PasswordHash != "new-hash" || got.Role != domain.RoleViewer {
		t.Fatalf("role overwrote credentials: %+v %v", got, err)
	}

	// A replacement insert failure must roll back the password and revocations.
	collision := domain.Session{ID: "credential-collision", AccountID: "other-account", TokenHash: "collision-token"}
	if err := db.CreateSession(ctx, &collision); err != nil {
		t.Fatal(err)
	}
	failedReplacement := domain.Session{ID: collision.ID, AccountID: account.ID, TokenHash: "replacement-collision"}
	if err := db.RotatePassword(ctx, account.ID, "new-hash", "rolled-back-hash", &failedReplacement); err == nil {
		t.Fatal("duplicate session succeeded")
	}
	got, err = db.GetAdminAccount(ctx, account.ID)
	if err != nil || got.PasswordHash != "new-hash" {
		t.Fatalf("password not rolled back: %+v %v", got, err)
	}
	if _, err := db.GetSessionByTokenHash(ctx, replacement.TokenHash); err != nil {
		t.Fatalf("session revocation not rolled back: %v", err)
	}
	if err := db.RotatePassword(ctx, account.ID, "new-hash", "reset-hash", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSessionByTokenHash(ctx, replacement.TokenHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reset retained session: %v", err)
	}
	if _, err := db.GetSessionByTokenHash(ctx, collision.TokenHash); err != nil {
		t.Fatalf("reset affected other account: %v", err)
	}
}

func testConcurrentCredentialRotation(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	account := domain.AdminAccount{ID: "concurrent-login", Username: "concurrent-login", PasswordHash: "before"}
	if err := db.CreateAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	failures := make(chan error, 9)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			session := domain.Session{ID: fmt.Sprintf("concurrent-session-%d", i), AccountID: account.ID, TokenHash: fmt.Sprintf("concurrent-token-%d", i)}
			err := db.CreateSessionForPassword(ctx, &session, "before")
			if errors.Is(err, ErrCredentialsChanged) {
				err = nil
			}
			failures <- err
		}(i)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		failures <- db.RotatePassword(ctx, account.ID, "before", "after", nil)
	}()
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.db.Model(&domain.Session{}).Where("account_id = ?", account.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale login survived rotation: %d %v", count, err)
	}
}
