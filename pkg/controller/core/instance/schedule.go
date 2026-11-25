package instance

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

const (
	scheduleRunTimeout    = 45 * time.Second
	scheduleBackupTimeout = 10 * time.Minute
	scheduleUnknownAfter  = 2 * time.Minute
	scheduleStatusTries   = 5
)

func scheduleTaskID(runID string) string {
	if strings.HasPrefix(runID, "sched-") {
		return runID
	}
	return "sched-" + strings.TrimSpace(runID)
}

func scheduleStatus(inst *corev1.Instance, id string) *corev1.InstanceScheduleStatus {
	for n := range inst.Status.ScheduleStatuses {
		if inst.Status.ScheduleStatuses[n].ID == id {
			return &inst.Status.ScheduleStatuses[n]
		}
	}
	return nil
}

// scheduleOccurrence selects the most recent due occurrence. LastRunAt records
// the occurrence time, while ClaimedAt records the actual dispatch claim. Missed
// intervals are coalesced and an uncertain occurrence is never replayed.
func scheduleOccurrence(inst *corev1.Instance, sched corev1.InstanceSchedule, now time.Time) (time.Time, bool) {
	if strings.TrimSpace(sched.ID) == "" {
		return time.Time{}, false
	}
	var lastRun *time.Time
	if status := scheduleStatus(inst, sched.ID); status != nil {
		if status.LastResult == corev1.ScheduleResultRunning {
			return time.Time{}, false
		}
		lastRun = status.LastRunAt
	}
	if sched.ManualTriggerAt != nil && !sched.ManualTriggerAt.IsZero() && (lastRun == nil || lastRun.Before(*sched.ManualTriggerAt)) {
		return sched.ManualTriggerAt.UTC(), true
	}
	if !sched.Enabled {
		return time.Time{}, false
	}
	if daily := strings.TrimSpace(sched.DailyAt); daily != "" {
		clock, err := time.Parse("15:04", daily)
		if err != nil {
			return time.Time{}, false
		}
		local := now.In(cstZone)
		due := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, cstZone)
		if local.Before(due) || local.Sub(due) > 30*time.Minute || lastRun != nil && !lastRun.Before(due) {
			return time.Time{}, false
		}
		return due.UTC(), true
	}
	if sched.IntervalMinutes <= 0 || int64(sched.IntervalMinutes) > int64((time.Duration(1<<63-1))/time.Minute) {
		return time.Time{}, false
	}
	interval := time.Duration(sched.IntervalMinutes) * time.Minute
	base := inst.Metadata.CreationTimestamp
	if lastRun != nil && !lastRun.IsZero() {
		base = *lastRun
	} else if inst.Status.StartedAt != nil && !inst.Status.StartedAt.IsZero() {
		base = *inst.Status.StartedAt
	}
	if base.IsZero() {
		return now.UTC(), true
	}
	elapsed := now.Sub(base)
	if elapsed < interval {
		return time.Time{}, false
	}
	return base.Add(elapsed / interval * interval).UTC(), true
}

func isScheduleDue(inst *corev1.Instance, sched corev1.InstanceSchedule, now time.Time) bool {
	_, due := scheduleOccurrence(inst, sched, now)
	return due
}

