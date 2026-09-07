package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdministratorResetRevokesOnlyTargetSessions(t *testing.T) {
	router, _, _ := newTestRouter(t)
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	setup := request(http.MethodPost, "/api/auth/setup", `{"username":"resetadmin","password":"secret123"}`, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup: %d", setup.Code)
	}
	adminCookie := authCookieFromRecorder(t, setup)
	created := request(http.MethodPost, "/api/users", `{"username":"resetmember","password":"secret123","role":"member"}`, adminCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var account authAccountResponse
	if err := json.Unmarshal(created.Body.Bytes(), &account); err != nil {
		t.Fatal(err)
	}
	login := request(http.MethodPost, "/api/auth/login", `{"username":"resetmember","password":"secret123"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d", login.Code)
	}
	oldCookie := authCookieFromRecorder(t, login)
	reset := request(http.MethodPut, "/api/users/"+account.ID+"/password", `{"newPassword":"changed123"}`, adminCookie)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", reset.Code, reset.Body.String())
	}
	if got := request(http.MethodGet, "/api/auth/me", "", oldCookie); got.Code != http.StatusUnauthorized {
		t.Fatalf("target session survived: %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/auth/me", "", adminCookie); got.Code != http.StatusOK {
		t.Fatalf("admin session revoked: %d", got.Code)
	}
	if got := request(http.MethodPost, "/api/auth/login", `{"username":"resetmember","password":"secret123"}`, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("old password accepted: %d", got.Code)
	}
	if got := request(http.MethodPost, "/api/auth/login", `{"username":"resetmember","password":"changed123"}`, nil); got.Code != http.StatusOK {
		t.Fatalf("new password rejected: %d", got.Code)
	}
}
