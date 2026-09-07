package modruntime

import (
	"os"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type uploadPlugin struct{ terraria.TModLoaderProvider }

func (uploadPlugin) Key() domain.ProviderKey { return "test-upload-plugin" }
func (uploadPlugin) ModSupport() domain.ModSupport {
	return domain.ModSupport{UploadExtensions: []string{".addon"}, CacheFiles: []string{"index.json"}}
}

func TestProviderUploadPolicyFlowsIntoCache(t *testing.T) {
	p := uploadPlugin{terraria.NewTModLoaderProvider()}
	service := NewService(registry(t, p), nil)
	cache := modsvc.NewService(t.TempDir(), service.StoredFileName)
	name, err := service.UploadFileName(p.Key(), "example.ADDON")
	if err != nil {
		t.Fatal(err)
	}
	path, size, err := cache.Upload("instance", p.Key(), name, strings.NewReader("payload"))
	if err != nil || size != 7 {
		t.Fatalf("upload size=%d err=%v", size, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "payload" {
		t.Fatalf("cached=%q err=%v", contents, err)
	}
	if _, err := service.UploadFileName(p.Key(), "index.json"); err == nil {
		t.Fatal("auxiliary file accepted as user upload")
	}
	if _, _, err := cache.Upload("instance", p.Key(), "index.json", strings.NewReader("{}")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []domain.ProviderKey{p.Key(), "unknown"} {
		for _, name := range []string{"../example.addon", "example.tmod", "enabled.json"} {
			if _, _, err := cache.Upload("instance", key, name, strings.NewReader("bad")); err == nil {
				t.Fatalf("accepted %s %s", key, name)
			}
		}
	}
	if _, err := service.UploadFileName("unknown", "example.addon"); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
