package mod

import (
	"strings"
	"testing"
)

func TestUploadValidatesModFiles(t *testing.T) {
	service := NewService(t.TempDir())
	for _, name := range []string{"cool.tmod", "install.txt", "enabled.json"} {
		if _, _, err := service.Upload("srv", name, strings.NewReader("x")); err != nil {
			t.Fatalf("expected %s to upload: %v", name, err)
		}
	}
	for _, name := range []string{"../cool.tmod", "notes.txt", "enabled.txt"} {
		if _, _, err := service.Upload("srv", name, strings.NewReader("x")); err == nil {
			t.Fatalf("expected %s to fail", name)
		}
	}
}