func scheduleRunID(uid, id string, occurrence time.Time) string {
	hash := sha256.Sum256([]byte(uid + "\x00" + id + "\x00" + occurrence.UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", hash)
}

func publishScheduleStatus(ctx context.Context, bus mq.Interface, inst *corev1.Instance) {
	if bus == nil {
		return
	}
	event := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion)
	event.Attributes = map[string]string{"statusOnly": "true", "statusVersion": inst.Metadata.StatusVersion, "prevPhase": string(inst.Status.Phase), "hasUsage": "false"}
	_ = bus.Publish(ctx, mq.TopicInstance, event)
}

func claimSchedule(ctx context.Context, instDB *instancedb.DB, uid, tenant, name, id string, now time.Time) (*corev1.Instance, corev1.InstanceSchedule, bool, error) {
	for range scheduleStatusTries {
		inst, err := instDB.Get(ctx, tenant, name)
		if err != nil {
			return nil, corev1.InstanceSchedule{}, false, err
		}
		if inst.Metadata.UID != uid || inst.Metadata.DeletionTimestamp != nil || inst.Status.Phase != corev1.InstancePhaseRunning {
			return inst, corev1.InstanceSchedule{}, false, nil
		}
		var sched corev1.InstanceSchedule
		for _, candidate := range inst.Spec.Schedules {
			if candidate.ID == id {
				sched = candidate
				break
			}
		}
		due, ready := scheduleOccurrence(inst, sched, now)
		if !ready {
			return inst, sched, false, nil
		}
		status := scheduleStatus(inst, id)
		if status == nil {
			inst.Status.ScheduleStatuses = append(inst.Status.ScheduleStatuses, corev1.InstanceScheduleStatus{ID: id})
			status = &inst.Status.ScheduleStatuses[len(inst.Status.ScheduleStatuses)-1]
		}
		claimed := now.UTC()
		*status = corev1.InstanceScheduleStatus{
			ID: id, RunID: scheduleRunID(uid, id, due), ClaimedAt: &claimed, LastRunAt: &due,
			LastResult: corev1.ScheduleResultRunning, Stage: corev1.ScheduleStageQueued,
			Progress: 5, StartedAt: &claimed, LastOutput: "正在准备执行任务...",
		}
		updated, err := instDB.UpdateScheduleStatuses(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, inst.Metadata.StatusVersion, inst.Status.ScheduleStatuses)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		// A lost claim acknowledgement never authorizes external execution.
		return updated, sched, err == nil, err
	}
	return nil, corev1.InstanceSchedule{}, false, fmt.Errorf("%w: schedule claim changed repeatedly", db.ErrConflict)
}

func updateScheduleProgress(ctx context.Context, instDB *instancedb.DB, bus mq.Interface, uid, tenant, name, id, runID, stage string, progress int32, output string) {
	for range scheduleStatusTries {
		inst, err := instDB.Get(ctx, tenant, name)
		if err != nil || inst.Metadata.UID != uid {
			return
		}
		status := scheduleStatus(inst, id)
		if status == nil || status.RunID != runID || status.LastResult != corev1.ScheduleResultRunning {
			return
		}
		status.Stage, status.Progress = stage, progress
		if output != "" {
			status.LastOutput = output
		}
		updated, err := instDB.UpdateScheduleStatuses(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, inst.Metadata.StatusVersion, inst.Status.ScheduleStatuses)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		if err == nil {
			publishScheduleStatus(ctx, bus, updated)
		}
		return
	}
}

func completeSchedule(ctx context.Context, instDB *instancedb.DB, uid, tenant, name, id, runID, result, stage, output string) (*corev1.Instance, error) {
	for range scheduleStatusTries {
		inst, err := instDB.Get(ctx, tenant, name)
		if err != nil {
			return nil, err
		}
		if inst.Metadata.UID != uid {
			return nil, fmt.Errorf("%w: scheduled instance was replaced", db.ErrConflict)
		}
		status := scheduleStatus(inst, id)
		if status == nil || status.RunID != runID {
			return nil, fmt.Errorf("%w: schedule run is no longer current", db.ErrConflict)
		}
		if status.LastResult == result && status.LastOutput == output {
			return inst, nil
		}
		if status.LastResult != corev1.ScheduleResultRunning && status.LastResult != corev1.ScheduleResultUnknown {
			return nil, fmt.Errorf("%w: schedule run already completed", db.ErrConflict)
		}
		now := time.Now().UTC()
		status.LastResult, status.LastOutput, status.Stage, status.FinishedAt = result, output, stage, &now
		if result == corev1.ScheduleResultSucceeded || result == corev1.ScheduleResultWarning {
			status.Progress = 100
		}
		updated, err := instDB.UpdateScheduleStatuses(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, inst.Metadata.StatusVersion, inst.Status.ScheduleStatuses)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		return updated, err
	}
	return nil, fmt.Errorf("%w: schedule result changed repeatedly", db.ErrConflict)
}

func scheduleClaimTTL(inst *corev1.Instance, scheduleID string) time.Duration {
	if inst != nil {
		for _, s := range inst.Spec.Schedules {
			if s.ID == scheduleID && strings.EqualFold(strings.TrimSpace(s.Action), corev1.ScheduleActionBackup) {
				return 11 * time.Minute
			}
		}
	}
	return scheduleUnknownAfter
}

func recoverScheduleClaims(ctx context.Context, instDB *instancedb.DB, input *corev1.Instance, now time.Time) (*corev1.Instance, []corev1.InstanceScheduleStatus, error) {
	inst := input
	nowUTC := now.UTC()
	for range scheduleStatusTries {
		var recovered []corev1.InstanceScheduleStatus
		for n := range inst.Status.ScheduleStatuses {
			status := &inst.Status.ScheduleStatuses[n]
			ttl := scheduleClaimTTL(inst, status.ID)
			if status.LastResult == corev1.ScheduleResultRunning && (status.ClaimedAt == nil || now.Sub(*status.ClaimedAt) >= ttl) {
				status.LastResult = corev1.ScheduleResultUnknown
				status.Stage = corev1.ScheduleStageFailed
				status.FinishedAt = &nowUTC
				status.LastOutput = "任务执行超时或控制面中断，为防止重复副作用本次不再重放"
				recovered = append(recovered, *status)
			}
		}
		if len(recovered) == 0 {
			return inst, nil, nil
		}
		updated, err := instDB.UpdateScheduleStatuses(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, inst.Metadata.StatusVersion, inst.Status.ScheduleStatuses)
		if !errors.Is(err, db.ErrConflict) {
			if err != nil {
				return updated, nil, err
			}
			return updated, recovered, nil
		}
		inst, err = instDB.Get(ctx, input.Metadata.Tenant, input.Metadata.Name)
		if err != nil {
			return nil, nil, err
		}
		if inst.Metadata.UID != input.Metadata.UID {
			return nil, nil, fmt.Errorf("%w: scheduled instance was replaced", db.ErrConflict)
		}
	}
	return nil, nil, fmt.Errorf("%w: schedule recovery changed repeatedly", db.ErrConflict)
}

func (c *Controller) reconcileDueSchedules(ctx context.Context, reg *repo.Registry, bus mq.Interface, inst *corev1.Instance) error {
	instDB := instancedb.NewDB(reg.Pool())
	inst, recovered, err := recoverScheduleClaims(ctx, instDB, inst, time.Now())
	if err != nil {
		return err
	}
	if len(recovered) > 0 {
		publishScheduleStatus(ctx, bus, inst)
	}
	if !hasDueSchedules(inst, time.Now()) {
		return nil
	}
	tl := tasklog.New(cache.Default(), bus)
	op := instancemodel.NewOperator(reg, nil, bus, instancemodel.WithHTTPClient(transferHTTPClient), instancemodel.WithTaskLog(tl))
	for _, candidate := range inst.Spec.Schedules {
		if err := runSingleDueSchedule(ctx, reg, bus, instDB, tl, op, inst, candidate.ID); err != nil {
			return err
		}
	}
	return nil
}

func scheduleActionTimeout(action string) time.Duration {
	if strings.EqualFold(strings.TrimSpace(action), corev1.ScheduleActionBackup) {
		return scheduleBackupTimeout
	}
	return scheduleRunTimeout
}

func runSingleDueSchedule(ctx context.Context, reg *repo.Registry, bus mq.Interface, instDB *instancedb.DB, tl tasklog.Interface, op instancemodel.Operator, inst *corev1.Instance, scheduleID string) error {
	claimed, sched, run, err := claimSchedule(ctx, instDB, inst.Metadata.UID, inst.Metadata.Tenant, inst.Metadata.Name, scheduleID, time.Now())
	if err != nil || !run {
		return err
	}
	publishScheduleStatus(ctx, bus, claimed)
	runID := scheduleStatus(claimed, sched.ID).RunID
	taskID := scheduleTaskID(runID)
	instancemodel.AppendInstanceTaskLog(ctx, tl, claimed.Metadata.UID, taskID, fmt.Sprintf("开始执行定时任务「%s」", sched.Name))

	runCtx, cancel := context.WithTimeout(ctx, scheduleActionTimeout(sched.Action))
	result, output, runErr := executeSchedule(runCtx, reg, instDB, bus, tl, op, claimed, sched, runID, taskID)
	cancel()
	stage := corev1.ScheduleStageCompleted
	if runErr != nil {
		result, stage = corev1.ScheduleResultFailed, corev1.ScheduleStageFailed
		if errors.Is(runErr, context.DeadlineExceeded) {
			result = corev1.ScheduleResultUnknown
		}
		output = instancemodel.HumanizeTaskError(runErr)
		instancemodel.AppendInstanceTaskLog(ctx, tl, claimed.Metadata.UID, taskID, "任务执行失败: "+output)
	} else {
		if result == "" {
			result = corev1.ScheduleResultSucceeded
		}
		instancemodel.AppendInstanceTaskLog(ctx, tl, claimed.Metadata.UID, taskID, "任务执行完成: "+output)
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	instancemodel.FinishInstanceTaskLog(finishCtx, tl, claimed.Metadata.UID, taskID)
	completed, statusErr := completeSchedule(finishCtx, instDB, claimed.Metadata.UID, claimed.Metadata.Tenant, claimed.Metadata.Name, sched.ID, runID, result, stage, output)
	if statusErr == nil {
		publishScheduleStatus(finishCtx, bus, completed)
	}
	finishCancel()
	if statusErr != nil {
		return statusErr
	}
	return runErr
}

func executeSchedule(ctx context.Context, reg *repo.Registry, instDB *instancedb.DB, bus mq.Interface, tl tasklog.Interface, op instancemodel.Operator, inst *corev1.Instance, sched corev1.InstanceSchedule, runID, taskID string) (string, string, error) {
	action := strings.ToLower(strings.TrimSpace(sched.Action))
	if action != corev1.ScheduleActionCommand && action != corev1.ScheduleActionBackup && action != corev1.ScheduleActionRestart {
		return "", "", fmt.Errorf("unsupported scheduled action %q", sched.Action)
	}
	output, err := executeScheduleCommands(ctx, reg, instDB, bus, tl, op, inst, sched, runID, taskID)
	if err != nil {
		return "", "", err
	}
	switch action {
	case corev1.ScheduleActionBackup:
		return executeScheduleBackup(ctx, instDB, bus, tl, op, inst, sched, runID, taskID)
	case corev1.ScheduleActionRestart:
		out, rErr := executeScheduleRestart(ctx, instDB, bus, tl, op, inst, sched, runID, taskID)
		return corev1.ScheduleResultSucceeded, out, rErr
	default:
		if output == "" {
			output = "已执行计划指令"
		}
		return corev1.ScheduleResultSucceeded, output, nil
	}
}

func executeScheduleCommands(ctx context.Context, reg *repo.Registry, instDB *instancedb.DB, bus mq.Interface, tl tasklog.Interface, op instancemodel.Operator, inst *corev1.Instance, sched corev1.InstanceSchedule, runID, taskID string) (string, error) {
	var lines []string
	for _, raw := range strings.Split(sched.Command, "\n") {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	updateScheduleProgress(ctx, instDB, bus, inst.Metadata.UID, inst.Metadata.Tenant, inst.Metadata.Name, sched.ID, runID, corev1.ScheduleStageExecutingCommand, 25, "正在发送服务器指令...")
	delivery := resolveInstanceConsoleDelivery(ctx, reg, inst)
	output := ""
	for _, line := range lines {
		instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "发送指令: "+line)
		response, err := op.Exec(ctx, inst.Metadata.Tenant, inst.Metadata.Name, &corev1.InstanceExec{Metadata: metav1.ObjectMeta{UID: inst.Metadata.UID}, Command: buildConsoleCommandArgv(delivery, line)})
		if err != nil {
			return "", err
		}
		output = "已下发指令: " + line
		if response != nil && strings.TrimSpace(response.Output) != "" {
			output = strings.TrimSpace(response.Output)
			instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "指令回执: "+output)
		}
	}
	return output, nil
}

func executeScheduleBackup(ctx context.Context, instDB *instancedb.DB, bus mq.Interface, tl tasklog.Interface, op instancemodel.Operator, inst *corev1.Instance, sched corev1.InstanceSchedule, runID, taskID string) (string, string, error) {
	updateScheduleProgress(ctx, instDB, bus, inst.Metadata.UID, inst.Metadata.Tenant, inst.Metadata.Name, sched.ID, runID, corev1.ScheduleStageArchivingBackup, 55, "正在归档并上传备份...")
	if config.Get() == nil || !config.Get().S3.IsConfigured() {
		instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "已执行存档保存指令")
		return corev1.ScheduleResultSucceeded, "已执行存档保存指令", nil
	}
	runAt := time.Now().UTC()
	if st := scheduleStatus(inst, sched.ID); st != nil && st.LastRunAt != nil {
		runAt = *st.LastRunAt
	}
	opID := "schedule:" + inst.Metadata.UID + ":" + runID
	instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "正在提交并执行归档备份任务...")
	if _, err := op.Files(ctx, inst.Metadata.Tenant, inst.Metadata.Name, &corev1.InstanceFiles{
		Metadata: metav1.ObjectMeta{UID: inst.Metadata.UID}, Action: corev1.FileActionBackup,
		OperationID: opID, Path: buildAutoBackupArchivePath(inst.Metadata.Name, runAt),
	}); err != nil {
		return "", "", err
	}
	latest, err := instDB.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return "", "", err
	}
	if err := op.ReconcileBackups(ctx, latest); err != nil {
		return "", "", err
	}
	after, err := instDB.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return "", "", err
	}
	for _, b := range after.Status.Backups {
		if b.OperationID == opID {
			return scheduleBackupOutcome(ctx, tl, inst.Metadata.UID, taskID, b)
		}
	}
	return corev1.ScheduleResultSucceeded, "已完成定时归档备份", nil
}

