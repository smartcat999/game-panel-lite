package authorization

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type countingStore struct {
	bindings []RoleBinding
	loads    int
}

func (s *countingStore) BindingsForPrincipal(_ context.Context, _ PrincipalID) ([]RoleBinding, error) {
	s.loads++
	return append([]RoleBinding(nil), s.bindings...), nil
}

func TestBatchAuthorizationLoadsBindingsOnceForHundredResources(t *testing.T) {
	principalID := PrincipalID("usr_operator")
	store := &countingStore{bindings: []RoleBinding{{ID: "rb_workspace", PrincipalID: principalID, Role: RoleWorkspaceOperator, Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}}}}
	checks := make([]Check, MaxBatchSize)
	for index := range checks {
		checks[index] = Check{ResourceType: "instance", ResourceID: fmt.Sprintf("lin_%03d", index), Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}, Action: ActionInstanceRestart}
	}
	decisions, err := NewEngine(store).CheckBatch(context.Background(), principalID, checks)
	if err != nil {
		t.Fatal(err)
	}
	if store.loads != 1 {
		t.Fatalf("binding query count = %d, want 1", store.loads)
	}
	for _, decision := range decisions {
		if !decision.Allowed {
			t.Fatalf("authorized resource denied: %#v", decision)
		}
	}
}

func TestScopesAreIndependentAndSensitiveCustomerActionsAreNotInherited(t *testing.T) {
	principalID := PrincipalID("usr_platform")
	store, err := NewMemoryStore([]RoleBinding{
		{ID: "rb_platform", PrincipalID: principalID, Role: RolePlatformAdmin, Scope: Scope{Type: ScopePlatform, ID: "platform"}},
		{ID: "rb_region", PrincipalID: principalID, Role: RoleRegionOperator, Scope: Scope{Type: ScopeRegion, ID: "reg_east"}},
		{ID: "rb_workspace", PrincipalID: principalID, Role: RoleWorkspaceViewer, Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := []Check{
		{ResourceType: "platform", ResourceID: "platform", Scope: Scope{Type: ScopePlatform, ID: "platform"}, Action: ActionPlatformGrantCredit},
		{ResourceType: "region", ResourceID: "reg_east", Scope: Scope{Type: ScopeRegion, ID: "reg_east"}, Action: ActionRegionOperate},
		{ResourceType: "instance", ResourceID: "lin_ember", Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}, Action: ActionInstanceRead},
		{ResourceType: "instance", ResourceID: "lin_other", Scope: Scope{Type: ScopeWorkspace, ID: "ws_other"}, Action: ActionInstanceRead},
		{ResourceType: "instance", ResourceID: "lin_ember", Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}, Action: ActionInstanceConsole},
		{ResourceType: "backup", ResourceID: "bkp_ember", Scope: Scope{Type: ScopeWorkspace, ID: "ws_ember"}, Action: ActionBackupDownload},
	}
	decisions, err := NewEngine(store).CheckBatch(context.Background(), principalID, checks)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, true, true, false, false, false}
	for index, decision := range decisions {
		if decision.Allowed != want[index] {
			t.Fatalf("decision %d allowed=%v want=%v", index, decision.Allowed, want[index])
		}
	}
}

func TestBindingScopeMustMatchRoleAndBatchIsBounded(t *testing.T) {
	if err := ValidateBinding(RoleBinding{ID: "rb_bad", PrincipalID: "usr_one", Role: RoleRegionOperator, Scope: Scope{Type: ScopeWorkspace, ID: "ws_one"}}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("ValidateBinding error = %v", err)
	}
	store, _ := NewMemoryStore(nil)
	_, err := NewEngine(store).CheckBatch(context.Background(), "usr_one", make([]Check, MaxBatchSize+1))
	if !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("CheckBatch error = %v", err)
	}
}
