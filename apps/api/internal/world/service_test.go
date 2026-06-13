package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportValidatesWorldExtension(t *testing.T) {
	service := NewService(t.TempDir())
	if _, _, err := service.Import("srv", "bad.txt", strings.NewReader("x")); err == nil {
		t.Fatal("expected invalid extension to fail")
	}
	if _, size, err := service.Import("srv", "good.wld", strings.NewReader("world")); err != nil || size != 5 {
		t.Fatalf("expected world import, size=%d err=%v", size, err)
	}
}

func TestDuplicateWorldCopiesWithinInstance(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	if _, _, err := service.Import("srv", "good.wld", strings.NewReader("world")); err != nil {
		t.Fatal(err)
	}
	path, size, err := service.Duplicate("srv", "good.wld", "copy.wld")
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 || filepath.Base(path) != "copy.wld" {
		t.Fatalf("expected duplicated world, path=%s size=%d", path, size)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("expected copied content, got %q", string(got))
	}
}
