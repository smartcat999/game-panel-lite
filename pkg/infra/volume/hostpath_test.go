package volume

import (
	"bytes"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestArchivePathsToTarGzConcurrentFileGrowth(t *testing.T) {
	srcDir := t.TempDir()
	logPath := filepath.Join(srcDir, "server.log")
	initial := bytes.Repeat([]byte("line\n"), 8192)
	if err := os.WriteFile(logPath, initial, 0o644); err != nil {
		t.Fatalf("write initial log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "world.wld"), []byte("world-data"), 0o644); err != nil {
		t.Fatalf("write world: %v", err)
	}

	root, err := os.OpenRoot(srcDir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		f, openErr := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr != nil {
			return
		}
		defer func() { _ = f.Close() }()
		chunk := bytes.Repeat([]byte("append-row-during-hot-backup\n"), 32)
		for !stop.Load() {
			_, _ = f.Write(chunk)
			time.Sleep(100 * time.Microsecond)
		}
	}()

	var archive bytes.Buffer
	written, sum, err := ArchivePathsToTarGzWithProgress(root, nil, &archive, 64<<20, nil)
	stop.Store(true)
	<-done
	if err != nil {
		t.Fatalf("archive with concurrent growth failed: %v", err)
	}
	if written == 0 || sum == "" {
		t.Fatalf("expected non-empty archive and sha256, got written=%d sum=%q", written, sum)
	}

	dstDir := t.TempDir()
	dstRoot, err := os.OpenRoot(dstDir)
	if err != nil {
		t.Fatalf("open dst root: %v", err)
	}
	defer func() { _ = dstRoot.Close() }()
	if err := ExtractTarGzToRoot(dstRoot, &archive, 64<<20); err != nil {
		t.Fatalf("extract archive after concurrent growth failed: %v", err)
	}
}

func TestArchivePathsToTarGzConcurrentShrinkAndVanishedFile(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a-first.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	shrinkPath := filepath.Join(srcDir, "b-shrink.log")
	if err := os.WriteFile(shrinkPath, bytes.Repeat([]byte("x"), 4096), 0o644); err != nil {
		t.Fatalf("write shrink file: %v", err)
	}
	vanishPath := filepath.Join(srcDir, "c-vanish.tmp")
	if err := os.WriteFile(vanishPath, []byte("temp"), 0o644); err != nil {
		t.Fatalf("write vanish file: %v", err)
	}

	root, err := os.OpenRoot(srcDir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	var archive bytes.Buffer
	mutated := false
	_, _, err = ArchivePathsToTarGzWithProgress(root, nil, &archive, 10<<20, func(_ int64, entries int) {
		if entries == 1 && !mutated {
			mutated = true
			_ = os.Truncate(shrinkPath, 16)
			_ = os.Remove(vanishPath)
		}
	})
	if err != nil {
		t.Fatalf("archive with shrink/vanished file failed: %v", err)
	}

	dstDir := t.TempDir()
	dstRoot, err := os.OpenRoot(dstDir)
	if err != nil {
		t.Fatalf("open dst root: %v", err)
	}
	defer func() { _ = dstRoot.Close() }()
	if err := ExtractTarGzToRoot(dstRoot, &archive, 10<<20); err != nil {
		t.Fatalf("extract padded archive failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, "b-shrink.log"))
	if err != nil || len(got) != 16 {
		t.Fatalf("expected truncated file size 16 or padded snapshot, got len=%d err=%v", len(got), err)
	}
}
