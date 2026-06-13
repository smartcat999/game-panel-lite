package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBackupZip(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "instance")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, size, err := NewService(root).Create("srv", source)
	if err != nil {
		t.Fatal(err)
	}
	if size == 0 || filepath.Ext(path) != ".zip" {
		t.Fatalf("expected zip backup, path=%s size=%d", path, size)
	}
}
