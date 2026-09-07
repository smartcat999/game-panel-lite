package gameconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type configStore struct {
	calls int
	err   error
	saved domain.GameServer
}

func (s *configStore) SaveGameServer(_ context.Context, server *domain.GameServer) error {
	s.calls++
	s.saved = *server
	return s.err
}
func testService(t *testing.T, store Store) *Service {
	t.Helper()
	registry, err := provider.NewRegistry(terraria.NewTModLoaderProvider(), terraria.NewVanillaProvider(), palworld.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(registry, store, domain.ProviderTerrariaVanilla)
}
func TestPresetsPreserveLegacyContract(t *testing.T) {
	presets := testService(t, &configStore{}).Presets()
	got, err := json.Marshal(presets)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(terraria.Presets)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("preset contract changed: got %s want %s", got, expected)
	}
}
func TestPreviewUsesCapabilityAndDefault(t *testing.T) {
	service := testService(t, &configStore{})
	raw, err := json.Marshal(terraria.Presets[0].Config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Preview("", 0, raw)
	if err != nil || !strings.Contains(result["serverconfig"], "worldname=Friends World") {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if _, err := service.Preview(domain.ProviderPalworld, 0, raw); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("expected unsupported: %v", err)
	}
	if _, err := service.Preview("missing", 0, raw); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if _, err := service.Preview("", 0, json.RawMessage(`{"maxPlayers":"invalid"}`)); err == nil {
		t.Fatal("invalid config accepted")
	}
}
func configServer(t *testing.T) domain.GameServer {
	t.Helper()
	return domain.GameServer{ID: "restore", ProviderKey: domain.ProviderTerrariaVanilla, Spec: domain.ServerSpec{
		Generation: 3, Config: terraria.PayloadFromConfig(terraria.Presets[0].Config),
		Runtime: domain.ServerRuntimeSpec{DataDir: t.TempDir()},
	}}
}
func TestRestorePersistsConfigAndGeneration(t *testing.T) {
	store := &configStore{}
	server := configServer(t)
	if err := os.WriteFile(filepath.Join(server.Spec.Runtime.DataDir, "serverconfig.txt"), []byte("worldname=Restored\nmaxplayers=14\nport=12345\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := testService(t, store).Restore(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || server.Spec.Generation != 4 || server.Spec.Network.Port != 7777 || server.Spec.Config["worldName"] != "Restored" || server.Status.Phase != domain.PhasePending {
		t.Fatalf("unexpected restored server: %+v", server)
	}
}
func TestRestoreFailureDoesNotMutateCaller(t *testing.T) {
	store := &configStore{err: errors.New("database unavailable")}
	server := configServer(t)
	before, _ := json.Marshal(server)
	if err := os.WriteFile(filepath.Join(server.Spec.Runtime.DataDir, "serverconfig.txt"), []byte("worldname=Restored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := testService(t, store).Restore(context.Background(), &server); !errors.Is(err, store.err) {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _ := json.Marshal(server)
	if string(before) != string(after) {
		t.Fatal("caller mutated on failed persistence")
	}
}
func TestRestoreRejectsEscapingSymlinkAndOversizedFile(t *testing.T) {
	for _, mode := range []string{"symlink", "oversized", "missing"} {
		t.Run(mode, func(t *testing.T) {
			store := &configStore{}
			server := configServer(t)
			path := filepath.Join(server.Spec.Runtime.DataDir, "serverconfig.txt")
			switch mode {
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("worldname=Outside\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(path, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := testService(t, store).Restore(context.Background(), &server)
			if (err == nil) != (mode == "missing") {
				t.Fatalf("unexpected error: %v", err)
			}
			if store.calls != 0 {
				t.Fatal("invalid or missing input persisted")
			}
		})
	}
}

func TestIncompatibleConfigCannotBePreviewedOrRestored(t *testing.T) {
	store := &configStore{}
	service := testService(t, store)
	server := configServer(t)
	server.Spec.ConfigVersion = 99
	before, _ := json.Marshal(server)
	if err := service.Restore(context.Background(), &server); err == nil {
		t.Fatal("incompatible restore accepted")
	}
	after, _ := json.Marshal(server)
	if string(before) != string(after) || store.calls != 0 {
		t.Fatal("incompatible restore mutated resource")
	}
	if _, err := service.Preview(server.ProviderKey, 99, json.RawMessage(`{}`)); err == nil {
		t.Fatal("incompatible preview accepted")
	}
}

func TestBackupCompatibilityUsesSourceVersion(t *testing.T) {
	service := testService(t, &configStore{})
	server := configServer(t)
	for _, tc := range []struct {
		source domain.Backup
		wantOK bool
	}{
		{domain.Backup{}, true},
		{domain.Backup{ProviderKey: server.ProviderKey, ConfigVersion: 1}, true},
		{domain.Backup{ProviderKey: server.ProviderKey, ConfigVersion: 2}, false},
		{domain.Backup{ProviderKey: domain.ProviderPalworld, ConfigVersion: 1}, false},
		{domain.Backup{ConfigVersion: -1}, false},
	} {
		if err := service.CheckBackup(server, tc.source); (err == nil) != tc.wantOK {
			t.Fatalf("source=%+v err=%v", tc.source, err)
		}
	}
}

func TestRestorePersistenceFailureRollsBackPublishedFiles(t *testing.T) {
	store := &configStore{err: errors.New("database write rejected")}
	service := testService(t, store)
	server := configServer(t)
	before, _ := json.Marshal(server)
	target := server.Spec.Runtime.DataDir
	if err := os.WriteFile(filepath.Join(target, "serverconfig.txt"), []byte("worldname=Original\n"), 0640); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("worldname=Restored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new-world"), []byte("restored-world"), 0600); err != nil {
		t.Fatal(err)
	}
	backups := backupsvc.NewService(t.TempDir())
	path, _, err := backups.Create(server.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	err = backups.RestoreChecked(server.ID, filepath.Base(path), target, backupsvc.RestoreHooks{Commit: func() error { return service.Restore(context.Background(), &server) }})
	if !errors.Is(err, store.err) || !errors.Is(err, backupsvc.ErrCommit) {
		t.Fatalf("commit error=%v", err)
	}
	if store.calls != 1 {
		t.Fatalf("persistence calls=%d", store.calls)
	}
	after, _ := json.Marshal(server)
	if string(before) != string(after) {
		t.Fatal("resource changed after failed commit")
	}
	contents, err := os.ReadFile(filepath.Join(target, "serverconfig.txt"))
	if err != nil || string(contents) != "worldname=Original\n" {
		t.Fatalf("config file not restored: %q %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(target, "new-world")); !os.IsNotExist(err) {
		t.Fatalf("new world remains: %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging remains: %v %v", entries, err)
	}
}
