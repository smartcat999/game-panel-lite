package identity

import (
	"context"
	"testing"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestPreferencesPersistToUser(t *testing.T) {
	userID := contract.UserID("usr_one")
	module := New(Seed{
		Users:       []User{{ID: userID}},
		Preferences: map[contract.UserID]Preferences{userID: {Locale: "en", Theme: "system", TimeZone: "UTC"}},
	})

	want := Preferences{Locale: "zh-CN", Theme: "dark", TimeZone: "Asia/Shanghai"}
	if _, err := module.UpdateUserPreferences(context.Background(), userID, want); err != nil {
		t.Fatal(err)
	}
	got, err := module.UserPreferences(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("preferences did not persist: got %#v, want %#v", got, want)
	}
}

func TestPlatformAuthorityIsIndependentFromWorkspaceMembership(t *testing.T) {
	operatorID := contract.UserID("usr_operator")
	module := New(Seed{Users: []User{{ID: operatorID}}, PlatformOperators: []contract.UserID{operatorID}})
	if !module.IsPlatformOperator(context.Background(), operatorID) {
		t.Fatal("explicit Platform Operator should have platform authority")
	}
	if module.IsPlatformOperator(context.Background(), contract.UserID("usr_member")) {
		t.Fatal("a Workspace member must not inherit platform authority")
	}
}
