package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestWorkspaceLibraryUploadAuthorizationAndCleanup(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	for _, user := range []string{"upload-alice", "upload-bob"} {
		account := domain.AdminAccount{ID: user, Username: user, Role: domain.RoleMember}
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
		org := domain.Organization{ID: user, Slug: user}
		if err := db.CreateOrganization(ctx, &org, user); err != nil {
			t.Fatal(err)
		}
		session := domain.Session{ID: user, AccountID: user, TokenHash: hashSessionToken(user), ExpiresAt: time.Now().Add(time.Hour)}
		if err := db.CreateSession(ctx, &session); err != nil {
			t.Fatal(err)
		}
	}
	upload := func(user, org, name string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		query := url.Values{"organizationId": {org}, "providerKey": {string(domain.ProviderTerrariaTModLoader)}, "fileName": {name}}
		request := httptest.NewRequest(http.MethodPost, "/api/auth/me/mods/upload?"+query.Encode(), body)
		request.Header.Set("Content-Type", "application/octet-stream")
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: user})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	legacy := domain.ModFile{ID: "private-legacy-mod", InstanceID: "unassigned", Source: "workshop", WorkshopID: "123", ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "legacy.tmod"}
	if err := db.CreateMod(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	legacyPack := domain.ModPack{ID: "private-legacy-pack", Name: "Legacy", ModIDsJSON: `["private-legacy-mod"]`}
	if err := db.CreateModPack(ctx, &legacyPack); err != nil {
		t.Fatal(err)
	}
	fixture := tmodFixture("Custom", "1", "2024")
	for _, user := range []string{"upload-alice", "upload-bob"} {
		response := upload(user, user, "same.tmod", bytes.NewReader(fixture))
		if response.Code != http.StatusCreated {
			t.Fatalf("upload: %d %s", response.Code, response.Body.String())
		}
		var item domain.ModFile
		if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		if item.OrganizationID != user || item.SizeBytes != int64(len(fixture)) || item.ContentHash == "" || item.ModName != "Custom" {
			t.Fatalf("upload record: %+v", item)
		}
		file, err := newTestModService(t, cfg.DataDir).OpenLibrary(item)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil || !bytes.Equal(content, fixture) {
			t.Fatalf("published bytes: %v", err)
		}
	}
	for _, user := range []string{"upload-alice", "upload-bob"} {
		request := httptest.NewRequest(http.MethodGet, "/api/auth/me/mods", nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: user})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var items []domain.ModFile
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &items) != nil || len(items) != 1 || items[0].OrganizationID != user {
			t.Fatalf("scoped list: %d %s", response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/api/mods", "/api/mod-packs"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "upload-alice"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-legacy") {
			t.Fatalf("legacy list leaked: %d %s", response.Code, response.Body.String())
		}
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/mods/upload"}, {http.MethodPost, "/api/mods/workshop"}, {http.MethodPost, "/api/mods/workshop/preview"}, {http.MethodPost, "/api/mods/workshop/items/preview"}, {http.MethodPost, "/api/mods/recommended/import"}, {http.MethodPost, "/api/mods/batch-delete"}, {http.MethodDelete, "/api/mods/private-legacy-mod"},
		{http.MethodPost, "/api/mod-packs"}, {http.MethodPost, "/api/mod-packs/workshop"}, {http.MethodPatch, "/api/mod-packs/private-legacy-pack"}, {http.MethodPost, "/api/mod-packs/batch-delete"}, {http.MethodDelete, "/api/mod-packs/private-legacy-pack"},
	} {
		request := httptest.NewRequest(route.method, route.path, unreadableUpload{t})
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "upload-alice"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("legacy mutation allowed %s: %d %s", route.path, response.Code, response.Body.String())
		}
	}
	if got := upload("upload-alice", "upload-alice", "same.tmod", bytes.NewReader(fixture)); got.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", got.Code, got.Body.String())
	}
	if got := upload("upload-alice", "upload-bob", "other.tmod", unreadableUpload{t}); got.Code != http.StatusForbidden {
		t.Fatalf("foreign upload: %d %s", got.Code, got.Body.String())
	}
	if got := upload("upload-alice", "upload-alice", "../bad.tmod", unreadableUpload{t}); got.Code != http.StatusBadRequest {
		t.Fatalf("traversal: %d %s", got.Code, got.Body.String())
	}
	if got := upload("upload-alice", "upload-alice", "invalid.tmod", strings.NewReader("invalid")); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid metadata: %d %s", got.Code, got.Body.String())
	}
	revoked := &revokingUpload{Reader: bytes.NewReader(fixture), revoke: func() {
		if err := db.RemoveOrganizationMember(ctx, "upload-alice", "upload-alice"); err != nil {
			t.Fatal(err)
		}
	}}
	if got := upload("upload-alice", "upload-alice", "revoked.tmod", revoked); got.Code != http.StatusForbidden {
		t.Fatalf("revoked during upload: %d %s", got.Code, got.Body.String())
	}
	published := 0
	if err := filepath.WalkDir(filepath.Join(cfg.DataDir, "mod-library"), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == "content" {
			published++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if published != 2 {
		t.Fatalf("failed uploads left %d published files; expected two successful uploads", published)
	}
}

type unreadableUpload struct{ t *testing.T }

func (r unreadableUpload) Read([]byte) (int, error) {
	r.t.Fatal("rejected upload consumed body")
	return 0, io.EOF
}

type revokingUpload struct {
	io.Reader
	revoke func()
}

func (r *revokingUpload) Read(p []byte) (int, error) {
	if r.revoke != nil {
		f := r.revoke
		r.revoke = nil
		f()
	}
	return r.Reader.Read(p)
}
