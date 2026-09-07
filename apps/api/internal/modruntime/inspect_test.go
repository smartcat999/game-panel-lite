package modruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type inspectPlugin struct{ terraria.TModLoaderProvider }

func (inspectPlugin) Key() domain.ProviderKey { return "test-inspector" }
func (inspectPlugin) InspectMod(r io.Reader) (domain.ModMetadata, error) {
	data, err := io.ReadAll(r)
	return domain.ModMetadata{Name: string(data), Version: "1", LoaderVersion: "2"}, err
}

func TestInspectUsesRegisteredProviderAndContext(t *testing.T) {
	p := inspectPlugin{terraria.NewTModLoaderProvider()}
	service := NewService(registry(t, p, palworld.NewProvider()), nil)
	path := filepath.Join(t.TempDir(), "mod")
	if err := os.WriteFile(path, []byte("custom-format"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := service.Inspect(context.Background(), p.Key(), path)
	if err != nil || got.Name != "custom-format" || got.LoaderVersion != "2" {
		t.Fatalf("metadata=%+v err=%v", got, err)
	}
	if _, err := service.Inspect(context.Background(), "unknown", path); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if _, err := service.Inspect(context.Background(), p.Key(), path+"missing"); !os.IsNotExist(err) {
		t.Fatalf("missing file err=%v", err)
	}
	got, err = service.Inspect(context.Background(), domain.ProviderPalworld, path)
	if err != nil || got != (domain.ModMetadata{}) {
		t.Fatalf("optional capability=%+v err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Inspect(ctx, p.Key(), path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}
