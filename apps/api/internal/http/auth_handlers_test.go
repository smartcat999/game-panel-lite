package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorsAllowsPatchPreflight(t *testing.T) {
	router, _, _ := newTestRouter(t)
	request := httptest.NewRequest(stdhttp.MethodOptions, "/api/servers/server-1/mods/mod-1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != stdhttp.StatusNoContent {
		t.Fatalf("expected CORS preflight 204, got %d", recorder.Code)
	}
	if methods := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "PATCH") {
		t.Fatalf("expected PATCH in allowed methods, got %q", methods)
	}
}

func TestAuthSetupLoginAndProtectedRoutes(t *testing.T) {
	router, _, _ := newTestRouter(t)

	bootstrap := httptest.NewRecorder()
	router.ServeHTTP(bootstrap, httptest.NewRequest(stdhttp.MethodGet, "/api/auth/bootstrap", nil))
	if bootstrap.Code != stdhttp.StatusOK {
		t.Fatalf("expected bootstrap 200, got %d: %s", bootstrap.Code, bootstrap.Body.String())
	}
	if !strings.Contains(bootstrap.Body.String(), `"initialized":false`) {
		t.Fatalf("expected uninitialized bootstrap, got %s", bootstrap.Body.String())
	}

	setup := httptest.NewRecorder()
	setupReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	setupReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(setup, setupReq)
	if setup.Code != stdhttp.StatusCreated {
		t.Fatalf("expected setup 201, got %d: %s", setup.Code, setup.Body.String())
	}
	setupCookie := authCookieFromRecorder(t, setup)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(stdhttp.MethodGet, "/api/version", nil))
	if unauthorized.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected protected route 401 after setup, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	me := httptest.NewRecorder()
	meReq := httptest.NewRequest(stdhttp.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(setupCookie)
	router.ServeHTTP(me, meReq)
	if me.Code != stdhttp.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", me.Code, me.Body.String())
	}
	if !strings.Contains(me.Body.String(), `"username":"admin"`) {
		t.Fatalf("expected account response, got %s", me.Body.String())
	}

	login := httptest.NewRecorder()
	loginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, loginReq)
	if login.Code != stdhttp.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", login.Code, login.Body.String())
	}
	loginCookie := authCookieFromRecorder(t, login)

	change := httptest.NewRecorder()
	changeReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/password", strings.NewReader(`{"currentPassword":"secret123","newPassword":"secret456"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(loginCookie)
	router.ServeHTTP(change, changeReq)
	if change.Code != stdhttp.StatusOK {
		t.Fatalf("expected password change 200, got %d: %s", change.Code, change.Body.String())
	}

	oldLogin := httptest.NewRecorder()
	oldLoginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(oldLogin, oldLoginReq)
	if oldLogin.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected old password login 401, got %d: %s", oldLogin.Code, oldLogin.Body.String())
	}

	newLogin := httptest.NewRecorder()
	newLoginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret456"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(newLogin, newLoginReq)
	if newLogin.Code != stdhttp.StatusOK {
		t.Fatalf("expected new password login 200, got %d: %s", newLogin.Code, newLogin.Body.String())
	}
}

func authCookieFromRecorder(t *testing.T, recorder *httptest.ResponseRecorder) *stdhttp.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatalf("expected %s cookie in response headers", sessionCookieName)
	return nil
}
