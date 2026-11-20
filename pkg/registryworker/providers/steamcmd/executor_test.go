//go:build unix

package steamcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func TestRunKillsProcessGroupOnContextCancel(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "child.pid")
	script := filepath.Join(tmp, "fake-steamcmd.sh")
	body := "#!/bin/sh\nsleep 30 &\nchild=$!\nprintf '%s\\n' \"$child\" > \"" + pidFile + "\"\necho ready\nwait \"$child\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &execSteamRunner{bin: script, workDir: tmp}
	start := time.Now()
	err := runner.Run(ctx, filepath.Join(tmp, "install"), nil, driver.SteamHooks{
		OnLine: func(line string) {
			if strings.TrimSpace(line) == "ready" {
				cancel()
			}
		},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected cancelled Run to return error")
	}
	if elapsed >= 3*time.Second {
		t.Fatalf("expected process group cancel to return quickly, took %v", elapsed)
	}
	assertChildProcessDead(t, pidFile)
}

func assertChildProcessDead(t *testing.T, pidFile string) {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid child pid %q: %v", string(raw), err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if killErr := syscall.Kill(pid, 0); errors.Is(killErr, syscall.ESRCH) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected child process %d to be terminated with SIGKILL", pid)
}
