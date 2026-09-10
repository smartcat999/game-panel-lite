package authentication

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

func TestPostgresIdentityAuthorizationRoundTrip(t *testing.T) {
	database := openAuthenticationDatabase(t)
	ctx := context.Background()
	store := NewPostgresStore(database)
	sessions := NewSessionService(store, SessionPolicy{AbsoluteLifetime: 24 * time.Hour, IdleTimeout: time.Hour, ReauthWindow: 10 * time.Minute, SecureCookies: true})
	passwords := NewPasswordService(store, sessions)
	user, err := passwords.CreateLocalAccount(ctx, "local-operator", "Local Operator", "temporary-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	var storedHash string
	if err := database.QueryRow(`SELECT password_hash FROM local_credentials WHERE user_id = $1`, user.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == "temporary-pass-123" || !strings.HasPrefix(storedHash, "$2") {
		t.Fatal("plaintext password persisted")
	}
	token, session, mustChange, err := passwords.Authenticate(ctx, "LOCAL-OPERATOR", "temporary-pass-123")
	if err != nil || token == "" || session.UserID != user.ID || !mustChange {
		t.Fatalf("local login failed: %#v mustChange=%v err=%v", session, mustChange, err)
	}
	var rawMatches int
	if err := database.QueryRow(`SELECT count(*) FROM auth_sessions WHERE token_hash = $1`, token).Scan(&rawMatches); err != nil || rawMatches != 0 {
		t.Fatalf("raw session token persisted: count=%d err=%v", rawMatches, err)
	}

	bindingStore := authorization.NewPostgresStore(database)
	binding := authorization.RoleBinding{ID: "rb_test", PrincipalID: authorization.PrincipalID(user.ID), Role: authorization.RoleWorkspaceOwner, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_test"}}
	if err := bindingStore.Put(ctx, binding); err != nil {
		t.Fatal(err)
	}
	decisions, err := authorization.NewEngine(bindingStore).CheckBatch(ctx, binding.PrincipalID, []authorization.Check{{ResourceType: "workspace", ResourceID: "ws_test", Scope: binding.Scope, Action: authorization.ActionMemberInvite}})
	if err != nil || len(decisions) != 1 || !decisions[0].Allowed {
		t.Fatalf("role binding round trip failed: %#v %v", decisions, err)
	}
}

func openAuthenticationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_GLOBAL_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_GLOBAL_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase2_auth_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`SET search_path TO "` + schema + `"`); err != nil {
		database.Close()
		admin.Close()
		t.Fatal(err)
	}
	_, filename, _, _ := runtime.Caller(0)
	for _, name := range []string{"0001_identity_workspace.sql", "0005_identity_authorization_rebaseline.sql"} {
		migration, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "global", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(migration), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := database.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := database.Exec(`INSERT INTO workspaces (id, slug, name, created_at) VALUES ('ws_test', 'test', 'Test', now())`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.Close()
		_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		admin.Close()
	})
	return database
}
