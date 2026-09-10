package workspace

import (
	"context"
	"errors"
	"sort"
	"sync"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrForbidden         = errors.New("workspace access forbidden")
	ErrWorkspaceNotFound = errors.New("workspace not found")
)

type Role string

const (
	RoleOwner         Role = "owner"
	RoleAdministrator Role = "administrator"
	RoleOperator      Role = "operator"
	RoleBilling       Role = "billing"
	RoleViewer        Role = "viewer"
)

type Workspace struct {
	ID   contract.WorkspaceID `json:"id"`
	Slug string               `json:"slug"`
	Name string               `json:"name"`
}

type Membership struct {
	ID          contract.MembershipID `json:"id"`
	WorkspaceID contract.WorkspaceID  `json:"workspaceId"`
	UserID      contract.UserID       `json:"userId"`
	Role        Role                  `json:"role"`
}

type Seed struct {
	Workspaces  []Workspace
	Memberships []Membership
	Selections  map[contract.UserID]contract.WorkspaceID
}

type Module struct {
	mu          sync.RWMutex
	workspaces  map[contract.WorkspaceID]Workspace
	memberships map[contract.WorkspaceID][]Membership
	selections  map[contract.UserID]contract.WorkspaceID
}

func New(seed Seed) *Module {
	module := &Module{
		workspaces:  make(map[contract.WorkspaceID]Workspace),
		memberships: make(map[contract.WorkspaceID][]Membership),
		selections:  make(map[contract.UserID]contract.WorkspaceID),
	}
	for _, item := range seed.Workspaces {
		module.workspaces[item.ID] = item
	}
	for _, membership := range seed.Memberships {
		module.memberships[membership.WorkspaceID] = append(module.memberships[membership.WorkspaceID], membership)
	}
	for userID, workspaceID := range seed.Selections {
		module.selections[userID] = workspaceID
	}
	return module
}

func (m *Module) ListForUser(_ context.Context, userID contract.UserID) []Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Workspace
	for workspaceID, memberships := range m.memberships {
		if containsUser(memberships, userID) {
			result = append(result, m.workspaces[workspaceID])
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Module) All(_ context.Context) []Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Workspace, 0, len(m.workspaces))
	for _, item := range m.workspaces {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Module) Select(ctx context.Context, userID contract.UserID, workspaceID contract.WorkspaceID) error {
	if err := m.RequireMembership(ctx, userID, workspaceID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.selections[userID] = workspaceID
	return nil
}

func (m *Module) Selected(_ context.Context, userID contract.UserID) (contract.WorkspaceID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	workspaceID, ok := m.selections[userID]
	return workspaceID, ok
}

func (m *Module) Members(ctx context.Context, actorID contract.UserID, workspaceID contract.WorkspaceID) ([]Membership, error) {
	if err := m.RequireMembership(ctx, actorID, workspaceID); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Membership(nil), m.memberships[workspaceID]...), nil
}

func (m *Module) RequireMembership(_ context.Context, userID contract.UserID, workspaceID contract.WorkspaceID) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workspaces[workspaceID]; !ok {
		return ErrWorkspaceNotFound
	}
	if !containsUser(m.memberships[workspaceID], userID) {
		return ErrForbidden
	}
	return nil
}

func containsUser(memberships []Membership, userID contract.UserID) bool {
	for _, membership := range memberships {
		if membership.UserID == userID {
			return true
		}
	}
	return false
}
