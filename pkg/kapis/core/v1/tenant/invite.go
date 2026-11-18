package tenant

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
	tenantrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrolebinding"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
)

const defaultInviteTTL = 7 * 24 * time.Hour

type inviteClaims struct {
	Tenant     string `json:"t"`
	TenantUID  string `json:"u"`
	Role       string `json:"r"`
	Inviter    string `json:"i"`
	ServerName string `json:"s,omitempty"`
	ExpiresAt  int64  `json:"e"`
}

func inviteSigningSecret() []byte {
	if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.Auth.JWTSecret) != "" {
		return []byte(cfg.Auth.JWTSecret)
	}
	return []byte("gamepanel-default-jwt-secret")
}

func signInviteClaims(c inviteClaims) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, inviteSigningSecret())
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig, nil
}

func parseInviteToken(token string) (*inviteClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("%w: invalid invite token format", models.ErrBadRequest)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite signature", models.ErrBadRequest)
	}
	mac := hmac.New(sha256.New, inviteSigningSecret())
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, fmt.Errorf("%w: invite signature verification failed", models.ErrForbidden)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite payload", models.ErrBadRequest)
	}
	var claims inviteClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("%w: malformed invite claims", models.ErrBadRequest)
	}
	if time.Now().UTC().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("%w: invite link has expired", models.ErrForbidden)
	}
	return &claims, nil
}

func normalizeInviteRole(role string) (string, error) {
	r := strings.TrimSpace(role)
	if r == "" {
		return "instance-operator", nil
	}
	switch r {
	case "instance-operator", "tenant-viewer", "tenant-admin":
		return r, nil
	default:
		return "", fmt.Errorf("%w: unsupported workspace invite role %q", models.ErrBadRequest, r)
	}
}

func (h *handler) CreateTenantInvite(w http.ResponseWriter, r *http.Request) {
	request.SkipAudit(r.Context())
	tenantName := r.PathValue("tenant")
	var req corev1.TenantInvite
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req)
	}
	role, err := normalizeInviteRole(req.Role)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	tenantObj, err := h.op.Get(r.Context(), tenantName)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if tenantObj.Metadata.DeletionTimestamp != nil {
		apiruntime.WriteError(w, http.StatusConflict, "tenant is terminating")
		return
	}
	userInfo, _ := request.UserFrom(r.Context())
	exp := time.Now().UTC().Add(defaultInviteTTL)
	token, err := signInviteClaims(inviteClaims{
		Tenant:     tenantObj.Metadata.Name,
		TenantUID:  tenantObj.Metadata.UID,
		Role:       role,
		Inviter:    userInfo.Name,
		ServerName: strings.TrimSpace(req.ServerName),
		ExpiresAt:  exp.Unix(),
	})
	if err != nil {
		apiruntime.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := buildTenantInviteResponse(tenantObj, token, role, userInfo.Name, strings.TrimSpace(req.ServerName), exp)
	apiruntime.WriteJSON(w, http.StatusCreated, out)
}

func buildTenantInviteResponse(t *corev1.Tenant, token, role, inviter, serverName string, exp time.Time) *corev1.TenantInvite {
	displayName := strings.TrimSpace(t.Spec.DisplayName)
	if displayName == "" {
		displayName = t.Metadata.Name
	}
	return &corev1.TenantInvite{
		TypeMeta:          metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "TenantInvite"},
		Metadata:          metav1.ObjectMeta{Name: token, Tenant: t.Metadata.Name, UID: t.Metadata.UID},
		TenantDisplayName: displayName,
		Inviter:           inviter,
		Role:              role,
		ServerName:        serverName,
		Token:             token,
		ExpiresAt:         &exp,
	}
}

func (h *handler) verifyTenantInviteRequest(r *http.Request) (*corev1.Tenant, *inviteClaims, error) {
	claims, err := parseInviteToken(r.PathValue("token"))
	if err != nil {
		return nil, nil, err
	}
	if claims.Tenant == "" || claims.Tenant != r.PathValue("tenant") {
		return nil, nil, fmt.Errorf("%w: invite tenant mismatch", models.ErrForbidden)
	}
	tenantObj, err := h.op.Get(r.Context(), claims.Tenant)
	if err != nil {
		return nil, nil, err
	}
	if tenantObj.Metadata.DeletionTimestamp != nil || tenantObj.Metadata.UID != claims.TenantUID {
		return nil, nil, fmt.Errorf("%w: invited workspace is no longer active", models.ErrForbidden)
	}
	return tenantObj, claims, nil
}

func (h *handler) GetTenantInvite(w http.ResponseWriter, r *http.Request) {
	tenantObj, claims, err := h.verifyTenantInviteRequest(r)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	exp := time.Unix(claims.ExpiresAt, 0).UTC()
	out := buildTenantInviteResponse(tenantObj, r.PathValue("token"), claims.Role, claims.Inviter, claims.ServerName, exp)
	apiruntime.WriteJSON(w, http.StatusOK, out)
}

