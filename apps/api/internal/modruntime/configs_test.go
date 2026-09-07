package modruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type configPlugin struct {
	terraria.TModLoaderProvider
	directory string
}

func (p configPlugin) Key() domain.ProviderKey        { return "test-json-config" }
func (p configPlugin) JSONModConfigDirectory() string { return p.directory }

func TestModConfigsUsePluginDirectory(t *testing.T) {
	p := configPlugin{terraria.NewTModLoaderProvider(), "custom/settings"}
	service := NewService(registry(t, p), modStore{})
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(t.TempDir(), "instance")}}}
	ctx := context.Background()
	items, err := service.ListConfigs(ctx, server)
	if err != nil || len(items) != 0 {
		t.Fatalf("missing list: %v %v", items, err)
	}
	if _, err := os.Stat(server.Spec.Runtime.DataDir); !os.IsNotExist(err) {
		t.Fatal("read created directories")
	}
	saved, err := service.WriteConfig(ctx, server, "Example.json", []byte(`{"enabled":true}`))
	if err != nil || saved.Content != `{"enabled":true}` {
		t.Fatalf("save: %+v %v", saved, err)
	}
	if _, err := os.Stat(filepath.Join(server.Spec.Runtime.DataDir, p.directory, saved.Name)); err != nil {
		t.Fatal(err)
	}
	read, err := service.ReadConfig(ctx, server, saved.Name)
	if err != nil || read.Content != saved.Content {
		t.Fatalf("read: %+v %v", read, err)
	}
	items, err = service.ListConfigs(ctx, server)
	if err != nil || len(items) != 1 || items[0].Content != "" {
		t.Fatalf("list: %+v %v", items, err)
	}
	for _, content := range []string{"null", "[]", "invalid", strings.Repeat(" ", MaxConfigBytes+1)} {
		if _, err := service.WriteConfig(ctx, server, saved.Name, []byte(content)); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("invalid write accepted: %v", err)
		}
	}
	read, err = service.ReadConfig(ctx, server, saved.Name)
	if err != nil || read.Content != saved.Content {
		t.Fatal("rejected write changed content")
	}
	if _, err := service.DeleteConfig(ctx, server, saved.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadConfig(ctx, server, saved.Name); !os.IsNotExist(err) {
		t.Fatalf("delete: %v", err)
	}
}

func TestModConfigsRejectUnsafePathsAndSnapshots(t *testing.T) {
	for _, directory := range []string{"../outside", ".", "linked/settings", "config"} {
		t.Run(directory, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
				t.Fatal(err)
			}
			p := configPlugin{terraria.NewTModLoaderProvider(), directory}
			service := NewService(registry(t, p), modStore{})
			server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: root}}}
			if directory == "config" {
				server.Spec.ConfigVersion = 999
			}
			if _, err := service.WriteConfig(context.Background(), server, "Example.json", []byte(`{}`)); !errors.Is(err, ErrConfigConflict) {
				t.Fatalf("unsafe configuration accepted: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("escaped instance directory")
			}
		})
	}
	p := configPlugin{terraria.NewTModLoaderProvider(), "config"}
	service := NewService(registry(t, p), modStore{})
	root := t.TempDir()
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: root}}}
	for _, name := range []string{"../outside.json", "bad.txt"} {
		if _, err := service.WriteConfig(context.Background(), server, name, []byte(`{}`)); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("invalid name accepted: %v", err)
		}
	}
	for _, phase := range []domain.ServerPhase{domain.PhasePending, domain.PhaseReconciling, domain.PhaseDeleting} {
		server.Status.Phase = phase
		if _, err := service.WriteConfig(context.Background(), server, "Example.json", []byte(`{}`)); !errors.Is(err, ErrConfigConflict) {
			t.Fatalf("busy write accepted: %v", err)
		}
	}
	server.Status.Phase = domain.PhaseStopped
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.WriteConfig(ctx, server, "Example.json", []byte(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected operations created files")
	}
}

func TestModConfigsRejectLinkedAndOversizedFiles(t *testing.T) {
	p := configPlugin{terraria.NewTModLoaderProvider(), "config"}
	service := NewService(registry(t, p), modStore{})
	root := t.TempDir()
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: root}}}
	if err := os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"secret":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "config", "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "large.json"), []byte(strings.Repeat(" ", MaxConfigBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link.json", "large.json"} {
		if _, err := service.ReadConfig(context.Background(), server, name); err == nil {
			t.Fatal("unsafe read accepted")
		}
	}
	if _, err := service.WriteConfig(context.Background(), server, "link.json", []byte(`{}`)); err == nil {
		t.Fatal("linked write accepted")
	}
	if _, err := service.DeleteConfig(context.Background(), server, "link.json"); err == nil {
		t.Fatal("linked delete accepted")
	}
	items, err := service.ListConfigs(context.Background(), server)
	if err != nil || len(items) != 0 {
		t.Fatalf("unsafe files listed: %v %v", items, err)
	}
	actual, err := os.ReadFile(outside)
	if err != nil || string(actual) != `{"secret":true}` {
		t.Fatal("outside file changed")
	}
}
