package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCancelledArchiveDoesNotCreateOutput(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	svc := NewService(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, create := range []func() (string, int64, error){
		func() (string, int64, error) { return svc.CreateContext(ctx, "server", source) },
		func() (string, int64, error) { return svc.CreateSubtreeContext(ctx, "server", source, "worlds") },
	} {
		path, size, err := create()
		if path != "" || size != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled archive: %q %d %v", path, size, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled task created output")
	}
}

func TestArchiveSourceFailureRemovesPartialOutput(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a-world"), []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "private"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private"), filepath.Join(source, "z-link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	path, size, err := NewService(root).CreateContext(context.Background(), "server", source)
	if err == nil || path != "" || size != 0 {
		t.Fatal("symlink source accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "backups", "server"))
	if err != nil || len(entries) != 0 {
		t.Fatal("failed archive left partial output")
	}
}

func TestSubtreeRejectsInternalSymlinkRoot(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "worlds"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("worlds", filepath.Join(source, "alias")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, _, err := NewService(t.TempDir()).CreateSubtreeContext(context.Background(), "server", source, "alias"); err == nil {
		t.Fatal("symlink subtree root accepted")
	}
}
