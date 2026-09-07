package docker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCreationLockSerializesProcessesAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	unlock, err := lockInstanceCreation(context.Background(), root, "same-server")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlock() }()
	runHelper := func(expected string) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCreationLockProcessHelper$")
		cmd.Env = append(os.Environ(), "GAMEPANEL_LOCK_TEST_ROOT="+root, "GAMEPANEL_LOCK_TEST_EXPECT="+expected)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("lock helper: %v\n%s", err, output)
		}
	}
	runHelper("busy")
	other, err := lockInstanceCreation(context.Background(), root, "other-server")
	if err != nil {
		t.Fatal(err)
	}
	other()
	unlock()
	// The release function owns one descriptor and must only run once.
	unlock = func() {}
	runHelper("free")
	if _, err := os.Stat(filepath.Join(root, ".creation-locks", "same-server.lock")); err != nil {
		t.Fatalf("lock inode removed: %v", err)
	}
}

func TestCreationLockProcessHelper(t *testing.T) {
	root := os.Getenv("GAMEPANEL_LOCK_TEST_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	unlock, err := lockInstanceCreation(ctx, root, "same-server")
	if os.Getenv("GAMEPANEL_LOCK_TEST_EXPECT") == "busy" {
		if !errors.Is(err, context.DeadlineExceeded) {
			if err == nil {
				unlock()
			}
			t.Fatalf("expected contention timeout, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
