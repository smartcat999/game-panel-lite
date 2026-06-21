package mod

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestUploadValidatesModFiles(t *testing.T) {
	service := NewService(t.TempDir())
	for _, name := range []string{"cool.tmod", "install.txt", "enabled.json"} {
		if _, _, err := service.Upload("srv", domain.ProviderTerrariaTModLoader, name, strings.NewReader("x")); err != nil {
			t.Fatalf("expected %s to upload: %v", name, err)
		}
	}
	for _, name := range []string{"../cool.tmod", "notes.txt", "enabled.txt"} {
		if _, _, err := service.Upload("srv", domain.ProviderTerrariaTModLoader, name, strings.NewReader("x")); err == nil {
			t.Fatalf("expected %s to fail", name)
		}
	}
}

func TestUploadValidatesPalworldPakFiles(t *testing.T) {
	service := NewService(t.TempDir())
	if _, _, err := service.Upload("srv", domain.ProviderPalworld, "better-pals.pak", strings.NewReader("x")); err != nil {
		t.Fatalf("expected pak to upload: %v", err)
	}
	for _, name := range []string{"../better-pals.pak", "notes.txt", "mod.tmod"} {
		if _, _, err := service.Upload("srv", domain.ProviderPalworld, name, strings.NewReader("x")); err == nil {
			t.Fatalf("expected %s to fail", name)
		}
	}
}
