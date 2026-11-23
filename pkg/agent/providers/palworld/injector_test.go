package palworld

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

func TestPalworldInjectorRoutesLogicModsLuaAndModsTxt(t *testing.T) {
	wsDir := t.TempDir()
	ext1 := t.TempDir()
	// Thunderstore BasesPlus style (LogicMods + Win64/Mods/BasesPlus/Scripts + Enabled.txt)
	_ = os.MkdirAll(filepath.Join(ext1, "Pal", "Content", "Paks", "LogicMods"), 0o755)
	_ = os.WriteFile(filepath.Join(ext1, "Pal", "Content", "Paks", "LogicMods", "BasesPlus.pak"), []byte("pak1"), 0o644)
	_ = os.WriteFile(filepath.Join(ext1, "Pal", "Content", "Paks", "LogicMods", "BasesPlus.modconfig.json"), []byte("{}"), 0o644)
	_ = os.MkdirAll(filepath.Join(ext1, "Pal", "Binaries", "Win64", "Mods", "BasesPlus", "Scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(ext1, "Pal", "Binaries", "Win64", "Mods", "BasesPlus", "Scripts", "main.lua"), []byte("lua1"), 0o644)
	_ = os.WriteFile(filepath.Join(ext1, "Pal", "Binaries", "Win64", "Mods", "BasesPlus", "Enabled.txt"), []byte(""), 0o644)

	ext2 := t.TempDir()
	// Thunderstore ElementalRebalance style (pak/ + mod/ + Windows dll ignored)
	_ = os.MkdirAll(filepath.Join(ext2, "pak"), 0o755)
	_ = os.WriteFile(filepath.Join(ext2, "pak", "ElementalRebalance_P.pak"), []byte("pak2"), 0o644)
	_ = os.MkdirAll(filepath.Join(ext2, "mod", "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(ext2, "mod", "scripts", "main.lua"), []byte("lua2"), 0o644)
	_ = os.WriteFile(filepath.Join(ext2, "mod", "bad.dll"), []byte("win"), 0o644)

	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	ws := &driver.WorkspaceContext{Root: root, DisableReflink: true, Next: &driver.AddonsManifest{}, State: map[string]any{}}
	inj := &Injector{}
	ws.BlobExtractedDir = func(string) string { return ext1 }
	if err := inj.Inject(ws, corev1.InstanceAddon{Package: "PalModders-BasesPlus"}, &driver.CachedArtifactMeta{Coordinate: "a1", Digest: "d1", HexDigest: "h1", ArchiveFormat: registryv1.ArchiveFormatZip}); err != nil {
		t.Fatalf("inject1: %v", err)
	}
	ws.BlobExtractedDir = func(string) string { return ext2 }
	if err := inj.Inject(ws, corev1.InstanceAddon{Package: "dubcats-ElementalRebalance"}, &driver.CachedArtifactMeta{Coordinate: "a2", Digest: "d2", HexDigest: "h2", ArchiveFormat: registryv1.ArchiveFormatZip}); err != nil {
		t.Fatalf("inject2: %v", err)
	}
	if err := inj.Finalize(ws); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	assertExists(t, filepath.Join(wsDir, "Pal", "Content", "Paks", "LogicMods", "BasesPlus.pak"))
	assertExists(t, filepath.Join(wsDir, "Pal", "Content", "Paks", "~mods", "ElementalRebalance_P.pak"))
	assertExists(t, filepath.Join(wsDir, "Pal", "Binaries", "Linux", "Mods", "BasesPlus", "scripts", "main.lua"))
	assertExists(t, filepath.Join(wsDir, "Pal", "Binaries", "Linux", "Mods", "BasesPlus", "enabled.txt"))
	if _, err := os.Stat(filepath.Join(wsDir, "Pal", "Binaries", "Linux", "Mods", "dubcats-ElementalRebalance", "bad.dll")); !os.IsNotExist(err) {
		t.Fatalf("expected Windows .dll skipped")
	}
	modsTxt, err := os.ReadFile(filepath.Join(wsDir, "Pal", "Binaries", "Linux", "Mods", "mods.txt"))
	if err != nil || !strings.Contains(string(modsTxt), "BasesPlus : 1") || !strings.Contains(string(modsTxt), "dubcats-ElementalRebalance : 1") {
		t.Fatalf("unexpected mods.txt: %q err=%v", string(modsTxt), err)
	}
	inj.Cleanup(root, *ws.Next)
	after, _ := os.ReadFile(filepath.Join(wsDir, "Pal", "Binaries", "Linux", "Mods", "mods.txt"))
	if strings.Contains(string(after), "BasesPlus : 1") {
		t.Fatalf("expected Cleanup to remove managed block, got %q", string(after))
	}
}

func assertExists(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file %s: %v", p, err)
	}
}
