package instance

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
)

const backupInspectingMessage = "正在解析归档目录与游戏配置"

func mergeBackupInspection(old, next *corev1.InstanceBackupRecord, phase backupdb.Phase) {
	if old != nil && old.Imported {
		next.Imported = true
	}
	if old != nil && next.Snapshot == nil {
		next.Snapshot = old.Snapshot
	}
	if old != nil && next.Inspection == nil {
		next.Inspection = old.Inspection
	}
	if next.Imported && phase == backupdb.Completed && (old == nil || old.Phase != corev1.BackupPhaseRestoring) {
		if needsBackupInspectEnqueue(next.Inspection) {
			next.Inspection = &corev1.InstanceBackupInspection{Phase: corev1.BackupInspectPhasePending}
		}
	}
}

func applyCompletedBackupProjection(next *corev1.InstanceBackupRecord) {
	next.ProcessedBytes = next.SizeBytes
	next.TotalBytes = next.SizeBytes
	next.SpeedBytesPerSec = 0
	next.Progress = 100
	if next.Imported && next.Inspection != nil {
		switch next.Inspection.Phase {
		case corev1.BackupInspectPhasePending:
			next.Phase = corev1.BackupPhaseInspecting
			next.Stage = corev1.BackupStageInspecting
			next.Progress = 90
			next.Message = backupInspectingMessage
			return
		case corev1.BackupInspectPhaseFailed:
			next.Phase = corev1.BackupPhaseInspectFailed
			next.Stage = corev1.BackupStageFailed
			next.Message = nonEmptyInspectMessage(next.Inspection.Message)
			return
		}
	}
	next.Phase = corev1.BackupPhaseCompleted
	next.Stage = corev1.BackupStageCompleted
	next.Message = ""
}

func mergeBackupTelemetry(old, next *corev1.InstanceBackupRecord, phase backupdb.Phase) {
	mergeBackupInspection(old, next, phase)
	if next.TaskID == "" {
		next.TaskID = backupTaskID(next.OperationID)
	}
	if old != nil {
		next.LastRestoreTaskID = old.LastRestoreTaskID
		next.LastRestoreResult = old.LastRestoreResult
		next.LastRestoreMessage = old.LastRestoreMessage
		next.LastRestoreAt = old.LastRestoreAt
		next.StartedAt = old.StartedAt
		if old.Phase == corev1.BackupPhaseRestoring && phase == backupdb.Completed {
			next.Phase = old.Phase
			next.TaskID = old.TaskID
			next.Stage = old.Stage
			next.Progress = old.Progress
			next.ProcessedBytes = old.ProcessedBytes
			next.TotalBytes = old.TotalBytes
			next.SpeedBytesPerSec = old.SpeedBytesPerSec
			next.Message = old.Message
			next.RestoreSourcePath = old.RestoreSourcePath
			next.RestoreTargetPath = old.RestoreTargetPath
			next.RestoreReplace = old.RestoreReplace
			return
		}
		if phase != backupdb.Completed && phase != backupdb.Deleted {
			next.Stage = old.Stage
			next.Progress = old.Progress
			next.ProcessedBytes = old.ProcessedBytes
			next.TotalBytes = old.TotalBytes
			next.SpeedBytesPerSec = old.SpeedBytesPerSec
			next.Message = old.Message
		}
	}
	if phase == backupdb.PendingQuota && next.Stage == "" {
		now := time.Now().UTC()
		if next.StartedAt == nil {
			next.StartedAt = &now
		}
		next.Stage = corev1.BackupStageQuota
		next.Progress = 5
	} else if phase == backupdb.Completed {
		applyCompletedBackupProjection(next)
	}
}

func (o *operator) updateBackupProgress(ctx context.Context, inst *corev1.Instance, opID, taskID, stage string, pct int32, processed, total, speed int64, msg string) {
	if inst == nil || strings.TrimSpace(opID) == "" {
		return
	}
	_ = o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for i := range items {
			if items[i].OperationID != opID {
				continue
			}
			applyTelemetryFields(&items[i], taskID, stage, pct, processed, total, speed, msg)
		}
		return items
	})
}

func (o *operator) updateRestoreProgress(ctx context.Context, inst *corev1.Instance, key, taskID, stage string, pct int32, processed, total, speed int64, msg string) {
	if inst == nil || strings.TrimSpace(key) == "" {
		return
	}
	_ = o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for i := range items {
			if items[i].Path != key || items[i].Phase != corev1.BackupPhaseRestoring {
				continue
			}
			applyTelemetryFields(&items[i], taskID, stage, pct, processed, total, speed, msg)
		}
		return items
	})
}

func applyTelemetryFields(rec *corev1.InstanceBackupRecord, taskID, stage string, pct int32, processed, total, speed int64, msg string) {
	if taskID != "" {
		rec.TaskID = taskID
	}
	if rec.StartedAt == nil {
		now := time.Now().UTC()
		rec.StartedAt = &now
	}
	if stage != "" {
		rec.Stage = stage
	}
	if pct >= 0 {
		rec.Progress = pct
	}
	if processed >= 0 {
		rec.ProcessedBytes = processed
	}
	if total >= 0 {
		rec.TotalBytes = total
	}
	if speed >= 0 {
		rec.SpeedBytesPerSec = speed
	}
	rec.Message = msg
}

func (o *operator) newTransferEventHandler(ctx context.Context, inst *corev1.Instance, id, taskID string, isRestore bool) func(corev1.InstanceTaskStreamEvent) {
	var mu sync.Mutex
	var lastStage string
	var lastPush time.Time
	return func(ev corev1.InstanceTaskStreamEvent) {
		switch ev.Type {
		case corev1.TaskStreamEventLog:
			AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, ev.Message)
		case corev1.TaskStreamEventError:
			msg := HumanizeTaskErrorString(ev.Message)
			if strings.HasPrefix(msg, "任务执行异常") {
				AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, msg)
			} else {
				AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "执行异常："+msg)
			}
		case corev1.TaskStreamEventProgress:
			mu.Lock()
			stageChanged := ev.Stage != "" && ev.Stage != lastStage
			shouldPush := stageChanged || time.Since(lastPush) >= 500*time.Millisecond
			if shouldPush {
				lastStage = ev.Stage
				lastPush = time.Now()
			}
			mu.Unlock()
			if !shouldPush {
				return
			}
			if isRestore {
				o.updateRestoreProgress(ctx, inst, id, taskID, ev.Stage, ev.Progress, ev.ProcessedBytes, ev.TotalBytes, ev.SpeedBytesPerSec, ev.Message)
			} else {
				o.updateBackupProgress(ctx, inst, id, taskID, ev.Stage, ev.Progress, ev.ProcessedBytes, ev.TotalBytes, ev.SpeedBytesPerSec, ev.Message)
			}
		}
	}
}

func HumanizeTaskErrorString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "未知错误"
	}
	return HumanizeTaskError(&taskStringError{msg: trimmed})
}

type taskStringError struct{ msg string }

func (e *taskStringError) Error() string { return e.msg }

func formatHumanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
