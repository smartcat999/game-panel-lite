package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type recoveryImageRuntime struct {
	present                           bool
	loaded, pulled                    string
	loadFails, pullFails, verifyFails bool
}

func (r *recoveryImageRuntime) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	state := runtime.ImageStatusMissing
	if r.present {
		state = runtime.ImageStatusReady
	}
	return domain.RuntimeImageStatus{Image: image, Status: state}
}
func (r *recoveryImageRuntime) LoadImageArchive(ctx context.Context, path string) error {
	r.loaded = path
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r.loadFails {
		return fmt.Errorf("corrupt offline archive")
	}
	r.present = true
	return nil
}
func (r *recoveryImageRuntime) PrepareImage(_ context.Context, image string) error {
	r.pulled = image
	if r.pullFails {
		return fmt.Errorf("registry unavailable")
	}
	r.present = !r.verifyFails
	return nil
}
func TestImageRecoveryRetainsDataAndRequiresRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, storage, archivePath                                                     string
		register, archive, cached, loadFails, pullFails, verifyFails, shared, canceled bool
		wantLoad, wantPull, wantError                                                  bool
	}{
		{name: "cached image", cached: true},
		{name: "legacy offline", register: true, archive: true, wantLoad: true},
		{name: "legacy missing archive", register: true, wantPull: true},
		{name: "new registration", register: true, storage: "docker", wantPull: true},
		{name: "latest shared runtime registration", register: true, storage: "docker", shared: true, wantPull: true},
		{name: "canceled recovery", register: true, storage: "docker", canceled: true, wantError: true},
		{name: "explicitly uninstalled", wantError: true},
		{name: "malicious archive path", register: true, archivePath: "../../outside.tar", wantError: true},
		{name: "unknown storage", register: true, storage: "unknown", wantError: true},
		{name: "corrupt archive falls back to registry", register: true, archive: true, loadFails: true, wantLoad: true, wantPull: true},
		{name: "registry failure keeps original data", register: true, storage: "docker", pullFails: true, wantPull: true, wantError: true},
		{name: "pull must be verified", register: true, storage: "docker", verifyFails: true, wantPull: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			image := "example/minecraft:latest"
			version := "1.21.4"
			world := filepath.Join(root, "instances", "existing", "world.save")
			if err := os.MkdirAll(filepath.Dir(world), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(world, []byte("existing day 49"), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.register {
				m := runtimeInstallMarker{ProviderKey: domain.ProviderMinecraft, Version: version, Image: image, Storage: tc.storage, ArchivePath: tc.archivePath}
				data, _ := json.Marshal(m)
				p := filepath.Join(root, "runtime-installs", "minecraft", version+".json")
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.archive {
				p := filepath.Join(root, "runtime-images", "minecraft", version+".tar")
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("offline fixture"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			r := &recoveryImageRuntime{present: tc.cached, loadFails: tc.loadFails, pullFails: tc.pullFails, verifyFails: tc.verifyFails}
			gameVersion := version
			if tc.shared {
				gameVersion = "1.20.6"
			}
			server := domain.GameServer{ProviderKey: domain.ProviderMinecraft, Spec: domain.ServerSpec{Version: gameVersion}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			err := NewRuntimeImageLoader(root, r).EnsureImage(ctx, server, image)
			if (err != nil) != tc.wantError || (r.loaded != "") != tc.wantLoad || (r.pulled != "") != tc.wantPull {
				t.Fatalf("recovery err=%v loaded=%s pulled=%s", err, r.loaded, r.pulled)
			}
			data, err := os.ReadFile(world)
			if err != nil || string(data) != "existing day 49" {
				t.Fatal("original data changed")
			}
		})
	}
}
