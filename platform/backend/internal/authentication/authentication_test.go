package authentication

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

var testNow = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)

func testSessions(store Store) *SessionService {
	service := NewSessionService(store, SessionPolicy{AbsoluteLifetime: 24 * time.Hour, IdleTimeout: time.Hour, ReauthWindow: 10 * time.Minute, SecureCookies: true})
	service.now = func() time.Time { return testNow }
	return service
}

type githubStub struct {
	profile OAuthProfile
}

func (s githubStub) AuthorizationURL(state string) string {
	return "https://github.com/login/oauth/authorize?state=" + url.QueryEscape(state)
}

func (s githubStub) Exchange(_ context.Context, code string) (OAuthProfile, error) {
	if code != "valid-code" {
		return OAuthProfile{}, ErrInvalidCredentials
	}
	return s.profile, nil
}

func TestGitHubOAuthStateIsHashedExpiringAndSingleUse(t *testing.T) {
	store := NewMemoryStore()
	sessions := testSessions(store)
	service := NewOAuthService(store, githubStub{profile: OAuthProfile{Provider: "github", Subject: "42", Username: "alexm", Email: "alex@example.test", DisplayName: "Alex"}}, sessions, 5*time.Minute)
	service.now = func() time.Time { return testNow }
	authorizationURL, err := service.Begin(context.Background(), "/w/ember/instances")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizationURL)
	state := parsed.Query().Get("state")
	if state == "" || store.states[state] != (OAuthState{}) {
		t.Fatal("raw OAuth state must not be persisted")
	}
	token, user, returnPath, err := service.Complete(context.Background(), state, "valid-code")
	if err != nil || token == "" || user.ID == "" || returnPath != "/w/ember/instances" {
		t.Fatalf("complete = token:%t user:%q return:%q err:%v", token != "", user.ID, returnPath, err)
	}
	if _, _, _, err := service.Complete(context.Background(), state, "valid-code"); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("state replay error = %v", err)
	}

	expiredURL, _ := service.Begin(context.Background(), "/")
	expiredState, _ := url.Parse(expiredURL)
	service.now = func() time.Time { return testNow.Add(6 * time.Minute) }
	if _, _, _, err := service.Complete(context.Background(), expiredState.Query().Get("state"), "valid-code"); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("expired state error = %v", err)
	}
	if _, err := service.Begin(context.Background(), "https://attacker.test"); err == nil {
		t.Fatal("external return URL accepted")
	}
}

func TestSessionSecurityLifecycleAndCookie(t *testing.T) {
	store := NewMemoryStore()
	service := testSessions(store)
	token, session, err := service.Issue(context.Background(), "usr_one", false)
	if err != nil {
		t.Fatal(err)
	}
	if token == session.TokenHash || store.sessions[token].ID != "" || store.sessions[tokenHash(token)].ID == "" {
		t.Fatal("session storage must contain only the token hash")
	}
	cookie := service.Cookie(token)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("unsafe cookie: %#v", cookie)
	}
	if _, err := service.Authenticate(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	rotated, recent, err := service.RotateAfterReauthentication(context.Background(), token)
	if err != nil || rotated == token || recent.ReauthenticatedAt == nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old token remains valid: %v", err)
	}
	if err := service.Revoke(context.Background(), rotated); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), rotated); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked token accepted: %v", err)
	}

	idleToken, _, _ := service.Issue(context.Background(), "usr_idle", false)
	service.now = func() time.Time { return testNow.Add(time.Hour + time.Second) }
	if _, err := service.Authenticate(context.Background(), idleToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("idle session accepted: %v", err)
	}

	absoluteStore := NewMemoryStore()
	absolute := NewSessionService(absoluteStore, SessionPolicy{AbsoluteLifetime: time.Hour, IdleTimeout: 24 * time.Hour, ReauthWindow: time.Minute, SecureCookies: true})
	absolute.now = func() time.Time { return testNow }
	absoluteToken, _, _ := absolute.Issue(context.Background(), "usr_absolute", false)
	absolute.now = func() time.Time { return testNow.Add(time.Hour) }
	if _, err := absolute.Authenticate(context.Background(), absoluteToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("absolute expiry accepted: %v", err)
	}
}

func TestLocalCredentialsAreHashedAndConstrained(t *testing.T) {
	store := NewMemoryStore()
	store.AddUser(User{ID: "usr_local", DisplayName: "Local", Email: ""})
	sessions := testSessions(store)
	passwords := NewPasswordService(store, sessions)
	passwords.now = func() time.Time { return testNow }
	if err := passwords.CreateOneTimeCredential(context.Background(), "usr_local", "local-admin", "temporary-pass-123"); err != nil {
		t.Fatal(err)
	}
	credential, _ := store.CredentialByUser(context.Background(), "usr_local")
	if credential.PasswordHash == "temporary-pass-123" || !strings.HasPrefix(credential.PasswordHash, "$2") || !credential.MustChange || credential.ExpiresAt == nil || !credential.ExpiresAt.Equal(testNow.Add(24*time.Hour)) {
		t.Fatalf("credential not safely stored: %#v", credential)
	}
	_, session, mustChange, err := passwords.Authenticate(context.Background(), "LOCAL-ADMIN", "temporary-pass-123")
	if err != nil || !mustChange || session.UserID != "usr_local" {
		t.Fatalf("temporary login failed: %v", err)
	}
	if err := passwords.ChangeTemporary(context.Background(), "usr_local", "temporary-pass-123", "permanent-pass-456"); err != nil {
		t.Fatal(err)
	}
	if _, _, mustChange, err = passwords.Authenticate(context.Background(), "local-admin", "permanent-pass-456"); err != nil || mustChange {
		t.Fatalf("changed credential failed: mustChange=%v err=%v", mustChange, err)
	}
}

