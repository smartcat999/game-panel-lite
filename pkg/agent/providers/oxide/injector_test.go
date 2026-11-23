package oxide

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
	next := &driver.AddonsManifest{Digests: make(map[string]string)}
	ws := &driver.WorkspaceContext{
		Root:             root,
		DisableReflink:   true,
		Next:             next,
		BlobFilePath:     func(hex string) string { return filepath.Join(workDir, "blobs", hex) },
		BlobExtractedDir: func(hex string) string { return filepath.Join(workDir, "extracted", hex) },
	}
	return root, ws
}

func TestOxideInjectorFrameworkAndSingleCSPlugin(t *testing.T) {
	tmp := t.TempDir()
	fwHex := "aaaa"
	fwDir := filepath.Join(tmp, "extracted", fwHex)
	writeTempTreeFile(t, fwDir, ".gp-extracted-ok", "ok\n")
	writeTempTreeFile(t, fwDir, "RustDedicated_Data/Managed/Oxide.Core.dll", "oxide-core")
	writeTempTreeFile(t, fwDir, "RustDedicated_Data/Managed/Assembly-CSharp.dll", "oxide-patched-csharp")
	writeTempTreeFile(t, fwDir, "RustDedicated_Data/Managed/x64/libsqlite3.so", "sqlite-so")

	csHex := "bbbb"
	writeTempTreeFile(t, tmp, "blobs/"+csHex, "// GatherManager plugin")

	root, ws := newTestWS(t, tmp)
	inj := &Injector{}

	if err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "rust-umod", Package: "oxide-rust", Version: "2.0.7912",
		InstallMode: corev1.AddonInstallModeOxide, TargetDir: "oxide",
	}, &driver.CachedArtifactMeta{
		Coordinate: "rust-umod.oxide-rust.2.0.7912",
		Digest:     "sha256:" + fwHex, HexDigest: fwHex, ArchiveFormat: registryv1.ArchiveFormatZip,
	}); err != nil {
		t.Fatalf("Inject Oxide framework: %v", err)
	}

	if err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "rust-umod", Package: "gather-manager", Version: "2.2.78",
		InstallMode: corev1.AddonInstallModeOxide, TargetDir: "oxide",
	}, &driver.CachedArtifactMeta{
		Coordinate: "rust-umod.gather-manager.2.2.78",
		Digest:     "sha256:" + csHex, HexDigest: csHex, ArchiveFormat: registryv1.ArchiveFormatRaw, FileName: "GatherManager.cs",
	}); err != nil {
		t.Fatalf("Inject GatherManager.cs: %v", err)
	}

	for _, rel := range []string{
		"RustDedicated_Data/Managed/Oxide.Core.dll",
		"RustDedicated_Data/Managed/Assembly-CSharp.dll",
		"RustDedicated_Data/Managed/x64/libsqlite3.so",
		"oxide/plugins/GatherManager.cs",
	} {
		if _, err := root.Lstat(filepath.FromSlash(rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
		if !slices.Contains(ws.Next.Files, rel) {
			t.Fatalf("expected %s tracked in ws.Next.Files: %v", rel, ws.Next.Files)
		}
	}
}
