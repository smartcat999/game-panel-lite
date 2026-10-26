package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

// Opt-in test against a real Docker daemon. The helper is a static Linux binary
// which verifies the fixture save/config/mod files and writes /data/resumed.
// Build it with CGO_ENABLED=0 go build -o /tmp/uninstall-resume ./testdata/uninstall-resume
// from this package, then set GAMEPANEL_UNINSTALL_HELPER to that binary and
// GAMEPANEL_DOCKER_UNINSTALL_INTEGRATION=1 before running this test on Linux.
func TestImageUninstallReinstallPreservesData(t *testing.T) {
	if os.Getenv("GAMEPANEL_DOCKER_UNINSTALL_INTEGRATION") != "1" {
		t.Skip("requires an isolated Docker test host and GAMEPANEL_UNINSTALL_HELPER")
	}
	helper, err := os.ReadFile(os.Getenv("GAMEPANEL_UNINSTALL_HELPER"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	adapter, err := NewAdapter(os.Getenv("DOCKER_HOST"))
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.client.Close()
	serverID := fmt.Sprintf("uninstall-test-%d", time.Now().UnixNano())
	image := "gamepanel-test/runtime-uninstall:old-" + serverID
	var currentID string
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if currentID != "" {
			_ = adapter.client.ContainerRemove(cleanup, currentID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: false})
		}
		_, _ = adapter.client.ImageRemove(cleanup, image, types.ImageRemoveOptions{Force: true})
	})
	root := t.TempDir()
	dataDir := filepath.Join(root, "instances", serverID)
	files := map[string]string{
		"world.save": "day=49\n", "serverconfig.txt": "world=existing\n",
		"Mods/kept.mod": "enabled\n", "ModConfigs/kept.json": "{\"enabled\":true}\n",
		".gamepanel-installed": "keep-existing-install\n",
	}
	for name, value := range files {
		path := filepath.Join(dataDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var rootfs bytes.Buffer
	tw := tar.NewWriter(&rootfs)
	if err := tw.WriteHeader(&tar.Header{Name: "resume", Mode: 0o755, Size: int64(len(helper))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(helper); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	importImage := func(generation string) string {
		t.Helper()
		response, err := adapter.client.ImageImport(ctx, types.ImageImportSource{Source: bytes.NewReader(rootfs.Bytes()), SourceName: "-"}, image,
			types.ImageImportOptions{Changes: []string{`ENTRYPOINT ["/resume"]`, "LABEL gamepanel.integration.generation=" + generation}})
		if err != nil {
			t.Fatal(err)
		}
		err = consumeImagePull(response)
		_ = response.Close()
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := adapter.client.ImageInspectWithRaw(ctx, image)
		if err != nil {
			t.Fatal(err)
		}
		return got.ID
	}
	oldImageID := importImage("old")
	start := func() {
		t.Helper()
		created, err := adapter.client.ContainerCreate(ctx, &container.Config{Image: image, NetworkDisabled: true},
			&container.HostConfig{Binds: []string{dataDir + ":/data"}}, nil, nil, "gamepanel-"+serverID)
		if err != nil {
			t.Fatal(err)
		}
		currentID = created.ID
		if err := adapter.StartWorkload(ctx, currentID); err != nil {
			t.Fatal(err)
		}
		wait, failures := adapter.client.ContainerWait(ctx, currentID, container.WaitConditionNotRunning)
		select {
		case result := <-wait:
			if result.StatusCode != 0 {
				t.Fatalf("fixture could not resume saved data, exit=%d", result.StatusCode)
			}
		case err := <-failures:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if data, err := os.ReadFile(filepath.Join(dataDir, "resumed")); err != nil || string(data) != files["world.save"] {
			t.Fatalf("resume failed: %q, %v", data, err)
		}
	}
	checkData := func() {
		t.Helper()
		for name, value := range files {
			data, err := os.ReadFile(filepath.Join(dataDir, name))
			if err != nil || string(data) != value {
				t.Fatalf("persistent file changed: %s, %v", name, err)
			}
		}
	}
	start()
	if err := adapter.RemoveImage(ctx, image, []runtime.ImageRemovalTarget{{ServerID: serverID, DataDir: dataDir}}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.client.ContainerInspect(ctx, currentID); !client.IsErrNotFound(err) {
		t.Fatalf("stopped container remains: %v", err)
	}
	if _, _, err := adapter.client.ImageInspectWithRaw(ctx, image); !client.IsErrNotFound(err) {
		t.Fatalf("image remains: %v", err)
	}
	checkData()
	if err := os.Remove(filepath.Join(dataDir, "resumed")); err != nil {
		t.Fatal(err)
	}
	image = "gamepanel-test/runtime-uninstall:latest-" + serverID
	if newID := importImage("latest"); newID == oldImageID {
		t.Fatal("reinstallation must use a new runtime image, not the old image")
	}
	start()
	checkData()
	t.Log("old image removed; latest image resumed original save, config, mods and data mount")
}
