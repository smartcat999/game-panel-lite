package tmod

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func TestTModInspectDirNewestFirst(t *testing.T) {
	insp, ok := driver.LookupWorkshopInspector(addonmirror.SteamAppTModLoader)
	if !ok {
		t.Fatalf("expected tModLoader workshop inspector to be registered")
	}
	root := t.TempDir()
	for _, rel := range []string{
		"2024.2/SharedMod.tmod",
		"2024.10/NewestMod.tmod",
		"2024.10/SharedMod.tmod",
		"2024.1/LegacyMod.tmod",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("tmod"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res := insp.InspectDir(root)
	want := []string{"NewestMod", "SharedMod", "LegacyMod"}
	if !slices.Equal(res.ModIDs, want) {
		t.Fatalf("got ModIDs=%v, want %v", res.ModIDs, want)
	}
}

func TestTModInspectArchiveNewestFirst(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "tmod.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, entry := range []string{"2024.3/Alpha.tmod", "2024.11/Beta.tmod", "2024.3/Beta.tmod"} {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	res := workshopInspector{}.InspectArchive(registryv1.ArchiveFormatZip, zipPath)
	want := []string{"Beta", "Alpha"}
	if !slices.Equal(res.ModIDs, want) {
		t.Fatalf("archive got ModIDs=%v, want %v", res.ModIDs, want)
	}
}
