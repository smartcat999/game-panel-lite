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

func TestUserManagementAndRegistrationPolicy(t *testing.T) {
	router, _, _ := newTestRouter(t)

	// 1. Setup Admin
	setup := httptest.NewRecorder()
	setupReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"superadmin","password":"password123"}`))
	setupReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(setup, setupReq)
	if setup.Code != stdhttp.StatusCreated {
		t.Fatalf("expected setup 201, got %d: %s", setup.Code, setup.Body.String())
	}
	adminCookie := authCookieFromRecorder(t, setup)

	// 2. Try register when registration is disabled by default -> should be 403
	regDisabled := httptest.NewRecorder()
	regDisabledReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"player1","password":"password123"}`))
	regDisabledReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(regDisabled, regDisabledReq)
	if regDisabled.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected register 403 when disabled, got %d: %s", regDisabled.Code, regDisabled.Body.String())
	}

	// 3. Admin creates a member user
	createUser := httptest.NewRecorder()
	createUserReq := httptest.NewRequest(stdhttp.MethodPost, "/api/users", strings.NewReader(`{"username":"gamer1","password":"password123","role":"member"}`))
	createUserReq.Header.Set("Content-Type", "application/json")
	createUserReq.AddCookie(adminCookie)
	router.ServeHTTP(createUser, createUserReq)
	if createUser.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create user 201, got %d: %s", createUser.Code, createUser.Body.String())
	}

	// 4. List users
	listUsers := httptest.NewRecorder()
	listUsersReq := httptest.NewRequest(stdhttp.MethodGet, "/api/users", nil)
	listUsersReq.AddCookie(adminCookie)
	router.ServeHTTP(listUsers, listUsersReq)
	if listUsers.Code != stdhttp.StatusOK {
		t.Fatalf("expected list users 200, got %d: %s", listUsers.Code, listUsers.Body.String())
	}
	if !strings.Contains(listUsers.Body.String(), `"username":"gamer1"`) {
		t.Fatalf("expected gamer1 in list, got %s", listUsers.Body.String())
	}

	// 5. Admin enables public registration
	enableReg := httptest.NewRecorder()
	enableRegReq := httptest.NewRequest(stdhttp.MethodPut, "/api/settings/registration", strings.NewReader(`{"allowRegistration":true}`))
	enableRegReq.Header.Set("Content-Type", "application/json")
	enableRegReq.AddCookie(adminCookie)
	router.ServeHTTP(enableReg, enableRegReq)
	if enableReg.Code != stdhttp.StatusOK {
		t.Fatalf("expected enable reg 200, got %d: %s", enableReg.Code, enableReg.Body.String())
	}

	// 6. Now register player1 -> should succeed
	regEnabled := httptest.NewRecorder()
	regEnabledReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"player1","password":"password123"}`))
	regEnabledReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(regEnabled, regEnabledReq)
	if regEnabled.Code != stdhttp.StatusCreated {
		t.Fatalf("expected register 201 when enabled, got %d: %s", regEnabled.Code, regEnabled.Body.String())
	}
	memberCookie := authCookieFromRecorder(t, regEnabled)

	// 7. Member tries to access admin-only /api/users -> should be 403 Forbidden
	memberAccess := httptest.NewRecorder()
	memberAccessReq := httptest.NewRequest(stdhttp.MethodGet, "/api/users", nil)
	memberAccessReq.AddCookie(memberCookie)
	router.ServeHTTP(memberAccess, memberAccessReq)
	if memberAccess.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected member access 403, got %d: %s", memberAccess.Code, memberAccess.Body.String())
	}
}

func TestRolePermissionsProtectMutations(t *testing.T) {
	router, _, _ := newTestRouter(t)

	setup := httptest.NewRecorder()
	setupReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"superadmin","password":"password123"}`))
	setupReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(setup, setupReq)
	adminCookie := authCookieFromRecorder(t, setup)

	createAccount := func(username, role string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(stdhttp.MethodPost, "/api/users", strings.NewReader(`{"username":"`+username+`","password":"password123","role":"`+role+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(adminCookie)
		router.ServeHTTP(recorder, request)
		if recorder.Code != stdhttp.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d: %s", role, recorder.Code, recorder.Body.String())
		}
	}
	login := func(username string) *stdhttp.Cookie {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"password123"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != stdhttp.StatusOK {
			t.Fatalf("login %s: expected 200, got %d: %s", username, recorder.Code, recorder.Body.String())
		}
		return authCookieFromRecorder(t, recorder)
	}
	request := func(method, path, body string, cookie *stdhttp.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.AddCookie(cookie)
		router.ServeHTTP(recorder, req)
		return recorder
	}

	createAccount("operator1", "member")
	createAccount("observer1", "viewer")
	memberCookie := login("operator1")
	viewerCookie := login("observer1")

	viewerBootstrap := request(stdhttp.MethodGet, "/api/auth/bootstrap", "", viewerCookie)
	if viewerBootstrap.Code != stdhttp.StatusOK || !strings.Contains(viewerBootstrap.Body.String(), `"permissions":["server.view"]`) {
		t.Fatalf("expected viewer bootstrap to expose read-only permissions, got %d: %s", viewerBootstrap.Code, viewerBootstrap.Body.String())
	}

	viewerRead := request(stdhttp.MethodGet, "/api/servers", "", viewerCookie)
	if viewerRead.Code != stdhttp.StatusOK {
		t.Fatalf("expected viewer read access 200, got %d: %s", viewerRead.Code, viewerRead.Body.String())
	}
	viewerWrite := request(stdhttp.MethodPost, "/api/servers", `{}`, viewerCookie)
	if viewerWrite.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected viewer write access 403, got %d: %s", viewerWrite.Code, viewerWrite.Body.String())
	}
	viewerActivity := request(stdhttp.MethodGet, "/api/activity", "", viewerCookie)
	if viewerActivity.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected viewer activity access 403, got %d: %s", viewerActivity.Code, viewerActivity.Body.String())
	}
	viewerMonitoringEvents := request(stdhttp.MethodGet, "/api/monitoring/events", "", viewerCookie)
	if viewerMonitoringEvents.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected viewer monitoring events access 403, got %d: %s", viewerMonitoringEvents.Code, viewerMonitoringEvents.Body.String())
	}
	viewerWorlds := request(stdhttp.MethodGet, "/api/worlds", "", viewerCookie)
	if viewerWorlds.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected viewer world access 403, got %d: %s", viewerWorlds.Code, viewerWorlds.Body.String())
	}

	memberDelete := request(stdhttp.MethodDelete, "/api/servers/missing", "", memberCookie)
	if memberDelete.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected member server delete 403, got %d: %s", memberDelete.Code, memberDelete.Body.String())
	}
	memberSettings := request(stdhttp.MethodPut, "/api/settings/public-host", `{"publicHost":"games.example.com"}`, memberCookie)
	if memberSettings.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected member settings mutation 403, got %d: %s", memberSettings.Code, memberSettings.Body.String())
	}
	memberTenant := request(stdhttp.MethodGet, "/api/organizations", "", memberCookie)
	if memberTenant.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected member tenant administration 403, got %d: %s", memberTenant.Code, memberTenant.Body.String())
	}

	memberOperation := request(stdhttp.MethodPost, "/api/servers/missing/start", "", memberCookie)
	if memberOperation.Code == stdhttp.StatusForbidden {
		t.Fatalf("expected member lifecycle permission, got 403: %s", memberOperation.Body.String())
	}
}
