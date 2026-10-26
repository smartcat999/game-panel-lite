package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type latestArchiveRuntime struct{ loaded string }

func (r *latestArchiveRuntime) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	status := runtime.ImageStatusMissing
	if r.loaded != "" {
		status = runtime.ImageStatusReady
	}
	return domain.RuntimeImageStatus{Image: image, Status: status}
}
func (r *latestArchiveRuntime) LoadImageArchive(_ context.Context, path string) error {
	r.loaded = path
	return nil
}

func TestImageLoaderFindsLatestSharedRuntimeForExistingGameVersion(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "runtime-images", "minecraft", "1.21.4.tar")
	marker := filepath.Join(root, "runtime-installs", "minecraft", "1.21.4.json")
	for path, data := range map[string]string{archive: "latest runtime", marker: `{"providerKey":"minecraft","version":"1.21.4","image":"example/minecraft:latest","archivePath":"runtime-images/minecraft/1.21.4.tar"}`} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &latestArchiveRuntime{}
	server := domain.GameServer{ProviderKey: domain.ProviderMinecraft, Spec: domain.ServerSpec{Version: "1.20.6"}}
	if err := NewRuntimeImageLoader(root, r).EnsureImage(context.Background(), server, "example/minecraft:latest"); err != nil {
		t.Fatal(err)
	}
	if r.loaded != archive {
		t.Fatalf("must load latest runtime archive, got %s", r.loaded)
	}
}
