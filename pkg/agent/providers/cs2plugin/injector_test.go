package cs2plugin

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

const sampleGameInfo = "\"GameInfo\"\n{\n\tFileSystem\n\t{\n\t\tSearchPaths\n\t\t{\n\t\t\tGame_LowViolence\tcsgo_lv\n\t\t\tGame\tcsgo\n\t\t}\n\t}\n}\n"

func writeFile(t *testing.T, rootDir, rel, content string) {
	t.Helper()
	p := filepath.Join(rootDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func newTestWS(t *testing.T, workDir string) (string, *os.Root, *driver.WorkspaceContext) {
	t.Helper()
	wsDir := filepath.Join(workDir, "ws")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	ws := &driver.WorkspaceContext{
		Root:             root,
		DisableReflink:   true,
		Next:             &driver.AddonsManifest{Digests: make(map[string]string)},
		State:            make(map[string]any),
		BlobFilePath:     func(hex string) string { return filepath.Join(workDir, "blobs", hex) },
		BlobExtractedDir: func(hex string) string { return filepath.Join(workDir, "extracted", hex) },
	}
	return wsDir, root, ws
}

func TestCS2PluginInjectorMetamodCSSAndCleanup(t *testing.T) {
	tmp := t.TempDir()
	wsDir, root, ws := newTestWS(t, tmp)
	writeFile(t, wsDir, gameInfoRelPath, sampleGameInfo)
	writeFile(t, wsDir, "game/csgo/addons/counterstrikesharp/configs/core.json", `{"Prefix":"!custom"}`)

	writeFile(t, filepath.Join(tmp, "extracted", "mmshex"), "addons/metamod/bin/metamod.2.csgo.so", "mms")
	writeFile(t, filepath.Join(tmp, "extracted", "csshex"), "addons/metamod/counterstrikesharp.vdf", "vdf")
	writeFile(t, filepath.Join(tmp, "extracted", "csshex"), "addons/counterstrikesharp/configs/core.json", `{"Prefix":"."}`)
	writeFile(t, filepath.Join(tmp, "extracted", "rtvhex"), "addons/counterstrikesharp/plugins/RockTheVote/RockTheVote.dll", "dll")

	inj := &Injector{}
	injectTestPkg(t, inj, ws, "metamod", "mmshex", registryv1.ArchiveFormatTarGz)
	injectTestPkg(t, inj, ws, "counterstrikesharp", "csshex", registryv1.ArchiveFormatZip)
	injectTestPkg(t, inj, ws, "abnerfs--cs2-rockthevote", "rtvhex", registryv1.ArchiveFormatZip)
	if err := inj.Finalize(ws); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	assertFileContains(t, filepath.Join(wsDir, gameInfoRelPath), "csgo/addons/metamod")
	assertFileContains(t, filepath.Join(wsDir, "game/csgo/addons/counterstrikesharp/configs/core.json"), `"!custom"`)
	if !slices.Contains(ws.Next.Files, "game/csgo/addons/counterstrikesharp/plugins/RockTheVote/RockTheVote.dll") {
		t.Fatalf("expected plugin DLL tracked in ws.Next.Files: %v", ws.Next.Files)
	}
	if _, err := root.Lstat("game/csgo/addons/metamod/bin/metamod.2.csgo.so"); err != nil {
		t.Fatalf("expected metamod binary materialized: %v", err)
	}

	inj.Cleanup(root, driver.AddonsManifest{})
	rawAfter, err := os.ReadFile(filepath.Join(wsDir, gameInfoRelPath))
	if err != nil || strings.Contains(string(rawAfter), "csgo/addons/metamod") || !strings.Contains(string(rawAfter), "Game_LowViolence") {
		t.Fatalf("expected Cleanup to remove only metamod SearchPath line, err=%v got:\n%s", err, rawAfter)
	}
}

func TestMapCS2ArchiveLayouts(t *testing.T) {
	if got := mapCS2ArchivePath("CounterStrikeSharp-v1/game/csgo/addons/counterstrikesharp/plugins/A/A.dll", "game/csgo", "game/csgo/addons", "A"); got != "game/csgo/addons/counterstrikesharp/plugins/A/A.dll" {
		t.Fatalf("unexpected game/csgo prefix mapping: %q", got)
	}
	if got := mapCS2ArchivePath("RockTheVote.dll", "game/csgo", "game/csgo/addons", "cs2-rockthevote"); got != "game/csgo/addons/counterstrikesharp/plugins/cs2-rockthevote/RockTheVote.dll" {
		t.Fatalf("unexpected bare plugin mapping: %q", got)
	}
}

func injectTestPkg(t *testing.T, inj *Injector, ws *driver.WorkspaceContext, pkg, hex, format string) {
	t.Helper()
	err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "cs2-plugins", Package: pkg, Version: "1.0.0",
		InstallMode: corev1.AddonInstallModeCS2Plugin, TargetDir: "game/csgo/addons",
	}, &driver.CachedArtifactMeta{
		Coordinate: "cs2-plugins." + pkg + ".1.0.0", Digest: "sha256:" + hex, HexDigest: hex, ArchiveFormat: format,
	})
	if err != nil {
		t.Fatalf("Inject %s: %v", pkg, err)
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), substr) {
		t.Fatalf("expected %s to contain %q, err=%v content=%q", path, substr, err, string(raw))
	}
}
