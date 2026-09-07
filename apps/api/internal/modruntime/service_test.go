package modruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type modStore struct{ mods []domain.ModFile }

func (s modStore) ListMods(context.Context, string) ([]domain.ModFile, error) { return s.mods, nil }
func registry(t *testing.T, items ...provider.GameProvider) *provider.Registry {
	t.Helper()
	result, err := provider.NewRegistry(items...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestSyncBuildsProviderManifestAndIsRepeatable(t *testing.T) {
	p := terraria.NewTModLoaderProvider()
	service := NewService(registry(t, p), modStore{mods: []domain.ModFile{
		{Enabled: true, FileName: "Example.tmod", ModName: "Example"},
		{Enabled: false, FileName: "Disabled.tmod", ModName: "Disabled"},
		{Enabled: true, Source: "workshop", WorkshopID: "123", ModName: "WorkshopExample"},
	}})
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: t.TempDir()}}}
	for i := 0; i < 2; i++ {
		if err := service.Sync(context.Background(), server); err != nil {
			t.Fatal(err)
		}
		enabled, err := os.ReadFile(filepath.Join(server.Spec.Runtime.DataDir, "Mods", "enabled.json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(enabled) != "[\n  \"Example\",\n  \"WorkshopExample\"\n]\n" {
			t.Fatalf("unexpected manifest: %s", enabled)
		}
		install, err := os.ReadFile(filepath.Join(server.Spec.Runtime.DataDir, "Mods", "install.txt"))
		if err != nil || string(install) != "123\n" {
			t.Fatalf("install=%q err=%v", install, err)
		}
	}
}

type manifestPlugin struct {
	terraria.TModLoaderProvider
	files map[string]string
}

func (p manifestPlugin) ModManifest([]domain.ModFile) (map[string]string, error) { return p.files, nil }
func TestSyncRejectsEscapingFiles(t *testing.T) {
	for _, name := range []string{"../outside", "linked/outside"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
				t.Fatal(err)
			}
			p := manifestPlugin{TModLoaderProvider: terraria.NewTModLoaderProvider(), files: map[string]string{name: "bad"}}
			service := NewService(registry(t, p), modStore{})
			server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: dir}}}
			if err := service.Sync(context.Background(), server); err == nil {
				t.Fatal("escaping manifest accepted")
			}
			if _, err := os.Stat(filepath.Join(outside, "outside")); !os.IsNotExist(err) {
				t.Fatalf("outside file touched: %v", err)
			}
		})
	}
}
func TestSyncCancellationAndUnsupportedCapability(t *testing.T) {
	p := terraria.NewTModLoaderProvider()
	dir := t.TempDir()
	service := NewService(registry(t, p, palworld.NewProvider()), modStore{})
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: dir}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Sync(ctx, server); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	server.ProviderKey = domain.ProviderPalworld
	if err := service.Sync(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("unexpected files=%v err=%v", files, err)
	}
	if len(service.Support(domain.ProviderPalworld).UploadExtensions) != 1 || service.Support(domain.ProviderPalworld).Workshop {
		t.Fatal("incorrect upload capability")
	}
}
