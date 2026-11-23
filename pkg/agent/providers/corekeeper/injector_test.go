package corekeeper

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

func TestCoreKeeperInjectorDualNativeAndBepInEx(t *testing.T) {
	wsDir := t.TempDir()
	nativeExt := t.TempDir()
	_ = os.MkdirAll(filepath.Join(nativeExt, "CoolMod"), 0o755)
	_ = os.WriteFile(filepath.Join(nativeExt, "CoolMod", "modmanifest.json"), []byte(`{"name":"CoolMod"}`), 0o644)
	_ = os.WriteFile(filepath.Join(nativeExt, "CoolMod", "CoolMod.dll"), []byte("mod"), 0o644)

	bepExt := t.TempDir()
	_ = os.WriteFile(filepath.Join(bepExt, "PlacementPlus.dll"), []byte("plugin"), 0o644)
	_ = os.WriteFile(filepath.Join(bepExt, "manifest.json"), []byte("{}"), 0o644)

	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	ws := &driver.WorkspaceContext{Root: root, DisableReflink: true, Next: &driver.AddonsManifest{}}
	inj := &Injector{}
	ws.BlobExtractedDir = func(string) string { return nativeExt }
	if err := inj.Inject(ws, corev1.InstanceAddon{Package: "CoolMod", InstallMode: corev1.AddonInstallModeCoreKeeperMod}, &driver.CachedArtifactMeta{
		Coordinate: "ck-m1", Digest: "d1", HexDigest: "h1", ArchiveFormat: registryv1.ArchiveFormatZip,
	}); err != nil {
		t.Fatalf("native inject: %v", err)
	}
	ws.BlobExtractedDir = func(string) string { return bepExt }
	if err := inj.Inject(ws, corev1.InstanceAddon{Package: "kremnev8-PlacementPlus", InstallMode: corev1.AddonInstallModeCoreKeeperMod}, &driver.CachedArtifactMeta{
		Coordinate: "ck-b1", Digest: "d2", HexDigest: "h2", ArchiveFormat: registryv1.ArchiveFormatZip,
	}); err != nil {
		t.Fatalf("bepinex inject: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "CoreKeeperServer_Data", "StreamingAssets", "Mods", "CoolMod", "ModManifest.json")); err != nil {
		t.Fatalf("expected normalized ModManifest.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "BepInEx", "plugins", "kremnev8-PlacementPlus", "PlacementPlus.dll")); err != nil {
		t.Fatalf("expected Thunderstore BepInEx plugin dll: %v", err)
	}
	if !slices.Contains(ws.Next.Dirs, "CoreKeeperServer_Data/StreamingAssets/Mods/CoolMod") ||
		!slices.Contains(ws.Next.Dirs, "BepInEx/plugins/kremnev8-PlacementPlus") {
		t.Fatalf("unexpected tracked dirs: %#v", ws.Next.Dirs)
	}
}
