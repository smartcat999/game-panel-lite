package modcatalog

import "testing"

func TestRecommendedDSTModsUsesMaintainedFastTravel(t *testing.T) {
	items, err := RecommendedDSTMods()
	if err != nil {
		t.Fatalf("load recommended DST mods: %v", err)
	}

	var foundMaintained bool
	for _, item := range items {
		if item.WorkshopID == "458587300" {
			t.Fatal("legacy Fast Travel workshop item must not be recommended")
		}
		if item.WorkshopID == "1530801499" {
			foundMaintained = true
			if item.Title != "Fast Travel (GUI)" {
				t.Fatalf("unexpected maintained Fast Travel title %q", item.Title)
			}
		}
	}
	if !foundMaintained {
		t.Fatal("maintained Fast Travel workshop item is missing")
	}
}