func scheduleBackupOutcome(ctx context.Context, tl tasklog.Interface, uid, taskID string, b corev1.InstanceBackupRecord) (string, string, error) {
	if b.Phase != corev1.BackupPhaseCompleted {
		if b.Message != "" {
			return "", "", errors.New(b.Message)
		}
		return "", "", fmt.Errorf("定时备份未完成（当前阶段: %s）", b.Phase)
	}
	if b.SaveConfirmation != nil && b.SaveConfirmation.Status == corev1.BackupSaveConfirmationUnconfirmed {
		warn := strings.TrimSpace(b.SaveConfirmation.Warning)
		msg := "已归档，存档未确认"
		if warn != "" {
			msg += "：" + warn
		}
		instancemodel.AppendInstanceTaskLog(ctx, tl, uid, taskID, fmt.Sprintf("归档备份已完成（%d 字节），警告：%s", b.SizeBytes, msg))
		return corev1.ScheduleResultWarning, msg, nil
	}
	instancemodel.AppendInstanceTaskLog(ctx, tl, uid, taskID, fmt.Sprintf("归档备份已完成（%d 字节）", b.SizeBytes))
	return corev1.ScheduleResultSucceeded, "已完成定时归档备份", nil
}

func executeScheduleRestart(ctx context.Context, instDB *instancedb.DB, bus mq.Interface, tl tasklog.Interface, op instancemodel.Operator, inst *corev1.Instance, sched corev1.InstanceSchedule, runID, taskID string) (string, error) {
	updateScheduleProgress(ctx, instDB, bus, inst.Metadata.UID, inst.Metadata.Tenant, inst.Metadata.Name, sched.ID, runID, corev1.ScheduleStageRestartingInstance, 75, "正在重启服务器...")
	instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "等待缓冲后执行平滑重启...")
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
	}
	if _, err := op.UpdateState(ctx, inst.Metadata.Tenant, inst.Metadata.Name, &corev1.InstanceState{Metadata: metav1.ObjectMeta{UID: inst.Metadata.UID}, State: corev1.InstanceStateRestart}); err != nil {
		return "", err
	}
	instancemodel.AppendInstanceTaskLog(ctx, tl, inst.Metadata.UID, taskID, "已下发服务器重启指令")
	return "已触发实例例行重启", nil
}

func resolveInstanceConsoleDelivery(ctx context.Context, reg *repo.Registry, inst *corev1.Instance) string {
	if reg == nil || reg.Pool() == nil || inst == nil {
		return ""
	}
	appDB := appdb.NewDB(reg.Pool())
	appName := strings.TrimSpace(inst.Metadata.Labels[corev1.LabelInstancePreset])
	if appName == "" {
		appName = strings.TrimSpace(inst.Metadata.Labels[corev1.LabelInstanceApp])
	}
	if appName != "" {
		if app, err := appDB.Get(ctx, appName); err == nil && app != nil {
			return strings.TrimSpace(app.ConsoleDelivery)
		}
	}
	return ""
}

func buildConsoleCommandArgv(delivery, line string) []string {
	return drvruntime.BuildConsoleCommandArgv(delivery, line)
}
