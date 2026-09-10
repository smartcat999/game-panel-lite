package authorization

import (
	"context"
	"errors"
	"fmt"
)

const MaxBatchSize = 100

type PrincipalID string
type Action string
type Role string
type ScopeType string

const (
	ScopePlatform  ScopeType = "platform"
	ScopeRegion    ScopeType = "region"
	ScopeWorkspace ScopeType = "workspace"
)

const (
	RolePlatformAdmin     Role = "platform.admin"
	RoleRegionOperator    Role = "region.operator"
	RoleWorkspaceOwner    Role = "workspace.owner"
	RoleWorkspaceOperator Role = "workspace.operator"
	RoleWorkspaceViewer   Role = "workspace.viewer"
)

const (
	ActionPlatformRead        Action = "platform.read"
	ActionPlatformManageUser  Action = "platform.user.manage"
	ActionPlatformGrantCredit Action = "platform.credit.grant"
	ActionPlatformPriceWrite  Action = "platform.price.write"
	ActionRegionRead          Action = "region.read"
	ActionRegionOperate       Action = "region.operate"
	ActionWorkspaceRead       Action = "workspace.read"
	ActionWorkspaceManage     Action = "workspace.manage"
	ActionMemberRead          Action = "member.read"
	ActionMemberInvite        Action = "member.invite"
	ActionBillingRead         Action = "billing.read"
	ActionInstanceRead        Action = "instance.read"
	ActionInstanceCreate      Action = "instance.create"
	ActionInstanceStart       Action = "instance.start"
	ActionInstanceStop        Action = "instance.stop"
	ActionInstanceRestart     Action = "instance.restart"
	ActionInstanceConfigure   Action = "instance.configure"
	ActionInstanceConsole     Action = "instance.console"
	ActionInstanceSecretRead  Action = "instance.secret.read"
	ActionBackupRead          Action = "backup.read"
	ActionBackupCreate        Action = "backup.create"
	ActionBackupRestore       Action = "backup.restore"
	ActionBackupDownload      Action = "backup.download"
)

var (
	ErrInvalidBinding  = errors.New("invalid role binding")
	ErrBatchTooLarge   = errors.New("authorization batch exceeds limit")
	ErrTooManyBindings = errors.New("principal role bindings exceed limit")
)

type Scope struct {
	Type ScopeType `json:"type"`
	ID   string    `json:"id"`
}

type RoleBinding struct {
	ID          string      `json:"id"`
	PrincipalID PrincipalID `json:"principalId"`
	Role        Role        `json:"role"`
	Scope       Scope       `json:"scope"`
}

type Check struct {
	ResourceType string
	ResourceID   string
	Scope        Scope
	Action       Action
}

type Decision struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Action       Action `json:"action"`
	Allowed      bool   `json:"allowed"`
	Reason       string `json:"reason,omitempty"`
}

type BindingStore interface {
	BindingsForPrincipal(context.Context, PrincipalID) ([]RoleBinding, error)
}

type BindingWriter interface {
	Put(context.Context, RoleBinding) error
}

type Engine struct {
	store BindingStore
}

func NewEngine(store BindingStore) *Engine {
	return &Engine{store: store}
}

func (e *Engine) CheckBatch(ctx context.Context, principalID PrincipalID, checks []Check) ([]Decision, error) {
	if len(checks) > MaxBatchSize {
		return nil, ErrBatchTooLarge
	}
	bindings, err := e.store.BindingsForPrincipal(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("load role bindings: %w", err)
	}
	decisions := make([]Decision, len(checks))
	for index, check := range checks {
		allowed := false
		for _, binding := range bindings {
			if binding.PrincipalID == principalID && binding.Scope == check.Scope && roleAllows(binding.Role, check.Action) {
				allowed = true
				break
			}
		}
		decisions[index] = Decision{ResourceType: check.ResourceType, ResourceID: check.ResourceID, Action: check.Action, Allowed: allowed}
		if !allowed {
			decisions[index].Reason = "no_matching_role_binding"
		}
	}
	return decisions, nil
}

func ValidateBinding(binding RoleBinding) error {
	wantScope, ok := roleScope[binding.Role]
	if !ok || binding.ID == "" || binding.PrincipalID == "" || binding.Scope.ID == "" || binding.Scope.Type != wantScope {
		return ErrInvalidBinding
	}
	return nil
}

var roleScope = map[Role]ScopeType{
	RolePlatformAdmin:     ScopePlatform,
	RoleRegionOperator:    ScopeRegion,
	RoleWorkspaceOwner:    ScopeWorkspace,
	RoleWorkspaceOperator: ScopeWorkspace,
	RoleWorkspaceViewer:   ScopeWorkspace,
}

var rolePermissions = map[Role]map[Action]struct{}{
	RolePlatformAdmin: permissionSet(
		ActionPlatformRead,
		ActionPlatformManageUser,
		ActionPlatformGrantCredit,
		ActionPlatformPriceWrite,
	),
	RoleRegionOperator: permissionSet(ActionRegionRead, ActionRegionOperate),
	RoleWorkspaceOwner: permissionSet(
		ActionWorkspaceRead,
		ActionWorkspaceManage,
		ActionMemberRead,
		ActionMemberInvite,
		ActionBillingRead,
		ActionInstanceRead,
		ActionInstanceCreate,
		ActionInstanceStart,
		ActionInstanceStop,
		ActionInstanceRestart,
		ActionInstanceConfigure,
		ActionInstanceConsole,
		ActionInstanceSecretRead,
		ActionBackupRead,
		ActionBackupCreate,
		ActionBackupRestore,
		ActionBackupDownload,
	),
	RoleWorkspaceOperator: permissionSet(
		ActionWorkspaceRead,
		ActionMemberRead,
		ActionInstanceRead,
		ActionInstanceCreate,
		ActionInstanceStart,
		ActionInstanceStop,
		ActionInstanceRestart,
		ActionInstanceConfigure,
		ActionInstanceConsole,
		ActionBackupRead,
		ActionBackupCreate,
		ActionBackupRestore,
	),
	RoleWorkspaceViewer: permissionSet(ActionWorkspaceRead, ActionMemberRead, ActionInstanceRead, ActionBackupRead, ActionBillingRead),
}

func roleAllows(role Role, action Action) bool {
	_, ok := rolePermissions[role][action]
	return ok
}

func permissionSet(actions ...Action) map[Action]struct{} {
	result := make(map[Action]struct{}, len(actions))
	for _, action := range actions {
		result[action] = struct{}{}
	}
	return result
}
