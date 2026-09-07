package workload

import (
	"strings"
	"testing"
)

func TestArtifactDestinations(t *testing.T) {
	valid := Artifact{ID: "immutable-id", Path: "Mods/mod.bin", SHA256: strings.Repeat("a", 64), SizeBytes: 10}
	if err := ValidateArtifacts(Options{Artifacts: []Artifact{valid}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", ".", "..", "../escape", "/absolute", "Mods/../escape", "Mods\\escape", "C:/file", "Mods//file", ".gamepanel-prepare-old/file", "Mods/.gamepanel-prepare-old/file", "Mods/file.", "Mods/file "} {
		item := valid
		item.Path = name
		if err := ValidateArtifacts(Options{Artifacts: []Artifact{item}}); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, name := range []string{"Mods/mod.bin", "mods/MOD.bin", "Mods", "Mods/mod.bin/nested"} {
		if err := ValidateArtifacts(Options{Artifacts: []Artifact{valid}, Files: map[string]string{name: "config"}}); err == nil {
			t.Fatalf("accepted conflict %q", name)
		}
	}
	other := valid
	other.Path = "mods/MOD.bin"
	if err := ValidateArtifacts(Options{Artifacts: []Artifact{valid, other}}); err == nil {
		t.Fatal("accepted case alias")
	}
	for _, edit := range []func(*Artifact){func(a *Artifact) { a.ID = "../other" }, func(a *Artifact) { a.SizeBytes = 0 }, func(a *Artifact) { a.SHA256 = "bad" }, func(a *Artifact) { a.SHA256 = strings.Repeat("A", 64) }} {
		item := valid
		edit(&item)
		if err := ValidateArtifacts(Options{Artifacts: []Artifact{item}}); err == nil {
			t.Fatalf("accepted invalid identity: %+v", item)
		}
	}
}
