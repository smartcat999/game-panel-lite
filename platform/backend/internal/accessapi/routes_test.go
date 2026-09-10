package accessapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type staticResolver struct {
	scopes map[string]authorization.Scope
}

func (r staticResolver) Resolve(_ context.Context, _ httpfilter.ResourceType, ids []string) (map[string]authorization.Scope, error) {
	result := make(map[string]authorization.Scope, len(ids))
	for _, id := range ids {
		if scope, ok := r.scopes[id]; ok {
			result[id] = scope
		}
	}
	return result, nil
}

func setupRoutes(t *testing.T) (http.Handler, *authentication.SessionService, *authorization.MemoryStore) {
	t.Helper()
	store := authentication.NewMemoryStore()
	sessions := authentication.NewSessionService(store, authentication.SessionPolicy{AbsoluteLifetime: 24 * time.Hour, IdleTimeout: time.Hour, ReauthWindow: 10 * time.Minute, SecureCookies: true})
	passwords := authentication.NewPasswordService(store, sessions)
	cipher, err := authentication.NewSecretCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	totp := authentication.NewTOTPService(store, cipher)
	bindings, err := authorization.NewMemoryStore([]authorization.RoleBinding{
		{ID: "rb_platform", PrincipalID: "usr_platform", Role: authorization.RolePlatformAdmin, Scope: authorization.Scope{Type: authorization.ScopePlatform, ID: "platform"}},
		{ID: "rb_owner", PrincipalID: "usr_owner", Role: authorization.RoleWorkspaceOwner, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_ember"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	services := Services{
		Passwords:   passwords,
		Sessions:    sessions,
		TOTP:        totp,
		Invitations: authentication.NewInvitationService(store, bindings),
		Authorizer:  authorization.NewEngine(bindings),
		Resolver: staticResolver{scopes: map[string]authorization.Scope{
			"platform": {Type: authorization.ScopePlatform, ID: "platform"},
			"ws_ember": {Type: authorization.ScopeWorkspace, ID: "ws_ember"},
		}},
	}
	return Routes(services), sessions, bindings
}

func requestWithSession(handler http.Handler, method, target, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestPlatformUserCreationRequiresRoleAndFreshTOTP(t *testing.T) {
	handler, sessions, _ := setupRoutes(t)
	token, _, err := sessions.Issue(context.Background(), "usr_platform", true)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"username":"local-operator","displayName":"Local Operator","oneTimePassword":"temporary-pass-123"}`
	if response := requestWithSession(handler, http.MethodPost, "/v1/platform/users", body, token); response.Code != http.StatusForbidden {
		t.Fatalf("operator action without TOTP status=%d", response.Code)
	}

	enroll := requestWithSession(handler, http.MethodPost, "/v1/auth/operator/totp/enroll", `{}`, token)
	if enroll.Code != http.StatusCreated {
		t.Fatalf("enroll status=%d body=%s", enroll.Code, enroll.Body.String())
	}
	var enrollment struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(enroll.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	code := testTOTPCode(enrollment.Secret, time.Now().UTC())
	verify := requestWithSession(handler, http.MethodPost, "/v1/auth/operator/totp/verify", `{"code":"`+code+`"}`, token)
	if verify.Code != http.StatusNoContent {
		t.Fatalf("verify status=%d body=%s", verify.Code, verify.Body.String())
	}
	cookies := verify.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == token {
		t.Fatal("operator verification did not rotate session")
	}
	created := requestWithSession(handler, http.MethodPost, "/v1/platform/users", body, cookies[0].Value)
	if created.Code != http.StatusCreated {
		t.Fatalf("verified operator create status=%d body=%s", created.Code, created.Body.String())
	}
	signIn := httptest.NewRequest(http.MethodPost, "/v1/auth/password/sign-in", strings.NewReader(`{"username":"local-operator","password":"temporary-pass-123"}`))
	signInResponse := httptest.NewRecorder()
	handler.ServeHTTP(signInResponse, signIn)
	if signInResponse.Code != http.StatusOK || !strings.Contains(signInResponse.Body.String(), `"mustChangePassword":true`) {
		t.Fatalf("temporary sign-in status=%d body=%s", signInResponse.Code, signInResponse.Body.String())
	}
	localCookie := signInResponse.Result().Cookies()[0]
	changed := requestWithSession(handler, http.MethodPut, "/v1/auth/password", `{"password":"permanent-pass-456"}`, localCookie.Value)
	if changed.Code != http.StatusNoContent {
		t.Fatalf("temporary password change status=%d body=%s", changed.Code, changed.Body.String())
	}
}

func TestWorkspaceInvitationRouteUsesWorkspacePolicy(t *testing.T) {
	handler, sessions, bindings := setupRoutes(t)
	ownerToken, _, _ := sessions.Issue(context.Background(), "usr_owner", true)
	create := requestWithSession(handler, http.MethodPost, "/v1/workspaces/ws_ember/invitations", `{"role":"workspace.viewer"}`, ownerToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create invitation status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	invitedToken, _, _ := sessions.Issue(context.Background(), "usr_invited", true)
	redeem := requestWithSession(handler, http.MethodPost, "/v1/invitations/"+created.Token+":redeem", `{}`, invitedToken)
	if redeem.Code != http.StatusOK {
		t.Fatalf("redeem status=%d body=%s", redeem.Code, redeem.Body.String())
	}
	decisions, err := authorization.NewEngine(bindings).CheckBatch(context.Background(), "usr_invited", []authorization.Check{{Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_ember"}, Action: authorization.ActionInstanceRead}})
	if err != nil || len(decisions) != 1 || !decisions[0].Allowed {
		t.Fatalf("redeemed binding unavailable: %#v %v", decisions, err)
	}
}

func TestAccessRoutesOverlayWithoutCapturingProductRoutes(t *testing.T) {
	access := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.Header().Set("X-Handler", "access") })
	product := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.Header().Set("X-Handler", "product") })
	handler := WithFallback(access, product)
	for _, test := range []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/v1/session", "access"},
		{http.MethodPost, "/v1/workspaces/ws_one/invitations", "access"},
		{http.MethodGet, "/v1/workspaces/ws_one/instances", "product"},
		{http.MethodGet, "/v1/platform/users", "product"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if got := response.Header().Get("X-Handler"); got != test.want {
			t.Fatalf("%s %s used %q, want %q", test.method, test.path, got, test.want)
		}
	}
}

func TestPublicPasswordRegistrationDoesNotExist(t *testing.T) {
	handler, _, _ := setupRoutes(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"username":"new-user","password":"password-long-enough"}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("public registration route status=%d", response.Code)
	}
}

func TestGitHubRouteFailsClosedWhenOAuthIsNotConfigured(t *testing.T) {
	handler, _, _ := setupRoutes(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/auth/github/start", strings.NewReader(`{"returnPath":"/"}`)))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "github_sign_in_unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func testTOTPCode(secret string, now time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(now.Unix()/30))
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
