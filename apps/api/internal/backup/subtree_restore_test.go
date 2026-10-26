package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreSubtreeRejectsWrongArchiveWithoutChangingSave(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "instance")
	if err := os.MkdirAll(filepath.Join(data, "userdata"), 0755); err != nil {
		t.Fatal(err)
	}
	save := filepath.Join(data, "userdata", "world.dat")
	if err := os.WriteFile(save, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "game.bin"), []byte("runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(root)
	archive, _, err := svc.Create("server", data)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreSubtree("server", filepath.Base(archive), data, "userdata"); err == nil {
		t.Fatal("accepted archive outside requested subtree")
	}
	content, err := os.ReadFile(save)
	if err != nil || string(content) != "keep" {
		t.Fatalf("save changed on failed restore: %q %v", content, err)
	}
	if err := svc.RestoreSubtree("server", filepath.Base(archive), data, "../outside"); err == nil {
		t.Fatal("accepted unsafe subtree")
	}
}
