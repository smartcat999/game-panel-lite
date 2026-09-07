package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestBackupEndpointsEnforceMembershipBeforeFileAccess(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	paths := map[string]string{}
	for _, id := range []string{"backup-a", "backup-b"} {
		account := domain.AdminAccount{ID: id, Username: id, Role: domain.RoleMember}
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: id, OrganizationID: id, Spec: domain.ServerSpec{DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
		item := domain.Backup{ID: id, InstanceID: id, FileName: "snapshot.zip"}
		if err := db.CreateBackup(ctx, &item); err != nil {
			t.Fatal(err)
		}
		path, err := backupsvc.NewService(cfg.DataDir).Path(id, item.FileName)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("private-"+id), 0600); err != nil {
			t.Fatal(err)
		}
		paths[id] = path
	}
	session := domain.Session{ID: "backup-session", AccountID: "backup-a", TokenHash: hashSessionToken("backup-token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "backup-token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/backups/backup-b/download"},
		{http.MethodPost, "/api/backups/backup-b/restore"},
		{http.MethodDelete, "/api/backups/backup-b"},
		{http.MethodGet, "/api/servers/backup-a/saves/backup-b/download"},
		{http.MethodPost, "/api/servers/backup-a/saves/backup-b/restore"},
	} {
		got := request(tc.method, tc.path)
		if got.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant %s %s: %d %s", tc.method, tc.path, got.Code, got.Body.String())
		}
	}
	content, err := os.ReadFile(paths["backup-b"])
	if err != nil || string(content) != "private-backup-b" {
		t.Fatalf("foreign file changed: %q %v", content, err)
	}
	if _, err := db.GetBackup(ctx, "backup-b"); err != nil {
		t.Fatalf("foreign metadata changed: %v", err)
	}
	got := request(http.MethodGet, "/api/backups?organizationId=backup-b")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"id":"backup-a"`) || strings.Contains(got.Body.String(), `"id":"backup-b"`) {
		t.Fatalf("scoped list: %d %s", got.Code, got.Body.String())
	}
	got = request(http.MethodGet, "/api/backups/backup-a/download")
	if got.Code != http.StatusOK || got.Body.String() != "private-backup-a" {
		t.Fatalf("own download: %d %s", got.Code, got.Body.String())
	}
	deletable := domain.Backup{ID: "owned-delete", InstanceID: "backup-a", FileName: "deletable.zip"}
	if err := db.CreateBackup(ctx, &deletable); err != nil {
		t.Fatal(err)
	}
	deletePath := filepath.Join(filepath.Dir(paths["backup-a"]), deletable.FileName)
	if err := os.WriteFile(deletePath, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodDelete, "/api/backups/owned-delete"); got.Code != http.StatusOK {
		t.Fatalf("owner delete: %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("owner file not removed: %v", err)
	}
	if _, err := db.GetBackup(ctx, deletable.ID); err == nil {
		t.Fatal("owner metadata not removed")
	}
	if err := db.RemoveOrganizationMember(ctx, "backup-a", "backup-a"); err != nil {
		t.Fatal(err)
	}
	member := domain.OrganizationMember{ID: "backup-viewer", UserID: "backup-a", OrganizationID: "backup-a", Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &member); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{{http.MethodPost, "/api/backups/backup-a/restore"}, {http.MethodDelete, "/api/backups/backup-a"}} {
		if got := request(tc.method, tc.path); got.Code != http.StatusForbidden {
			t.Fatalf("viewer mutation: %d %s", got.Code, got.Body.String())
		}
	}
	if err := os.Remove(paths["backup-a"]); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/backups", "/api/backups/backup-a/download", "/api/servers/backup-a/saves", "/api/servers/backup-a/saves/backup-a/download"} {
		request(http.MethodGet, path)
		if _, err := db.GetBackup(ctx, "backup-a"); err != nil {
			t.Fatalf("read %s deleted metadata: %v", path, err)
		}
	}
	if err := db.RemoveOrganizationMember(ctx, "backup-a", "backup-a"); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "/api/backups/backup-a/download"); got.Code != http.StatusNotFound {
		t.Fatalf("revoked download: %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/backups"); got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != "[]" {
		t.Fatalf("revoked list: %d %s", got.Code, got.Body.String())
	}
}
