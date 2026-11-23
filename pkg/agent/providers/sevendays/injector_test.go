package sevendays

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

func TestSevenDaysInjectorFlattensAndNormalizesModInfo(t *testing.T) {
	wsDir := t.TempDir()
	extDir := t.TempDir()
	// Preserved vanilla harmony folder must never be deleted or tracked.
	if err := os.MkdirAll(filepath.Join(wsDir, "Mods", "0_TFP_Harmony"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "Mods", "0_TFP_Harmony", "harmony.dll"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	nestedMod := filepath.Join(extDir, "outer-wrapper", "OcbStopFuelWaste")
	if err := os.MkdirAll(filepath.Join(nestedMod, "Config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(nestedMod, "modinfo.xml"), []byte("<xml/>"), 0o644)
	_ = os.WriteFile(filepath.Join(nestedMod, "StopFuelWaste.dll"), []byte("dll"), 0o644)
	_ = os.WriteFile(filepath.Join(nestedMod, "Config", "blocks.xml"), []byte("<configs/>"), 0o644)
	_ = os.WriteFile(filepath.Join(extDir, "InstallInstructions.txt"), []byte("ignore me"), 0o644)

	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	ws := &driver.WorkspaceContext{
		Root:             root,
		DisableReflink:   true,
		Next:             &driver.AddonsManifest{},
		BlobExtractedDir: func(string) string { return extDir },
	}
	inj := &Injector{}
	err = inj.Inject(ws, corev1.InstanceAddon{Package: "OCB7D2D--OcbStopFuelWaste", InstallMode: corev1.AddonInstallModeSevenDaysMod}, &driver.CachedArtifactMeta{
		Coordinate: "seven-days-mods.OCB7D2D--OcbStopFuelWaste@1.1.0", Digest: "sha256:aaa", HexDigest: "aaa", ArchiveFormat: registryv1.ArchiveFormatZip,
	})
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}
	if !slices.Equal(ws.Next.Dirs, []string{"Mods/OcbStopFuelWaste"}) {
		t.Fatalf("expected Mods/OcbStopFuelWaste tracked in Dirs, got %#v", ws.Next.Dirs)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "Mods", "OcbStopFuelWaste", "ModInfo.xml")); err != nil {
		t.Fatalf("expected normalized ModInfo.xml at mod root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "Mods", "0_TFP_Harmony", "harmony.dll")); err != nil {
		t.Fatalf("expected 0_TFP_Harmony preserved: %v", err)
	}
}