func TestOneTimeCredentialExpiresAfterTwentyFourHours(t *testing.T) {
	store := NewMemoryStore()
	store.AddUser(User{ID: "usr_expiring", Username: "expiring"})
	sessions := testSessions(store)
	passwords := NewPasswordService(store, sessions)
	passwords.now = func() time.Time { return testNow }
	if err := passwords.CreateOneTimeCredential(context.Background(), "usr_expiring", "expiring", "temporary-pass-123"); err != nil {
		t.Fatal(err)
	}
	passwords.now = func() time.Time { return testNow.Add(24 * time.Hour) }
	if _, _, _, err := passwords.Authenticate(context.Background(), "expiring", "temporary-pass-123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired one-time credential accepted: %v", err)
	}
}

func TestOAuthUserPasswordRequiresRecentReauthentication(t *testing.T) {
	store := NewMemoryStore()
	sessions := testSessions(store)
	user, err := store.UpsertOAuthUser(context.Background(), OAuthProfile{Provider: "github", Subject: "77", Username: "owner", Email: "owner@example.test", DisplayName: "Owner"}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	passwords := NewPasswordService(store, sessions)
	passwords.now = func() time.Time { return testNow }
	stale := testNow.Add(-11 * time.Minute)
	if err := passwords.SetForOAuthUser(context.Background(), Session{UserID: user.ID, ReauthenticatedAt: &stale}, "secure-password-123"); !errors.Is(err, ErrReauthentication) {
		t.Fatalf("stale reauthentication accepted: %v", err)
	}
	recent := testNow.Add(-time.Minute)
	if err := passwords.SetForOAuthUser(context.Background(), Session{UserID: user.ID, ReauthenticatedAt: &recent}, "secure-password-123"); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorTOTPSecretIsEncryptedAndCodeCannotReplay(t *testing.T) {
	store := NewMemoryStore()
	cipher, err := NewSecretCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewTOTPService(store, cipher)
	service.now = func() time.Time { return testNow }
	secret, err := service.Enroll(context.Background(), "usr_operator")
	if err != nil {
		t.Fatal(err)
	}
	record, _ := store.TOTPByUser(context.Background(), "usr_operator")
	if string(record.EncryptedSecret) == secret || strings.Contains(string(record.EncryptedSecret), secret) {
		t.Fatal("raw TOTP secret persisted")
	}
	code := totpCode(secret, testNow.Unix()/30)
	if err := service.Verify(context.Background(), "usr_operator", code); err != nil {
		t.Fatal(err)
	}
	if err := service.Verify(context.Background(), "usr_operator", code); !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("TOTP replay accepted: %v", err)
	}
}

func TestWorkspaceInvitationIsScopedExpiringAndSingleUse(t *testing.T) {
	store := NewMemoryStore()
	bindings, _ := authorization.NewMemoryStore(nil)
	service := NewInvitationService(store, bindings)
	service.now = func() time.Time { return testNow }
	token, invitation, err := service.Create(context.Background(), "ws_ember", authorization.RoleWorkspaceOperator, "usr_owner", time.Hour)
	if err != nil || store.invitations[token].ID != "" {
		t.Fatalf("create invitation: %v", err)
	}
	binding, err := service.Accept(context.Background(), token, "usr_invited")
	if err != nil || binding.Scope.ID != "ws_ember" || binding.Role != authorization.RoleWorkspaceOperator {
		t.Fatalf("accept invitation: %#v %v", binding, err)
	}
	repeated, err := service.Accept(context.Background(), token, "usr_invited")
	if err != nil || repeated.ID != binding.ID {
		t.Fatalf("same-user invitation retry not idempotent: %#v %v", repeated, err)
	}
	if _, err := service.Accept(context.Background(), token, "usr_other"); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("invitation replay accepted: %v", err)
	}
	service.now = func() time.Time { return testNow.Add(2 * time.Hour) }
	expiredToken, _, _ := service.Create(context.Background(), "ws_ember", authorization.RoleWorkspaceViewer, "usr_owner", -time.Minute)
	if _, err := service.Accept(context.Background(), expiredToken, "usr_late"); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("expired invitation accepted: %v", err)
	}
	if invitation.TokenHash == token {
		t.Fatal("raw invitation token persisted")
	}
	if _, _, err := service.Create(context.Background(), "ws_ember", authorization.RolePlatformAdmin, "usr_owner", time.Hour); err == nil {
		t.Fatal("platform role accepted in workspace invitation")
	}
}

type retryBindingWriter struct {
	puts    int
	binding authorization.RoleBinding
}

func (w *retryBindingWriter) Put(_ context.Context, binding authorization.RoleBinding) error {
	w.puts++
	w.binding = binding
	if w.puts == 1 {
		return errors.New("temporary database failure")
	}
	return nil
}

func TestInvitationAcceptanceResumesAfterBindingWriteFailure(t *testing.T) {
	store := NewMemoryStore()
	writer := &retryBindingWriter{}
	service := NewInvitationService(store, writer)
	service.now = func() time.Time { return testNow }
	token, _, err := service.Create(context.Background(), "ws_ember", authorization.RoleWorkspaceViewer, "usr_owner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Accept(context.Background(), token, "usr_invited")
	if err == nil {
		t.Fatal("simulated binding failure was hidden")
	}
	firstBindingID := writer.binding.ID
	second, err := service.Accept(context.Background(), token, "usr_invited")
	if err != nil || firstBindingID != second.ID || writer.binding.ID != second.ID {
		t.Fatalf("acceptance did not resume deterministically: firstID=%q second=%#v stored=%#v err=%v", firstBindingID, second, writer.binding, err)
	}
}
