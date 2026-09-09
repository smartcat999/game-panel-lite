package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestRegisteredUsersOwnSeparateWorkspaces(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	request := func(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		req.Header.Set("X-User-ID", "alice")
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}
	if got := request("/api/auth/me/organizations", nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("uninitialized access: %d", got.Code)
	}
	if err := db.SetSetting(ctx, domain.SettingKeyAllowRegistration, "true"); err != nil {
		t.Fatal(err)
	}
	var cookies []*http.Cookie
	var spaces []domain.Organization
	var accounts []domain.AdminAccount
	for _, name := range []string{"alice", "bruce"} {
		result := httptest.NewRecorder()
		router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+name+`","password":"secret123"}`)))
		if result.Code != http.StatusCreated {
			t.Fatalf("register: %d %s", result.Code, result.Body.String())
		}
		cookie := authCookieFromRecorder(t, result)
		cookies = append(cookies, cookie)
		account, err := db.GetAdminAccountByUsername(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, account)
		listed := request("/api/auth/me/organizations?userId=alice", cookie)
		var orgs []domain.OrganizationMembershipSummary
		if err := json.Unmarshal(listed.Body.Bytes(), &orgs); err != nil || listed.Code != http.StatusOK || len(orgs) != 1 {
			t.Fatalf("list: %d %s %v", listed.Code, listed.Body.String(), err)
		}
		if orgs[0].MembershipRole != domain.RoleOwner {
			t.Fatalf("membership role: %s", orgs[0].MembershipRole)
		}
		spaces = append(spaces, orgs[0].Organization)
	}
	if spaces[0].ID == spaces[1].ID {
		t.Fatal("shared personal workspace")
	}
	for i, cookie := range cookies {
		if got := request("/api/auth/me/organizations/"+spaces[i].ID, cookie); got.Code != http.StatusOK {
			t.Fatalf("own workspace: %d", got.Code)
		}
		if got := request("/api/auth/me/organizations/"+spaces[1-i].ID, cookie); got.Code != http.StatusNotFound {
			t.Fatalf("other workspace: %d", got.Code)
		}
		if got := request("/api/organizations", cookie); got.Code != http.StatusForbidden {
			t.Fatalf("platform access: %d", got.Code)
		}
	}
	// Even platform role elevation does not bypass membership on the user endpoint.
	accounts[1].Role = domain.RoleAdmin
	accounts[1].PlatformRole = domain.PlatformRoleAdmin
	if err := db.SaveAdminAccount(ctx, &accounts[1]); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/auth/me/organizations/"+spaces[0].ID, cookies[1]); got.Code != http.StatusNotFound {
		t.Fatalf("admin membership bypass: %d", got.Code)
	}
	if err := db.RemoveOrganizationMember(ctx, spaces[0].ID, accounts[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/auth/me/organizations/"+spaces[0].ID, cookies[0]); got.Code != http.StatusNotFound {
		t.Fatalf("revoked membership: %d", got.Code)
	}
	if got := request("/api/auth/me/organizations", cookies[0]); got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != "[]" {
		t.Fatalf("revoked list: %d %s", got.Code, got.Body.String())
	}
}
