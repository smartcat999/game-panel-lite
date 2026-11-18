package instance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

var validTaskIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)

// InstanceTaskLogKey returns the isolated Redis/MQ key for an instance task log stream.
func InstanceTaskLogKey(instanceUID, taskID string) (string, error) {
	uid := strings.TrimSpace(instanceUID)
	tid := strings.TrimSpace(taskID)
	if uid == "" {
		return "", fmt.Errorf("%w: instance UID is required for task logs", models.ErrConflict)
	}
	if !validTaskIDPattern.MatchString(tid) || strings.Contains(tid, "..") {
		return "", fmt.Errorf("%w: invalid taskID", models.ErrBadRequest)
	}
	return "core/instances/" + uid + "/" + tid, nil
}

// FormatTaskLogLine prefixes a task log entry with local CST timestamp.
func FormatTaskLogLine(ts time.Time, msg string) string {
	loc := time.FixedZone("CST", 8*3600)
	return fmt.Sprintf("[%s] %s", ts.In(loc).Format("15:04:05"), strings.TrimSpace(msg))
}

// AppendInstanceTaskLog appends timestamped lines to the persistent instance task log.
func AppendInstanceTaskLog(ctx context.Context, tl tasklog.Interface, instanceUID, taskID string, msgs ...string) {
	if tl == nil || len(msgs) == 0 {
		return
	}
	key, err := InstanceTaskLogKey(instanceUID, taskID)
	if err != nil {
		return
	}
	now := time.Now()
	lines := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if trimmed := strings.TrimSpace(msg); trimmed != "" {
			lines = append(lines, FormatTaskLogLine(now, trimmed))
		}
	}
	if len(lines) > 0 {
		_ = tl.Append(ctx, key, lines)
	}
}

// FinishInstanceTaskLog marks the persistent instance task log stream complete.
func FinishInstanceTaskLog(ctx context.Context, tl tasklog.Interface, instanceUID, taskID string) {
	if tl == nil {
		return
	}
	key, err := InstanceTaskLogKey(instanceUID, taskID)
	if err != nil {
		return
	}
	_ = tl.Complete(ctx, key)
}

func backupTaskID(operationID string) string {
	sum := sha256.Sum256([]byte("backup:" + strings.TrimSpace(operationID)))
	return "bak-" + hex.EncodeToString(sum[:8])
}

func restoreTaskID(key string, ts time.Time) string {
	sum := sha256.Sum256([]byte("restore:" + strings.TrimSpace(key) + ":" + ts.UTC().Format(time.RFC3339Nano)))
	return "restore-" + hex.EncodeToString(sum[:8])
}

func (o *operator) TaskLogger() tasklog.Interface {
	return o.taskLogs
}

// StreamTaskLogs streams persistent + live task logs belonging to a tenant's instance.
func (o *operator) StreamTaskLogs(ctx context.Context, tenant, name, taskID string, follow bool, tailLines int) (io.ReadCloser, error) {
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	key, err := InstanceTaskLogKey(existing.Metadata.UID, taskID)
	if err != nil {
		return nil, err
	}
	if o.taskLogs == nil {
		return nil, fmt.Errorf("%w: task log service unavailable", models.ErrBadGateway)
	}
	return o.taskLogs.Stream(ctx, key, tasklog.StreamOptions{Follow: follow, TailLines: tailLines})
}
