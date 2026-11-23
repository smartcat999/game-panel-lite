package cs2workshop

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func TestCS2WorkshopInjectorManifestAndCleanup(t *testing.T) {
	wsDir := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	ws := &driver.WorkspaceContext{
		Root:  root,
		Next:  &driver.AddonsManifest{Digests: make(map[string]string)},
		State: make(map[string]any),
	}
	inj := &Injector{}
	if err := inj.Inject(ws, corev1.InstanceAddon{
		Repository: "cs2-workshop", Package: "3070244462", Title: "Aim Botz", Version: "1",
		InstallMode: corev1.AddonInstallModeCS2Workshop, TargetDir: "game/csgo/addons/workshop",
	}, &driver.CachedArtifactMeta{
		Coordinate: "cs2-workshop.3070244462.1", MapFolders: []string{"3070244462"},
	}); err != nil {
		t.Fatalf("Inject cs2 workshop: %v", err)
	}
	if err := inj.Finalize(ws); err != nil {
		t.Fatalf("Finalize cs2 workshop: %v", err)
	}
	if !slices.Contains(ws.Next.MapFolders, "3070244462") || !slices.Contains(ws.Next.Files, cfgRelPath) {
		t.Fatalf("expected mapFolders and cfg tracked, got maps=%v files=%v", ws.Next.MapFolders, ws.Next.Files)
	}
	raw, err := os.ReadFile(filepath.Join(wsDir, filepath.FromSlash(cfgRelPath)))
	if err != nil || !strings.Contains(string(raw), "host_workshop_map 3070244462") {
		t.Fatalf("unexpected workshop cfg content: err=%v raw=%q", err, string(raw))
	}
	inj.Cleanup(root, driver.AddonsManifest{})
	if _, err := root.Lstat(cfgRelPath); !os.IsNotExist(err) {
		t.Fatalf("expected Cleanup to remove %s, err=%v", cfgRelPath, err)
	}
}
