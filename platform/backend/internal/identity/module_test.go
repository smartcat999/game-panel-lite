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

func TestRegionOperatorRequiresPlatformAuthorityAndExplicitScope(t *testing.T) {
	operatorID := contract.UserID("usr_region_operator")
	scopedWithoutPlatform := contract.UserID("usr_scoped_member")
	module := New(Seed{
		PlatformOperators: []contract.UserID{operatorID},
		RegionOperators: map[contract.UserID][]contract.RegionID{
			operatorID:            {"reg_one"},
			scopedWithoutPlatform: {"reg_one"},
		},
	})
	if !module.IsRegionOperator(context.Background(), operatorID, "reg_one") {
		t.Fatal("scoped Platform Operator was rejected")
	}
	if module.IsRegionOperator(context.Background(), operatorID, "reg_two") {
		t.Fatal("Region scope leaked")
	}
	if module.IsRegionOperator(context.Background(), scopedWithoutPlatform, "reg_one") {
		t.Fatal("Region scope manufactured Platform authority")
	}
}
