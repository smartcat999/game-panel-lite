package workspace

import (
	"context"
	"errors"
	"testing"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestUserSwitchesBetweenTwoWorkspacesWithoutSelectionLeak(t *testing.T) {
	firstUser := contract.UserID("usr_first")
	secondUser := contract.UserID("usr_second")
	firstWorkspace := contract.WorkspaceID("ws_first")
	secondWorkspace := contract.WorkspaceID("ws_second")
	module := New(Seed{
		Workspaces: []Workspace{{ID: firstWorkspace}, {ID: secondWorkspace}},
		Memberships: []Membership{
			{ID: contract.MembershipID("mem_1"), UserID: firstUser, WorkspaceID: firstWorkspace, Role: RoleOwner},
			{ID: contract.MembershipID("mem_2"), UserID: firstUser, WorkspaceID: secondWorkspace, Role: RoleOperator},
			{ID: contract.MembershipID("mem_3"), UserID: secondUser, WorkspaceID: firstWorkspace, Role: RoleViewer},
		},
		Selections: map[contract.UserID]contract.WorkspaceID{firstUser: firstWorkspace, secondUser: firstWorkspace},
	})

	if got := module.ListForUser(context.Background(), firstUser); len(got) != 2 {
		t.Fatalf("expected two Workspaces, got %d", len(got))
	}
	if err := module.Select(context.Background(), firstUser, secondWorkspace); err != nil {
		t.Fatal(err)
	}
	if selected, _ := module.Selected(context.Background(), firstUser); selected != secondWorkspace {
		t.Fatalf("first User selection = %q, want %q", selected, secondWorkspace)
	}
	if selected, _ := module.Selected(context.Background(), secondUser); selected != firstWorkspace {
		t.Fatalf("second User selection leaked: got %q, want %q", selected, firstWorkspace)
	}
}

func TestCrossWorkspaceIDIsForbidden(t *testing.T) {
	userID := contract.UserID("usr_member")
	allowedWorkspace := contract.WorkspaceID("ws_allowed")
	otherWorkspace := contract.WorkspaceID("ws_other")
	module := New(Seed{
		Workspaces:  []Workspace{{ID: allowedWorkspace}, {ID: otherWorkspace}},
		Memberships: []Membership{{ID: contract.MembershipID("mem_allowed"), UserID: userID, WorkspaceID: allowedWorkspace, Role: RoleOwner}},
	})

	_, err := module.Members(context.Background(), userID, otherWorkspace)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-Workspace access error = %v, want ErrForbidden", err)
	}
}
