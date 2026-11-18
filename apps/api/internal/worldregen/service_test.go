package worldregen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantineRestoreAndCommit(t *testing.T) {
	root := t.TempDir()
	save := filepath.Join(root, "dst", "Cluster", "Master", "save")
	if err := os.MkdirAll(save, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(save, "session"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService()
	moves, err := service.Quarantine(root, []string{"dst/Cluster/Master/save"}, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 1 {
		t.Fatalf("expected one quarantined path, got %d", len(moves))
	}
	if err := os.MkdirAll(save, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(save, "session"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Restore(moves); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(save, "session"))
	if err != nil || string(content) != "old" {
		t.Fatalf("expected restored old world, content=%q err=%v", content, err)
	}

	moves, err = service.Quarantine(root, []string{"dst/Cluster/Master/save"}, "job-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Commit(moves); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(moves[0].Quarantine); !os.IsNotExist(err) {
		t.Fatalf("expected quarantine removed, err=%v", err)
	}
}

func TestQuarantineRejectsTraversalAndSymlink(t *testing.T) {
	root := t.TempDir()
	service := NewService()
	if _, err := service.Quarantine(root, []string{"../outside"}, "job-1"); err == nil {
		t.Fatal("expected traversal to fail")
	}
	outside := t.TempDir()
	link := filepath.Join(root, "save")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := service.Quarantine(root, []string{"save"}, "job-1"); err == nil {
		t.Fatal("expected symlink to fail")
	}
}
