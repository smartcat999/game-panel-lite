package sml

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

func writeTempFile(t *testing.T, rootDir, rel, content string) {
	t.Helper()
	p := filepath.Join(rootDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func newTestWS(t *testing.T, workDir string) (*os.Root, *driver.WorkspaceContext) {
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
		BlobFilePath:     func(hex string) string { return filepath.Join(workDir, "blobs", hex) },
		BlobExtractedDir: func(hex string) string { return filepath.Join(workDir, "extracted", hex) },
	}
	return root, ws
}

func TestSMLInjectorRootAndNestedUPlugin(t *testing.T) {
	tmp := t.TempDir()
	writeTempFile(t, filepath.Join(tmp, "extracted", "smlhex"), "SML.uplugin", `{"FriendlyName":"SML"}`)
	writeTempFile(t, filepath.Join(tmp, "extracted", "smlhex"), "Binaries/Linux/libFactoryServer-SML-Linux-Shipping.so", "so")
	writeTempFile(t, filepath.Join(tmp, "extracted", "modhex"), "WrappedDir/Additional_300_Inventory_Slots.uplugin", `{}`)
	writeTempFile(t, filepath.Join(tmp, "extracted", "modhex"), "WrappedDir/Content/Paks/LinuxServer/mod.pak", "pak")

	root, ws := newTestWS(t, tmp)
	inj := &Injector{}
	if err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "satisfactory-ficsit", Package: "SML", Version: "3.12.1",
		InstallMode: corev1.AddonInstallModeSML, TargetDir: "FactoryGame/Mods",
	}, &driver.CachedArtifactMeta{
		Coordinate: "satisfactory-ficsit.SML.3.12.1", Digest: "sha256:smlhex", HexDigest: "smlhex", ArchiveFormat: registryv1.ArchiveFormatZip,
	}); err != nil {
		t.Fatalf("Inject SML: %v", err)
	}
	if err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "satisfactory-ficsit", Package: "Additional_300_Inventory_Slots", Version: "6.2.9",
		InstallMode: corev1.AddonInstallModeSML, TargetDir: "FactoryGame/Mods",
	}, &driver.CachedArtifactMeta{
		Coordinate: "satisfactory-ficsit.Additional_300_Inventory_Slots.6.2.9", Digest: "sha256:modhex", HexDigest: "modhex", ArchiveFormat: registryv1.ArchiveFormatZip,
	}); err != nil {
		t.Fatalf("Inject nested mod: %v", err)
	}
	for _, rel := range []string{
		"FactoryGame/Mods/SML/SML.uplugin",
		"FactoryGame/Mods/SML/Binaries/Linux/libFactoryServer-SML-Linux-Shipping.so",
		"FactoryGame/Mods/Additional_300_Inventory_Slots/Additional_300_Inventory_Slots.uplugin",
		"FactoryGame/Mods/Additional_300_Inventory_Slots/Content/Paks/LinuxServer/mod.pak",
	} {
		if _, err := root.Lstat(filepath.FromSlash(rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	for _, wantDir := range []string{"FactoryGame/Mods/SML", "FactoryGame/Mods/Additional_300_Inventory_Slots"} {
		if !slices.Contains(ws.Next.Dirs, wantDir) {
			t.Fatalf("expected tracked dir %q in %v", wantDir, ws.Next.Dirs)
		}
	}
}
