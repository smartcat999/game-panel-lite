package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type cancelArchiveWriter struct {
	cancel  context.CancelFunc
	written int
}

func (w *cancelArchiveWriter) Write(p []byte) (int, error) {
	w.written += len(p)
	w.cancel()
	return len(p), nil
}

func TestArchiveCopyStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data := bytes.Repeat([]byte("a"), 256<<10)
	writer := &cancelArchiveWriter{cancel: cancel}
	copied, err := io.Copy(writer, archiveContextReader{ctx: ctx, source: bytes.NewReader(data)})
	if !errors.Is(err, context.Canceled) || copied <= 0 || copied >= int64(len(data)) || int64(writer.written) != copied {
		t.Fatalf("copy continued after cancellation: %d %v", copied, err)
	}
}

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
