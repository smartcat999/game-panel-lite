package instance

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
)

func TestInstanceTaskLogKeyValidation(t *testing.T) {
	if _, err := InstanceTaskLogKey("", "bak-1"); err == nil {
		t.Fatalf("expected error for empty instance UID")
	}
	for _, badID := range []string{"", "../secret", "a/b", "bad space"} {
		if _, err := InstanceTaskLogKey("uid-123", badID); err == nil {
			t.Fatalf("expected validation error for taskID %q", badID)
		}
	}
	key, err := InstanceTaskLogKey("uid-123", "sched-abc:123")
	if err != nil || key != "core/instances/uid-123/sched-abc:123" {
		t.Fatalf("unexpected task log key %q err=%v", key, err)
	}
}

func TestAppendAndFinishInstanceTaskLog(t *testing.T) {
	tl := tasklog.New(nil, nil)
	ctx := context.Background()
	AppendInstanceTaskLog(ctx, tl, "uid-1", "bak-01", "开始备份", "  ", "正在上传")
	FinishInstanceTaskLog(ctx, tl, "uid-1", "bak-01")

	key, err := InstanceTaskLogKey("uid-1", "bak-01")
	if err != nil {
		t.Fatalf("InstanceTaskLogKey error: %v", err)
	}
	rc, err := tl.Stream(ctx, key, tasklog.StreamOptions{Follow: false, TailLines: 50})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read log stream error: %v", err)
	}
	out := string(raw)
	if !strings.Contains(out, "开始备份") || !strings.Contains(out, "正在上传") {
		t.Fatalf("expected appended task logs in %q", out)
	}
}
