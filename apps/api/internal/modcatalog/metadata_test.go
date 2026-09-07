package modcatalog

import (
	"reflect"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestDependencyMetadataPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		item domain.ModFile
		want []string
	}{
		{"explicit names", domain.ModFile{Dependencies: []string{" A ", "A", "", "B"}, DependenciesJSON: `["ignored"]`}, []string{"A", "B"}},
		{"persisted names", domain.ModFile{DependenciesJSON: `[" B ", "B", "C"]`}, []string{"B", "C"}},
		{"empty persisted list overrides catalog", domain.ModFile{ProviderKey: domain.ProviderTerrariaTModLoader, ModName: "MagicStorage", DependenciesJSON: `[]`}, []string{}},
		{"catalog fallback", domain.ModFile{ProviderKey: domain.ProviderTerrariaTModLoader, ModName: "MagicStorage", DependenciesJSON: `invalid`}, []string{"SerousCommonLib"}},
		{"foreign provider", domain.ModFile{ProviderKey: domain.ProviderDST, ModName: "MagicStorage"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Dependencies(tc.item)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if len(got) > 0 && len(tc.item.Dependencies) > 0 {
				got[0] = "changed"
				if tc.item.Dependencies[0] == "changed" {
					t.Fatal("modified input slice")
				}
			}
		})
	}
}

func TestIdentityFallbacks(t *testing.T) {
	for _, tc := range []struct {
		item domain.ModFile
		want string
	}{
		{domain.ModFile{ModName: " name ", Title: "title", FileName: "file.tmod"}, "name"},
		{domain.ModFile{Title: " title ", FileName: "file.tmod"}, "title"},
		{domain.ModFile{FileName: "file.pak"}, "file"},
		{domain.ModFile{FileName: "workshop-123"}, ""},
	} {
		if got := Identity(tc.item); got != tc.want {
			t.Fatalf("identity=%q want=%q", got, tc.want)
		}
	}
}
