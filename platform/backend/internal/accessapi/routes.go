package accessapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type Services struct {
	OAuth       *authentication.OAuthService
	Passwords   *authentication.PasswordService
	Sessions    *authentication.SessionService
	TOTP        *authentication.TOTPService
	Invitations *authentication.InvitationService
	Authorizer  httpfilter.Authorizer
	Resolver    httpfilter.ScopeResolver
}

func Routes(services Services) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/auth/github/start", services.startGitHub)
	mux.HandleFunc("GET /v1/auth/github/callback", services.completeGitHub)
	mux.HandleFunc("POST /v1/auth/password/sign-in", services.passwordSignIn)
	mux.Handle("PUT /v1/auth/password", services.authenticated(httpfilter.RequireRecentReauthentication(services.Sessions, http.HandlerFunc(services.setPassword))))
	mux.Handle("GET /v1/session", services.authenticated(http.HandlerFunc(services.getSession)))
	mux.Handle("DELETE /v1/session", services.authenticated(http.HandlerFunc(services.deleteSession)))
	mux.Handle("POST /v1/invitations/", services.authenticated(http.HandlerFunc(services.redeemInvitation)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/invitations", services.workspaceAuthorized(authorization.ActionMemberInvite, http.HandlerFunc(services.createInvitation)))
	mux.Handle("POST /v1/auth/operator/totp/enroll", services.platformAuthorized(false, http.HandlerFunc(services.enrollTOTP)))
	mux.Handle("POST /v1/auth/operator/totp/verify", services.platformAuthorized(false, http.HandlerFunc(services.verifyTOTP)))
	mux.Handle("POST /v1/platform/users", services.platformAuthorized(true, http.HandlerFunc(services.createLocalUser)))
	return mux
}

func (s Services) authenticated(next http.Handler) http.Handler {
	return httpfilter.Authenticate(s.Sessions, next)
}

func (s Services) workspaceAuthorized(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceWorkspace, ResourceIDs: httpfilter.PathID("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) platformAuthorized(requireTOTP bool, next http.Handler) http.Handler {
	if requireTOTP {
		next = httpfilter.RequireOperatorVerification(s.Sessions, next)
	} else {
		next = httpfilter.RequireRecentReauthentication(s.Sessions, next)
	}
	policy := httpfilter.RoutePolicy{
		Action:       authorization.ActionPlatformManageUser,
		ResourceType: httpfilter.ResourcePlatform,
		ResourceIDs:  func(*http.Request) ([]string, error) { return []string{"platform"}, nil },
	}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) startGitHub(response http.ResponseWriter, request *http.Request) {
	if s.OAuth == nil {
		respondError(response, http.StatusServiceUnavailable, "github_sign_in_unavailable")
		return
	}
	var body struct {
		ReturnPath string `json:"returnPath"`
	}
	if !decode(response, request, &body) {
		return
	}
	authorizationURL, err := s.OAuth.Begin(request.Context(), body.ReturnPath)
	if err != nil {
		respondError(response, http.StatusBadRequest, "invalid_oauth_request")
		return
	}
	respondJSON(response, http.StatusOK, map[string]string{"authorizationUrl": authorizationURL})
}

func (s Services) completeGitHub(response http.ResponseWriter, request *http.Request) {
	if s.OAuth == nil {
		respondError(response, http.StatusServiceUnavailable, "github_sign_in_unavailable")
		return
	}
	token, _, returnPath, err := s.OAuth.Complete(request.Context(), request.URL.Query().Get("state"), request.URL.Query().Get("code"))
	if err != nil {
		respondError(response, http.StatusUnauthorized, "github_sign_in_failed")
		return
	}
	http.SetCookie(response, s.Sessions.Cookie(token))
	http.Redirect(response, request, returnPath, http.StatusFound)
}

func (s Services) passwordSignIn(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(response, request, &body) {
		return
	}
	token, session, mustChange, err := s.Passwords.Authenticate(request.Context(), body.Username, body.Password)
	if err != nil {
		respondError(response, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	http.SetCookie(response, s.Sessions.Cookie(token))
	respondJSON(response, http.StatusOK, map[string]any{"userId": session.UserID, "mustChangePassword": mustChange})
}

func (s Services) setPassword(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decode(response, request, &body) {
		return
	}
	session, _ := httpfilter.SessionFromContext(request.Context())
	if err := s.Passwords.SetForAuthenticatedUser(request.Context(), session, body.Password); err != nil {
		respondError(response, http.StatusBadRequest, "password_not_set")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (s Services) getSession(response http.ResponseWriter, request *http.Request) {
	session, _ := httpfilter.SessionFromContext(request.Context())
	payload := map[string]any{"userId": session.UserID, "expiresAt": session.ExpiresAt}
	if session.ReauthenticatedAt != nil {
		payload["reauthenticatedAt"] = session.ReauthenticatedAt
	}
	if session.OperatorVerifiedAt != nil {
		payload["operatorVerifiedAt"] = session.OperatorVerifiedAt
	}
	respondJSON(response, http.StatusOK, payload)
}

func (s Services) deleteSession(response http.ResponseWriter, request *http.Request) {
	token, _ := authentication.SessionToken(request)
	if err := s.Sessions.Revoke(request.Context(), token); err != nil {
		respondError(response, http.StatusUnauthorized, "invalid_session")
		return
	}
	cookie := s.Sessions.Cookie("")
	cookie.MaxAge = -1
	http.SetCookie(response, cookie)
	response.WriteHeader(http.StatusNoContent)
}

func (s Services) redeemInvitation(response http.ResponseWriter, request *http.Request) {
	prefix := "/v1/invitations/"
	token := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, prefix), ":redeem")
	if token == "" || !strings.HasSuffix(request.URL.Path, ":redeem") {
		respondError(response, http.StatusNotFound, "not_found")
		return
	}
	principal, _ := httpfilter.PrincipalFromContext(request.Context())
	binding, err := s.Invitations.Accept(request.Context(), token, principal.ID)
	if err != nil {
		respondError(response, http.StatusBadRequest, "invalid_invitation")
		return
	}
	respondJSON(response, http.StatusOK, binding)
}

func (s Services) createInvitation(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Role authorization.Role `json:"role"`
	}
	if !decode(response, request, &body) {
		return
	}
	principal, _ := httpfilter.PrincipalFromContext(request.Context())
	token, invitation, err := s.Invitations.Create(request.Context(), request.PathValue("workspaceId"), body.Role, principal.ID, 72*time.Hour)
	if err != nil {
		respondError(response, http.StatusBadRequest, "invitation_not_created")
		return
	}
	respondJSON(response, http.StatusCreated, map[string]any{"id": invitation.ID, "token": token, "expiresAt": invitation.ExpiresAt})
}

func (s Services) enrollTOTP(response http.ResponseWriter, request *http.Request) {
	principal, _ := httpfilter.PrincipalFromContext(request.Context())
	secret, err := s.TOTP.Enroll(request.Context(), principal.ID)
	if err != nil {
		respondError(response, http.StatusInternalServerError, "totp_not_enrolled")
		return
	}
	respondJSON(response, http.StatusCreated, map[string]string{"secret": secret})
}

func (s Services) verifyTOTP(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(response, request, &body) {
		return
	}
	principal, _ := httpfilter.PrincipalFromContext(request.Context())
	if err := s.TOTP.Verify(request.Context(), principal.ID, body.Code); err != nil {
		respondError(response, http.StatusUnauthorized, "invalid_totp")
		return
	}
	oldToken, _ := authentication.SessionToken(request)
	newToken, _, err := s.Sessions.RotateAfterOperatorVerification(request.Context(), oldToken)
	if err != nil {
		respondError(response, http.StatusUnauthorized, "session_rotation_failed")
		return
	}
	http.SetCookie(response, s.Sessions.Cookie(newToken))
	response.WriteHeader(http.StatusNoContent)
}

func (s Services) createLocalUser(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Password    string `json:"oneTimePassword"`
	}
	if !decode(response, request, &body) {
		return
	}
	user, err := s.Passwords.CreateLocalAccount(request.Context(), body.Username, body.DisplayName, body.Password)
	if err != nil {
		respondError(response, http.StatusBadRequest, "local_user_not_created")
		return
	}
	respondJSON(response, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username, "mustChangePassword": true})
}

func decode(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 64<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		respondError(response, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func respondJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func respondError(response http.ResponseWriter, status int, code string) {
	respondJSON(response, status, map[string]string{"error": code})
}