func (h *handler) AcceptTenantInvite(w http.ResponseWriter, r *http.Request) {
	request.SetAuditTarget(r.Context(), corev1.InviteActionAccept, r.PathValue("tenant"), nil)
	tenantObj, claims, err := h.verifyTenantInviteRequest(r)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	auditAttrs := map[string]string{"role": claims.Role, "inviter": claims.Inviter}
	request.SetAuditTarget(r.Context(), corev1.InviteActionAccept, tenantObj.Metadata.Name, auditAttrs)
	var req corev1.TenantInvite
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req)
	}
	username, sessionToken, err := h.resolveOrProvisionInvitedUser(r.Context(), &req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditActor(r.Context(), username)
	request.SetAuditResourceName(r.Context(), "member-"+username)
	request.SetAuditTarget(r.Context(), corev1.InviteActionAccept, username, auditAttrs)
	if err := h.ensureInvitedMemberBinding(r.Context(), tenantObj, username, claims.Role); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	exp := time.Unix(claims.ExpiresAt, 0).UTC()
	out := buildTenantInviteResponse(tenantObj, r.PathValue("token"), claims.Role, claims.Inviter, claims.ServerName, exp)
	out.Username = username
	out.SessionToken = sessionToken
	apiruntime.WriteJSON(w, http.StatusOK, out)
}

func (h *handler) resolveOrProvisionInvitedUser(ctx context.Context, req *corev1.TenantInvite) (string, string, error) {
	if u, ok := request.UserFrom(ctx); ok && !u.IsAnonymous() && u.Name != "" {
		return u.Name, "", nil
	}
	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		return "", "", fmt.Errorf("%w: username and password are required", models.ErrBadRequest)
	}
	userOp := usermodel.NewOperatorWithRevoker(h.reg, h.sessions, h.mq)
	if err := h.ensureInviteAccount(ctx, userOp, req, username, password); err != nil {
		return "", "", err
	}
	sessOp := h.sessions
	if sessOp == nil {
		sessOp = sessionmodel.NewOperator(h.reg, nil, nil)
	}
	sess, err := sessOp.Create(ctx, &iamv1.Session{
		Kind:       iamv1.SubjectKindUser,
		Identifier: username,
		User:       username,
		Password:   password,
	})
	if err != nil {
		return "", "", err
	}
	return sess.User, sess.Token, nil
}

// ensureInviteAccount enforces the invitee's chosen mode: Login never
// provisions a new account (a mistyped username must not silently register),
// Register never reuses an existing one, and empty mode keeps the legacy
// register-or-login behavior.
func (h *handler) ensureInviteAccount(ctx context.Context, userOp usermodel.Operator, req *corev1.TenantInvite, username, password string) error {
	if req.Mode == corev1.TenantInviteModeLogin {
		// Session authentication resolves username or email and rejects
		// unknown identities; Login must never provision an account.
		return nil
	}
	_, err := userOp.Get(ctx, username)
	exists := err == nil
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	switch req.Mode {
	case corev1.TenantInviteModeRegister:
		if exists {
			return fmt.Errorf("%w: username already taken", models.ErrAlreadyExists)
		}
	case "":
		if exists {
			return nil
		}
	default:
		return fmt.Errorf("%w: unsupported invite mode %q", models.ErrBadRequest, req.Mode)
	}
	created, err := userOp.Create(ctx, &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: username},
		Spec:     iamv1.UserSpec{Email: strings.TrimSpace(req.Email), Password: password, State: iamv1.UserStateActive},
	})
	if err != nil {
		return err
	}
	_ = usermodel.EnsurePersonalTenantForUID(ctx, h.reg, created.Metadata.Name, created.Metadata.UID)
	return nil
}

func (h *handler) ensureInvitedMemberBinding(ctx context.Context, tenantObj *corev1.Tenant, username, role string) error {
	if username == "" || username == tenantObj.Spec.Owner {
		return nil
	}
	_ = tenantrolemodel.EnsureBuiltInRoles(ctx, h.reg, tenantObj.Metadata.Name, tenantObj.Spec.Owner)
	trbOp := tenantrolebindingmodel.NewOperator(h.reg, h.mq)
	bindName := "member-" + username
	if existing, err := trbOp.Get(ctx, tenantObj.Metadata.Name, bindName); err == nil && existing != nil {
		return nil
	}
	_, err := trbOp.Create(ctx, tenantObj.Metadata.Name, &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: bindName, Tenant: tenantObj.Metadata.Name},
		RoleRef:  iamv1.RoleRef{Name: role},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: username}},
	})
	if errors.Is(err, db.ErrAlreadyExists) {
		return nil
	}
	return err
}
