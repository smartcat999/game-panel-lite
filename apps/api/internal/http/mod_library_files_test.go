package http

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestLibraryCopyAndCleanupUsePersistedOwnership(t *testing.T) {
	dir := t.TempDir()
	mods := modruntime.NewService(mustRegistry(t, terraria.NewTModLoaderProvider()), nil)
	h := &Handler{cfg: config.Config{DataDir: dir}, modRuntime: mods}
	files := modsvc.NewService(dir, mods.StoredFileName)
	item := domain.ModFile{ID: "private-mod", OrganizationID: "workspace-a", InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "same.tmod"}
	if _, err := files.PutLibrary(context.Background(), item, strings.NewReader("owned-content"), 100); err != nil {
		t.Fatal(err)
	}
	if _, _, err := files.Upload("unassigned", item.ProviderKey, item.FileName, strings.NewReader("legacy-content")); err != nil {
		t.Fatal(err)
	}
	if size, err := h.copyLibraryModToServerCache(item, "target"); err != nil || size != 13 {
		t.Fatalf("copy: %d %v", size, err)
	}
	target := item
	target.OrganizationID = ""
	target.InstanceID = "target"
	read := func(record domain.ModFile) string {
		t.Helper()
		file, err := files.Open(record)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := read(target); got != "owned-content" {
		t.Fatalf("copied legacy source: %q", got)
	}
	if err := h.removeCachedMod(item); err != nil {
		t.Fatal(err)
	}
	legacy := item
	legacy.OrganizationID = ""
	if got := read(legacy); got != "legacy-content" {
		t.Fatalf("legacy bytes changed: %q", got)
	}
	if got := read(target); got != "owned-content" {
		t.Fatalf("installed cache changed: %q", got)
	}
}
