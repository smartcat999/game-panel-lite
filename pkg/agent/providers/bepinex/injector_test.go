package bepinex

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

func writeTempTreeFile(t *testing.T, rootDir, rel, content string) {
	t.Helper()
	p := filepath.Join(rootDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func newTestWS(t *testing.T, workDir, extractedDir string) (*os.Root, *driver.WorkspaceContext) {
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
	next := &driver.AddonsManifest{Digests: make(map[string]string)}
	ws := &driver.WorkspaceContext{
		Root:             root,
		DisableReflink:   true,
		Next:             next,
		BlobExtractedDir: func(hex string) string { return filepath.Join(extractedDir, hex) },
	}
	return root, ws
}

func TestBepInExInjectorFrameworkPackPreservesUserConfig(t *testing.T) {
	tmp := t.TempDir()
	fwHex := "aaaa"
	fwDir := filepath.Join(tmp, fwHex)
	writeTempTreeFile(t, fwDir, ".gp-extracted-ok", "ok\n")
	writeTempTreeFile(t, fwDir, "README.md", "# BepInExPack")
	writeTempTreeFile(t, fwDir, "BepInExPack_Valheim/BepInEx/core/BepInEx.Preloader.dll", "preloader-bin")
	writeTempTreeFile(t, fwDir, "BepInExPack_Valheim/BepInEx/config/BepInEx.cfg", "default-cfg")
	writeTempTreeFile(t, fwDir, "BepInExPack_Valheim/doorstop_libs/libdoorstop_x64.so", "doorstop-so")

	root, ws := newTestWS(t, tmp, tmp)
	wsDir := filepath.Join(tmp, "ws")
	writeTempTreeFile(t, wsDir, "BepInEx/config/BepInEx.cfg", "custom-user-cfg")

	inj := &Injector{}
	err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "valheim-thunderstore", Package: "denikson-BepInExPack_Valheim", Version: "5.4.2360",
		InstallMode: corev1.AddonInstallModeBepInEx, TargetDir: "BepInEx",
	}, &driver.CachedArtifactMeta{
		Coordinate: "valheim-thunderstore.denikson-BepInExPack_Valheim.5.4.2360",
		Digest:     "sha256:" + fwHex, HexDigest: fwHex, ArchiveFormat: registryv1.ArchiveFormatZip,
	})
	if err != nil {
		t.Fatalf("Inject framework failed: %v", err)
	}
	if _, err := root.Lstat("BepInEx/core/BepInEx.Preloader.dll"); err != nil {
		t.Fatalf("expected preloader materialized at workspace root: %v", err)
	}
	if _, err := root.Lstat("doorstop_libs/libdoorstop_x64.so"); err != nil {
		t.Fatalf("expected doorstop_libs at workspace root: %v", err)
	}
	cfgBytes, err := os.ReadFile(filepath.Join(wsDir, "BepInEx/config/BepInEx.cfg"))
	if err != nil || string(cfgBytes) != "custom-user-cfg" {
		t.Fatalf("expected existing user BepInEx.cfg preserved, got %q err=%v", string(cfgBytes), err)
	}
	if slices.Contains(ws.Next.Files, "BepInEx/config/BepInEx.cfg") {
		t.Fatalf("framework config must not be tracked in ws.Next.Files: %v", ws.Next.Files)
	}
	if !slices.Contains(ws.Next.Files, "BepInEx/core/BepInEx.Preloader.dll") {
		t.Fatalf("expected preloader tracked in ws.Next.Files for clean uninstall, got %v", ws.Next.Files)
	}
}

func TestBepInExInjectorModLayoutsAndMetadataFiltering(t *testing.T) {
	tmp := t.TempDir()
	writeTempTreeFile(t, filepath.Join(tmp, "jot"), "plugins/Jotunn.dll", "jotunn-dll")
	writeTempTreeFile(t, filepath.Join(tmp, "jot"), "manifest.json", `{}`)
	writeTempTreeFile(t, filepath.Join(tmp, "jot"), "icon.png", "png")
	writeTempTreeFile(t, filepath.Join(tmp, "plant"), "Advize_PlantEverything.dll", "plant-dll")
	writeTempTreeFile(t, filepath.Join(tmp, "plant"), "config/advize.PlantEverything.cfg", "plant-cfg")
	writeTempTreeFile(t, filepath.Join(tmp, "patch"), "BepInEx/patchers/FastPatch.dll", "patch-dll")

	root, ws := newTestWS(t, tmp, tmp)
	inj := &Injector{}
	cases := []struct{ pkg, hex string }{
		{"ValheimModding-Jotunn", "jot"},
		{"Advize-PlantEverything", "plant"},
		{"SomeAuthor-Patch", "patch"},
	}
	for _, tc := range cases {
		err := inj.Inject(ws, corev1.InstanceAddon{
			Repository: "valheim-thunderstore", Package: tc.pkg, Version: "1.0.0",
			InstallMode: corev1.AddonInstallModeBepInEx, TargetDir: "BepInEx",
		}, &driver.CachedArtifactMeta{
			Coordinate: "valheim-thunderstore." + tc.pkg + ".1.0.0",
			Digest:     "sha256:" + tc.hex, HexDigest: tc.hex, ArchiveFormat: registryv1.ArchiveFormatZip,
		})
		if err != nil {
			t.Fatalf("Inject %s failed: %v", tc.pkg, err)
		}
	}
	for _, p := range []string{
		"BepInEx/plugins/ValheimModding-Jotunn/Jotunn.dll",
		"BepInEx/plugins/Advize-PlantEverything/Advize_PlantEverything.dll",
		"BepInEx/config/advize.PlantEverything.cfg",
		"BepInEx/patchers/SomeAuthor-Patch/FastPatch.dll",
	} {
		if _, err := root.Lstat(filepath.FromSlash(p)); err != nil {
			t.Fatalf("expected materialized mod path %s: %v", p, err)
		}
	}
	if _, err := root.Lstat("BepInEx/plugins/ValheimModding-Jotunn/manifest.json"); !os.IsNotExist(err) {
		t.Fatalf("expected root Thunderstore metadata manifest.json skipped, got err=%v", err)
	}
	if !slices.Contains(ws.Next.Dirs, "BepInEx/plugins/ValheimModding-Jotunn") || !slices.Contains(ws.Next.Dirs, "BepInEx/patchers/SomeAuthor-Patch") {
		t.Fatalf("expected per-mod tracked dirs, got %v", ws.Next.Dirs)
	}
}
