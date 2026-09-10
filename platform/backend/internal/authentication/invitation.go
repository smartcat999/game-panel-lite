package authentication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

type InvitationService struct {
	store    Store
	bindings authorization.BindingWriter
	now      func() time.Time
}

func NewInvitationService(store Store, bindings authorization.BindingWriter) *InvitationService {
	return &InvitationService{store: store, bindings: bindings, now: time.Now}
}

func (s *InvitationService) Create(ctx context.Context, workspaceID string, role authorization.Role, invitedBy string, lifetime time.Duration) (string, Invitation, error) {
	if role != authorization.RoleWorkspaceOwner && role != authorization.RoleWorkspaceOperator && role != authorization.RoleWorkspaceViewer {
		return "", Invitation{}, fmt.Errorf("invitation role must be workspace scoped")
	}
	token, err := randomToken(32)
	if err != nil {
		return "", Invitation{}, err
	}
	id, err := randomID("inv")
	if err != nil {
		return "", Invitation{}, err
	}
	now := s.now().UTC()
	invitation := Invitation{ID: id, WorkspaceID: workspaceID, Role: string(role), TokenHash: tokenHash(token), InvitedBy: invitedBy, ExpiresAt: now.Add(lifetime), CreatedAt: now}
	if err := s.store.PutInvitation(ctx, invitation); err != nil {
		return "", Invitation{}, err
	}
	return token, invitation, nil
}

func (s *InvitationService) Accept(ctx context.Context, token, userID string) (authorization.RoleBinding, error) {
	now := s.now().UTC()
	invitation, err := s.store.ConsumeInvitation(ctx, tokenHash(token), userID, now)
	if err != nil {
		return authorization.RoleBinding{}, ErrInvitationInvalid
	}
	bindingID := invitationBindingID(invitation.ID, userID)
	binding := authorization.RoleBinding{
		ID:          bindingID,
		PrincipalID: authorization.PrincipalID(userID),
		Role:        authorization.Role(invitation.Role),
		Scope:       authorization.Scope{Type: authorization.ScopeWorkspace, ID: invitation.WorkspaceID},
	}
	if err := s.bindings.Put(ctx, binding); err != nil {
		return authorization.RoleBinding{}, err
	}
	return binding, nil
}

func invitationBindingID(invitationID, userID string) string {
	sum := sha256.Sum256([]byte(invitationID + ":" + userID))
	return "rb_" + hex.EncodeToString(sum[:12])
}
