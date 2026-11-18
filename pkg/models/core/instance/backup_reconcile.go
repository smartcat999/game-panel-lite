package instance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// ReconcileBackups advances all pending backup creations, deletions, and restores for an instance.
func (o *operator) ReconcileBackups(ctx context.Context, inst *corev1.Instance) error {
	if inst == nil || inst.Metadata.UID == "" || inst.Metadata.DeletionTimestamp != nil || o.backupDB == nil {
		return nil
	}
	unlock, acquired := cache.TryLock(ctx, "gamepanel:lock:instance:backup:"+inst.Metadata.UID, backupReconcileLockTTL)
	if !acquired {
		return nil
	}
	defer unlock()
	restoreErr := o.reconcilePendingRestores(ctx, inst)
	records, err := o.backupDB.ListInstance(ctx, inst.Metadata.UID)
	if err != nil {
		return errors.Join(restoreErr, err)
	}
	var errs []error
	if restoreErr != nil {
		errs = append(errs, restoreErr)
	}
	known := make(map[string]struct{}, len(records))
	for _, r := range records {
		known[r.Command.ObjectKey] = struct{}{}
		if rErr := o.reconcileBackupRecord(ctx, inst, r); rErr != nil {
			errs = append(errs, rErr)
		}
	}
	if lErr := o.reconcileLegacyDeletingBackups(ctx, inst, known); lErr != nil {
		errs = append(errs, lErr)
	}
	if iErr := o.reconcilePendingBackupInspections(ctx, o.latestInstanceForBackupInspect(ctx, inst)); iErr != nil {
		errs = append(errs, iErr)
	}
	return errors.Join(errs...)
}

func (o *operator) reconcilePendingRestores(ctx context.Context, inst *corev1.Instance) error {
	var errs []error
	for _, b := range inst.Status.Backups {
		if b.Phase != corev1.BackupPhaseRestoring {
			continue
		}
		if err := o.executePendingRestore(ctx, inst, b); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (o *operator) executePendingRestore(ctx context.Context, inst *corev1.Instance, b corev1.InstanceBackupRecord) error {
	taskID := strings.TrimSpace(b.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(b.LastRestoreTaskID)
	}
	if taskID == "" {
		taskID = restoreTaskID(b.Path, time.Now().UTC())
	}
	if err := o.runRestoreTransfer(ctx, inst, b, taskID); err != nil {
		o.abortFailedRestore(ctx, inst, b.Path, taskID, err)
		return err
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "存档恢复已完成")
	FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID)
	return o.finishRestoreOutcome(ctx, inst, b.Path, taskID, corev1.RestoreResultSucceeded, "")
}

func (o *operator) runRestoreTransfer(ctx context.Context, inst *corev1.Instance, b corev1.InstanceBackupRecord, taskID string) error {
	cli := o.s3Client()
	if !cli.IsConfigured() {
		return fmt.Errorf("%w: platform object storage (s3) is not configured", models.ErrBadRequest)
	}
	presignedURL, _, err := cli.PresignGet(ctx, b.Path)
	if err != nil {
		return err
	}
	maxBytes, err := resolveFileActionMaxBytes(inst, corev1.InstanceFiles{Action: corev1.FileActionRestore})
	if err != nil {
		return err
	}
	sourceRel, targetRel, replaceTarget, err := resolveRestoreMapping(inst, b.RestoreSourcePath, b.RestoreTargetPath, b.RestoreReplace)
	if err != nil {
		return err
	}
	input := &corev1.InstanceFiles{
		Metadata:      inst.Metadata,
		Action:        corev1.FileActionRestore,
		Path:          ".",
		SourcePath:    sourceRel,
		TargetPath:    targetRel,
		ReplaceTarget: &replaceTarget,
		PresignedURL:  presignedURL,
		MaxBytes:      maxBytes,
	}
	onEvent := o.newTransferEventHandler(ctx, inst, b.Path, taskID, true)
	_, err = o.transferFilesStreaming(ctx, inst, input, time.Now().UTC().Add(30*time.Minute), onEvent)
	return err
}

func (o *operator) finishRestoreOutcome(ctx context.Context, inst *corev1.Instance, key, taskID, result, message string) error {
	now := time.Now().UTC()
	return o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for n := range items {
			if items[n].Path == key && items[n].Phase == corev1.BackupPhaseRestoring {
				items[n].Phase = string(backupdb.Completed)
				items[n].Stage = corev1.BackupStageCompleted
				items[n].Progress = 100
				items[n].SpeedBytesPerSec = 0
				items[n].Message = ""
				items[n].TaskID = backupTaskID(items[n].OperationID)
				items[n].RestoreSourcePath = ""
				items[n].RestoreTargetPath = ""
				items[n].RestoreReplace = nil
				if taskID != "" {
					items[n].LastRestoreTaskID = taskID
				}
				items[n].LastRestoreResult = result
				items[n].LastRestoreMessage = message
				items[n].LastRestoreAt = &now
			}
		}
		return items
	})
}

func (o *operator) abortFailedRestore(ctx context.Context, inst *corev1.Instance, key, taskID string, err error) {
	if errors.Is(err, db.ErrConflict) || ctx.Err() != nil {
		return
	}
	msg := HumanizeTaskError(err)
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "存档恢复失败: "+msg)
	FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID)
	_ = o.finishRestoreOutcome(ctx, inst, key, taskID, corev1.RestoreResultFailed, msg)
	RecordTaskFailure(ctx, o.reg, TaskFailureNotice{
		DedupKey:     "bak-restore:" + inst.Metadata.UID + ":" + key + ":" + time.Now().UTC().Truncate(time.Minute).Format(time.RFC3339),
		Tenant:       inst.Metadata.Tenant,
		InstanceName: inst.Metadata.Name,
		InstanceUID:  inst.Metadata.UID,
		TaskKind:     corev1.TaskKindBackupRestore,
		TaskName:     FormatBackupDisplayName(inst.Metadata.Name, key, ""),
		Path:         key,
		Err:          err,
	})
}

func (o *operator) recordBackupRecordFailure(ctx context.Context, inst *corev1.Instance, r backupdb.Record, stage string, err error) {
	msg := HumanizeTaskError(err)
	taskID := backupTaskID(r.Command.OperationID)
	o.updateBackupProgress(ctx, inst, r.Command.OperationID, taskID, corev1.BackupStageFailed, -1, -1, -1, 0, msg)
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "备份执行失败: "+msg)
	if r.Phase == backupdb.PendingQuota || r.Phase == backupdb.Unknown {
		FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID)
	}
	RecordTaskFailure(ctx, o.reg, TaskFailureNotice{
		DedupKey:     stage + ":" + r.Command.OperationID,
		Tenant:       inst.Metadata.Tenant,
		InstanceName: inst.Metadata.Name,
		InstanceUID:  inst.Metadata.UID,
		TaskKind:     corev1.TaskKindBackupArchive,
		TaskName:     FormatBackupDisplayName(inst.Metadata.Name, r.Command.ObjectKey, r.Command.BackupUID),
		OperationID:  r.Command.OperationID,
		Path:         r.Command.ObjectKey,
		Err:          err,
	})
}

func (o *operator) recordBackupTransientRetry(ctx context.Context, inst *corev1.Instance, r backupdb.Record, _ error) {
	taskID := backupTaskID(r.Command.OperationID)
	o.updateBackupProgress(ctx, inst, r.Command.OperationID, taskID, corev1.BackupStageArchiving, -1, -1, -1, 0, "传输中断，正在自动重试...")
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "首次打包传输受阻，正在自动重试...")
}

const (
	directUploadSetupTTL     = 30 * time.Second
	directUploadExpiryMargin = 5 * time.Second
)

func directUploadFenceDeadline(exp *time.Time) (time.Time, bool) {
	if exp == nil || exp.IsZero() {
		return time.Time{}, false
	}
	return exp.UTC().Add(directUploadExpiryMargin), true
}

func (o *operator) reconcileDirectUploadSetupTimeout(ctx context.Context, inst *corev1.Instance, r backupdb.Record) error {
	if r.CreatedAt.IsZero() || o.now().Before(r.CreatedAt.UTC().Add(directUploadSetupTTL)) {
		return nil
	}
	if err := o.closeBackup(ctx, r, true); err != nil {
		return err
	}
	o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	return nil
}

func (o *operator) reconcileBackupRecord(ctx context.Context, inst *corev1.Instance, r backupdb.Record) error {
	switch r.Phase {
	case backupdb.PendingQuota:
		if r.Command.DirectUpload {
			return o.reconcileDirectUploadSetupTimeout(ctx, inst, r)
		}
		return o.reconcilePendingQuotaBackup(ctx, inst, r)
	case backupdb.Ready:
		if r.Command.DirectUpload {
			return o.reconcileDirectUploadSetupTimeout(ctx, inst, r)
		}
		_, err := o.streamAndCompleteBackup(ctx, inst, r)
		return err
	case backupdb.Uploading, backupdb.Unknown:
		_, err := o.recoverBackup(ctx, inst, r)
		return err
	case backupdb.Completed:
		return o.reconcileCompletedBackup(ctx, inst, r)
	case backupdb.Deleting:
		if err := o.closeBackup(ctx, r, true); err != nil {
			return err
		}
		o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
		return nil
	default:
		return nil
	}
}

func (o *operator) reconcilePendingQuotaBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record) error {
	if err := o.projectBackup(ctx, r); err != nil {
		return err
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, backupTaskID(r.Command.OperationID), "正在校验工作区备份数量与存储配额...")
	if err := o.makeBackupRoom(ctx, inst, r); err != nil {
		o.recordBackupRecordFailure(ctx, inst, r, "bak-quota", err)
		return err
	}
	funded, err := o.fundBackup(ctx, inst, r)
	if err != nil {
		o.recordBackupRecordFailure(ctx, inst, r, "bak-quota", err)
		return backupConflict(err)
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, backupTaskID(funded.Command.OperationID), "配额校验通过，正在连接服务器所在节点...")
	_, err = o.streamAndCompleteBackup(ctx, inst, funded)
	return err
}

func (o *operator) reconcileCompletedBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record) error {
	if r.Result == nil {
		return nil
	}
	r, err := o.finalizeDirectUploadCompletion(ctx, r)
	if err != nil {
		return err
	}
	settled, err := o.settleBackupQuota(ctx, r, r.Result.SizeBytes)
	if err != nil {
		return err
	}
	if err := o.projectBackup(ctx, r); err != nil {
		return err
	}
	if settled {
		o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	}
	return nil
}

func (o *operator) reconcileLegacyDeletingBackups(ctx context.Context, inst *corev1.Instance, known map[string]struct{}) error {
	var errs []error
	for _, b := range inst.Status.Backups {
		if b.Phase != string(backupdb.Deleting) {
			continue
		}
		if _, ok := known[b.Path]; ok {
			continue
		}
		if err := o.deleteBackupObject(ctx, b.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		key := b.Path
		if err := o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
			var out []corev1.InstanceBackupRecord
			for _, item := range items {
				if item.Path != key {
					out = append(out, item)
				}
			}
			return out
		}); err != nil {
			errs = append(errs, err)
			continue
		}
		o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	}
	return errors.Join(errs...)
}

func (o *operator) streamAndCompleteBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record) (*corev1.InstanceFiles, error) {
	current, presigned, err := o.prepareBackupUpload(ctx, inst, r)
	if err != nil {
		o.recordBackupRecordFailure(ctx, inst, r, "bak-xfer", err)
		return backupResponse(inst, r), err
	}
	expires := time.Now().UTC().Add(30 * time.Minute)
	claimed, err := o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Uploading, UploadExpiresAt: &expires})
	if err != nil {
		return backupResponse(inst, r), backupConflict(err)
	}
	r = claimed
	_ = o.projectBackup(ctx, r)
	taskID := backupTaskID(r.Command.OperationID)
	o.updateBackupProgress(ctx, current, r.Command.OperationID, taskID, corev1.BackupStageArchiving, 10, 0, -1, 0, "正在打包存档")
	input := &corev1.InstanceFiles{
		Action: corev1.FileActionBackup, OperationID: r.Command.OperationID, Path: r.Command.ObjectKey,
		PresignedURL: presigned, MaxBytes: r.Command.MaxBytes, RateLimitBytesPerSec: r.Command.RateLimit,
	}
	onEvent := o.newTransferEventHandler(ctx, current, r.Command.OperationID, taskID, false)
	result, err := o.transferFilesStreaming(ctx, current, input, expires, onEvent)
	if err != nil {
		unknown, markErr := o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Unknown})
		if markErr == nil {
			_ = o.projectBackup(ctx, unknown)
			o.recordBackupTransientRetry(ctx, inst, unknown, err)
			return backupResponse(inst, unknown), err
		}
		if !errors.Is(markErr, backupdb.ErrConflict) {
			o.recordBackupRecordFailure(ctx, inst, r, "bak-xfer", err)
		}
		return backupResponse(inst, r), err
	}
	return o.finalizeCompletedBackup(ctx, inst, r, result)
}

func (o *operator) prepareBackupUpload(ctx context.Context, inst *corev1.Instance, r backupdb.Record) (*corev1.Instance, string, error) {
	current, err := o.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return nil, "", err
	}
	claims, err := o.fileExecutionClaims(ctx, current)
	if err != nil {
		return nil, "", err
	}
	if current.Metadata.UID != r.Command.Owner.InstanceUID || claims.NodeUID != r.Command.Binding.NodeUID || claims.AttemptID != r.Command.Binding.AttemptID || claims.Epoch != r.Command.Binding.PlacementEpoch || claims.ApprovedRevision != r.Command.ApprovedRevision {
		return nil, "", db.ErrConflict
	}
	cli, err := o.backupS3Client(r)
	if err != nil {
		return nil, "", err
	}
	presigned, _, err := cli.PresignPut(ctx, r.Command.ObjectKey)
	if err != nil {
		return nil, "", err
	}
	return current, presigned, nil
}

func (o *operator) finalizeCompletedBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record, result *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	completedAt := time.Now().UTC()
	if result.CompletedAt != nil {
		completedAt = result.CompletedAt.UTC()
	}
	taskID := backupTaskID(r.Command.OperationID)
	o.updateBackupProgress(ctx, inst, r.Command.OperationID, taskID, corev1.BackupStageSettling, 99, result.SizeBytes, result.SizeBytes, 0, "正在结算配额")
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, fmt.Sprintf("备份上传成功（%s），正在结算空间配额...", formatHumanBytes(result.SizeBytes)))
	completed, err := o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{
		Phase: backupdb.Completed, Result: &backupdb.Result{SizeBytes: result.SizeBytes, SHA256: result.SHA256, CompletedAt: completedAt},
	})
	if err != nil {
		return backupResponse(inst, r), backupConflict(err)
	}
	if _, err := o.settleBackupQuota(ctx, completed, result.SizeBytes); err != nil {
		return backupResponse(inst, completed), err
	}
	if err := o.projectBackup(ctx, completed); err != nil {
		return backupResponse(inst, completed), err
	}
	o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, "备份已完成")
	FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID)
	return backupResponse(inst, completed), o.enforceBackupRetention(ctx, inst, completed.Command.BackupUID)
}

func (r *StageReconciler) cleanupBackups(ctx context.Context, inst *corev1.Instance) (*corev1.Instance, error) {
	if inst == nil || inst.Metadata.UID == "" || inst.Metadata.DeletionTimestamp == nil || GetStage(inst) != StageDeleting {
		return inst, db.ErrConflict
	}
	if r.backupDB == nil || r.quotaDB == nil {
		return inst, errors.New("backup command and quota stores are required")
	}
	if err := r.backupDB.CloseInstance(ctx, inst.Metadata.UID); err != nil {
		return inst, err
	}
	records, err := r.backupDB.ListInstance(ctx, inst.Metadata.UID)
	if err != nil {
		return inst, err
	}
	if len(records) == 0 && len(inst.Status.Backups) == 0 {
		return inst, nil
	}
	op := NewOperator(r.reg, nil, nil, WithHTTPClient(&http.Client{Timeout: 20 * time.Second})).(*operator)
	known := make(map[string]backupdb.Record, len(records))
	for _, rec := range records {
		if err := op.closeBackup(ctx, rec, false); err != nil {
			return inst, err
		}
		known[rec.Command.ObjectKey] = rec
	}
	for _, backup := range inst.Status.Backups {
		key := strings.TrimLeft(strings.TrimSpace(backup.Path), "/")
		if key == "" {
			return inst, errors.New("recorded backup has no cleanup object key")
		}
		if _, ok := known[key]; ok {
			continue
		}
		if backup.UID != "" || backup.OperationID != "" || backup.AllocationID != "" {
			return inst, errors.New("recorded backup command is missing")
		}
		if err := op.deleteBackupObject(ctx, key); err != nil {
			return inst, err
		}
	}
	return r.clearStatusBackups(ctx, inst, known)
}

func (r *StageReconciler) clearStatusBackups(ctx context.Context, inst *corev1.Instance, known map[string]backupdb.Record) (*corev1.Instance, error) {
	for range 8 {
		current, err := r.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
		if err != nil {
			return inst, err
		}
		if current.Metadata.UID != inst.Metadata.UID || current.Metadata.DeletionTimestamp == nil || GetStage(current) != StageDeleting {
			return inst, db.ErrConflict
		}
		if !allBackupsKnownOrRecorded(current.Status.Backups, inst.Status.Backups, known) {
			return inst, db.ErrConflict
		}
		if len(current.Status.Backups) == 0 {
			return current, nil
		}
		updated, err := r.db.UpdateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, current.Metadata.UID, current.Metadata.StatusVersion, nil)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		return updated, err
	}
	return inst, db.ErrConflict
}

func allBackupsKnownOrRecorded(current, snapshot []corev1.InstanceBackupRecord, known map[string]backupdb.Record) bool {
	for _, b := range current {
		if _, ok := known[b.Path]; ok {
			continue
		}
		found := false
		for _, old := range snapshot {
			if old.Path == b.Path && old.UID == b.UID && old.OperationID == b.OperationID && old.AllocationID == b.AllocationID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (o *operator) fundBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record) (backupdb.Record, error) {
	if r.Phase != backupdb.PendingQuota {
		return r, nil
	}
	if _, err := o.requireBackupQuota(ctx, inst); err != nil {
		return r, err
	}
	c := backupQuotaCommand(r, corev1.Grant)
	if _, err := o.quotaDB.OpenAllocation(ctx, pgutil.AllocationIntent{AllocationID: c.AllocationID, Owner: c.Owner, ResourceRevision: 1, Quantity: c.Quantity}); err != nil {
		return r, err
	}
	if _, err := o.quotaDB.RegisterCommand(ctx, c); err != nil {
		return r, err
	}
	if _, err := o.quotaDB.TryGrant(ctx, c.OperationID); err != nil {
		return r, err
	}
	return o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Ready})
}

var errInvalidBackupObservation = errors.New("invalid object storage backup observation")

func (o *operator) inspectBackup(ctx context.Context, r backupdb.Record) (*backupdb.Result, error) {
	cli, err := o.backupS3Client(r)
	if err != nil {
		return nil, err
	}
	if r.Command.DirectUpload {
		return o.inspectDirectUploadBackup(ctx, cli, r)
	}
	stat, err := cli.Stat(ctx, r.Command.ObjectKey)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, s3.ErrNotFound) {
			return nil, &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: r.Command.OperationID, Detail: "original upload is unconfirmed; its budget is retained"}
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	if stat.SizeBytes < 0 || stat.SizeBytes > r.Command.MaxBytes {
		return nil, errors.Join(models.ErrBadGateway, errInvalidBackupObservation)
	}
	return &backupdb.Result{SizeBytes: stat.SizeBytes, SHA256: stat.SHA256, CompletedAt: o.now()}, nil
}

func (o *operator) inspectDirectUploadBackup(ctx context.Context, cli s3.Interface, r backupdb.Record) (*backupdb.Result, error) {
	stagingKey := directUploadStagingKey(r.Command.OperationID)
	stat, err := cli.Stat(ctx, r.Command.ObjectKey)
	if err == nil {
		if stat.SizeBytes < 0 || stat.SizeBytes > r.Command.MaxBytes {
			if delErr := o.deleteBackupObject(ctx, r.Command.ObjectKey); delErr != nil {
				return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, delErr)
			}
			if delErr := o.deleteBackupObject(ctx, stagingKey); delErr != nil {
				return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, delErr)
			}
			return nil, errors.Join(models.ErrBadGateway, errInvalidBackupObservation)
		}
		cleanErr := o.deleteBackupObject(ctx, stagingKey)
		return &backupdb.Result{SizeBytes: stat.SizeBytes, SHA256: stat.SHA256, CompletedAt: o.now(), StagingCleaned: cleanErr == nil}, nil
	}
	if !errors.Is(err, s3.ErrNotFound) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	stageStat, err := cli.Stat(ctx, stagingKey)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, s3.ErrNotFound) {
			return nil, &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: r.Command.OperationID, Detail: "direct upload is unconfirmed; its budget is retained"}
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	if stageStat.SizeBytes < 0 || stageStat.SizeBytes > r.Command.MaxBytes {
		if delErr := o.deleteBackupObject(ctx, stagingKey); delErr != nil {
			return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, delErr)
		}
		return nil, errors.Join(models.ErrBadGateway, errInvalidBackupObservation)
	}
	return o.promoteDirectUploadStaging(ctx, cli, r, stagingKey)
}

func (o *operator) promoteDirectUploadStaging(ctx context.Context, cli s3.Interface, r backupdb.Record, stagingKey string) (*backupdb.Result, error) {
	if err := cli.Copy(ctx, stagingKey, r.Command.ObjectKey); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: promote staged backup: %v", models.ErrBadGateway, err)
	}
	finalStat, err := cli.Stat(ctx, r.Command.ObjectKey)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	if finalStat.SizeBytes < 0 || finalStat.SizeBytes > r.Command.MaxBytes {
		if delErr := o.deleteBackupObject(ctx, r.Command.ObjectKey); delErr != nil {
			return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, delErr)
		}
		if delErr := o.deleteBackupObject(ctx, stagingKey); delErr != nil {
			return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, delErr)
		}
		return nil, errors.Join(models.ErrBadGateway, errInvalidBackupObservation)
	}
	cleanErr := o.deleteBackupObject(ctx, stagingKey)
	return &backupdb.Result{SizeBytes: finalStat.SizeBytes, SHA256: finalStat.SHA256, CompletedAt: o.now(), StagingCleaned: cleanErr == nil}, nil
}

func (o *operator) finalizeDirectUploadCompletion(ctx context.Context, r backupdb.Record) (backupdb.Record, error) {
	if !r.Command.DirectUpload || r.Result == nil || r.Result.StagingCleaned {
		return r, nil
	}
	if _, err := o.backupS3Client(r); err != nil {
		return r, err
	}
	if err := o.deleteBackupObject(ctx, directUploadStagingKey(r.Command.OperationID)); err != nil {
		return r, fmt.Errorf("%w: backup staging cleanup failed: %v", models.ErrBadGateway, err)
	}
	updated, err := o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{
		Phase:              backupdb.Completed,
		MarkStagingCleaned: true,
	})
	if err != nil {
		return r, backupConflict(err)
	}
	return updated, nil
}

func (o *operator) recoverBackup(ctx context.Context, inst *corev1.Instance, r backupdb.Record) (*corev1.InstanceFiles, error) {
	result, err := o.inspectBackup(ctx, r)
	if err != nil {
		if r.Command.DirectUpload {
			return o.reconcileDirectUploadError(ctx, inst, r, err)
		}
		var ctrlErr *corev1.Error
		if r.Phase == backupdb.Unknown && errors.As(err, &ctrlErr) && ctrlErr.Code == corev1.OutcomeUnknown {
			retryRes, retryErr := o.retryBackupTransfer(ctx, inst, r)
			if retryErr != nil {
				o.recordBackupRecordFailure(ctx, inst, r, "bak-xfer", retryErr)
				return backupResponse(inst, r), retryErr
			}
			result, err = retryRes, nil
		}
		if err != nil {
			return backupResponse(inst, r), err
		}
	}
	r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Completed, Result: result})
	if err != nil {
		return nil, backupConflict(err)
	}
	r, err = o.finalizeDirectUploadCompletion(ctx, r)
	if err != nil {
		return backupResponse(inst, r), err
	}
	if _, err := o.settleBackupQuota(ctx, r, result.SizeBytes); err != nil {
		return backupResponse(inst, r), err
	}
	if err := o.projectBackup(ctx, r); err != nil {
		return backupResponse(inst, r), err
	}
	o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	o.finishRecoveredBackupTaskLog(ctx, inst, r, result.SizeBytes)
	return backupResponse(inst, r), o.enforceBackupRetention(ctx, inst, r.Command.BackupUID)
}

func (o *operator) reconcileDirectUploadError(ctx context.Context, inst *corev1.Instance, r backupdb.Record, inspectErr error) (*corev1.InstanceFiles, error) {
	now := o.now()
	var ctrlErr *corev1.Error
	isOutcomeUnknown := errors.As(inspectErr, &ctrlErr) && ctrlErr.Code == corev1.OutcomeUnknown
	fence, hasFence := directUploadFenceDeadline(r.UploadExpiresAt)
	if r.Phase == backupdb.Uploading && hasFence && now.Before(fence) && isOutcomeUnknown {
		return backupResponse(inst, r), nil
	}
	if errors.Is(inspectErr, errInvalidBackupObservation) || (isOutcomeUnknown && (r.Phase == backupdb.Unknown || !hasFence || !now.Before(fence))) {
		o.recordBackupRecordFailure(ctx, inst, r, "bak-upload", inspectErr)
		if closeErr := o.closeBackup(ctx, r, true); closeErr != nil {
			return backupResponse(inst, r), closeErr
		}
		return backupResponse(inst, r), nil
	}
	o.recordBackupRecordFailure(ctx, inst, r, "bak-upload", inspectErr)
	return backupResponse(inst, r), inspectErr
}

func (o *operator) finishRecoveredBackupTaskLog(ctx context.Context, inst *corev1.Instance, r backupdb.Record, sizeBytes int64) {
	taskID := backupTaskID(r.Command.OperationID)
	if r.Command.DirectUpload {
		AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, fmt.Sprintf("上传校验完成（%s），正在解析归档目录与游戏配置...", formatHumanBytes(sizeBytes)))
		return
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID, fmt.Sprintf("备份已完成（%s）", formatHumanBytes(sizeBytes)))
	FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, taskID)
}

func (o *operator) settleBackupQuota(ctx context.Context, r backupdb.Record, actualBytes int64) (bool, error) {
	if o.quotaDB == nil || actualBytes < 0 || actualBytes >= r.Command.MaxBytes {
		return false, nil
	}
	h, err := o.quotaDB.GetAllocation(ctx, r.Command.AllocationID)
	if err != nil {
		return false, err
	}
	if h.Allocation.State != corev1.AllocationActive || h.Allocation.Current.BackupStorageBytes <= actualBytes {
		return false, nil
	}
	plan := pgutil.ResourceChange{
		ChangeID:     "quota:settle:" + r.Command.OperationID,
		AllocationID: r.Command.AllocationID,
		Owner:        r.Command.Owner,
		Binding:      r.Command.Binding,
		FromRevision: 1,
		ToRevision:   2,
		Old:          corev1.Resources{BackupsCount: 1, BackupStorageBytes: r.Command.MaxBytes},
		Target:       corev1.Resources{BackupsCount: 1, BackupStorageBytes: actualBytes},
	}
	change, err := o.quotaDB.RegisterChange(ctx, plan)
	if err != nil {
		return false, err
	}
	cmd := pgutil.Command{
		OperationID:      "quota:SettleDecrease:" + r.Command.OperationID,
		Domain:           corev1.QuotaDomain,
		AccountID:        r.Command.Owner.TenantUID,
		AllocationID:     r.Command.AllocationID,
		Owner:            r.Command.Owner,
		Kind:             corev1.SettleDecrease,
		Quantity:         corev1.Resources{BackupStorageBytes: r.Command.MaxBytes - actualBytes},
		ExpectedVersion:  change.SourceVersion,
		ResourceRevision: 2,
		ChangePlanID:     plan.ChangeID,
	}
	if _, err := o.quotaDB.RegisterCommand(ctx, cmd); err != nil {
		return false, err
	}
	if _, err := o.quotaDB.SettleDecrease(ctx, cmd.OperationID); err != nil {
		return false, err
	}
	return true, nil
}

func (o *operator) retryBackupTransfer(ctx context.Context, inst *corev1.Instance, r backupdb.Record) (*backupdb.Result, error) {
	current, err := o.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return nil, err
	}
	claims, err := o.fileExecutionClaims(ctx, current)
	if err != nil {
		return nil, err
	}
	if current.Metadata.UID != r.Command.Owner.InstanceUID || claims.NodeUID != r.Command.Binding.NodeUID || claims.AttemptID != r.Command.Binding.AttemptID || claims.Epoch != r.Command.Binding.PlacementEpoch || claims.ApprovedRevision != r.Command.ApprovedRevision {
		return nil, db.ErrConflict
	}
	cli, err := o.backupS3Client(r)
	if err != nil {
		return nil, err
	}
	presigned, _, err := cli.PresignPut(ctx, r.Command.ObjectKey)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(30 * time.Minute)
	if r.UploadExpiresAt != nil && r.UploadExpiresAt.After(time.Now().UTC()) {
		expires = r.UploadExpiresAt.UTC()
	}
	input := &corev1.InstanceFiles{
		Action: corev1.FileActionBackup, OperationID: r.Command.OperationID, Path: r.Command.ObjectKey,
		PresignedURL: presigned, MaxBytes: r.Command.MaxBytes, RateLimitBytesPerSec: r.Command.RateLimit,
	}
	taskID := backupTaskID(r.Command.OperationID)
	onEvent := o.newTransferEventHandler(ctx, current, r.Command.OperationID, taskID, false)
	res, err := o.transferFilesStreaming(ctx, current, input, expires, onEvent)
	if err != nil {
		return nil, err
	}
	completedAt := time.Now().UTC()
	if res.CompletedAt != nil {
		completedAt = res.CompletedAt.UTC()
	}
	return &backupdb.Result{SizeBytes: res.SizeBytes, SHA256: res.SHA256, CompletedAt: completedAt}, nil
}

func (o *operator) closeBackup(ctx context.Context, r backupdb.Record, project bool) error {
	if r.Phase == backupdb.Deleted {
		if project {
			return o.projectBackup(ctx, r)
		}
		return nil
	}
	if r.Phase != backupdb.Deleting {
		var err error
		r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Deleting})
		if err != nil {
			return backupConflict(err)
		}
	}
	if project {
		if err := o.projectBackup(ctx, r); err != nil {
			return err
		}
	}
	if _, err := o.quotaDB.RequestClose(ctx, backupQuotaCommand(r, corev1.RequestClose)); err != nil {
		return err
	}
	var err error
	if r, err = o.cleanupUploadedBackupObject(ctx, r); err != nil {
		return err
	}
	if _, err := o.quotaDB.FinalizeClose(ctx, backupQuotaCommand(r, corev1.FinalizeClose)); err != nil {
		return err
	}
	r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Deleted})
	if err != nil {
		return backupConflict(err)
	}
	if project {
		o.publishTenantReconcile(ctx, r.Command.Tenant)
		return o.projectBackup(ctx, r)
	}
	return nil
}

func (o *operator) cleanupUploadedBackupObject(ctx context.Context, r backupdb.Record) (backupdb.Record, error) {
	now := o.now()
	if r.Command.DirectUpload {
		return o.cleanupDirectUploadObjects(ctx, r, now)
	}
	if r.UploadExpiresAt != nil && r.Result == nil && now.Before(r.UploadExpiresAt.UTC()) {
		observed, err := o.inspectBackup(ctx, r)
		if err != nil {
			return r, err
		}
		r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Deleting, Result: observed})
		if err != nil {
			return r, backupConflict(err)
		}
	}
	if r.UploadExpiresAt != nil || r.Result != nil {
		if _, err := o.backupS3Client(r); err != nil {
			return r, err
		}
		if err := o.deleteBackupObject(ctx, r.Command.ObjectKey); err != nil {
			return r, fmt.Errorf("%w: backup cleanup failed: %v", models.ErrBadGateway, err)
		}
	}
	return r, nil
}

func (o *operator) cleanupDirectUploadObjects(ctx context.Context, r backupdb.Record, now time.Time) (backupdb.Record, error) {
	if r.UploadExpiresAt != nil || r.Result != nil {
		if _, err := o.backupS3Client(r); err != nil {
			return r, err
		}
		if err := o.deleteBackupObject(ctx, directUploadStagingKey(r.Command.OperationID)); err != nil {
			return r, fmt.Errorf("%w: backup staging cleanup failed: %v", models.ErrBadGateway, err)
		}
		if err := o.deleteBackupObject(ctx, r.Command.ObjectKey); err != nil {
			return r, fmt.Errorf("%w: backup cleanup failed: %v", models.ErrBadGateway, err)
		}
	}
	if fence, hasFence := directUploadFenceDeadline(r.UploadExpiresAt); hasFence && now.Before(fence) {
		return r, &corev1.Error{
			Code:        corev1.OutcomeUnknown,
			OperationID: r.Command.OperationID,
			Detail:      "direct upload presigned URL has not expired yet; quota refund is deferred",
		}
	}
	return r, nil
}

func isRetentionBackupPhase(phase string) bool {
	return phase == "" || phase == string(backupdb.Completed) || phase == corev1.BackupPhaseInspectFailed || phase == string(backupdb.Deleting)
}

func (o *operator) makeBackupRoom(ctx context.Context, inst *corev1.Instance, r backupdb.Record) error {
	if r.Phase != backupdb.PendingQuota {
		return nil
	}
	count := effectiveBackupRetentionCount(inst)
	if count <= 0 {
		return nil
	}
	current, err := o.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return err
	}
	if current.Metadata.UID != inst.Metadata.UID || current.Metadata.DeletionTimestamp != nil {
		return db.ErrConflict
	}
	kept := 0
	for _, b := range current.Status.Backups {
		if b.UID == r.Command.BackupUID || !isRetentionBackupPhase(b.Phase) {
			continue
		}
		kept++
		if kept < count {
			continue
		}
		if err := o.deleteRecordedBackup(ctx, current, b.Path); err != nil {
			return err
		}
	}
	return nil
}

func (o *operator) enforceBackupRetention(ctx context.Context, inst *corev1.Instance, newestUID string) error {
	count := effectiveBackupRetentionCount(inst)
	if count <= 0 {
		return nil
	}
	current, err := o.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil {
		return err
	}
	if current.Metadata.UID != inst.Metadata.UID {
		return db.ErrConflict
	}
	kept := 0
	for _, b := range current.Status.Backups {
		if b.UID == newestUID {
			kept++
			continue
		}
		if !isRetentionBackupPhase(b.Phase) {
			continue
		}
		kept++
		if kept <= count {
			continue
		}
		if err := o.deleteRecordedBackup(ctx, current, b.Path); err != nil {
			return err
		}
	}
	return nil
}

func (o *operator) deleteRecordedBackup(ctx context.Context, inst *corev1.Instance, key string) error {
	if o.backupDB == nil {
		return fmt.Errorf("backup command store is required")
	}
	r, err := o.backupDB.GetByKey(ctx, inst.Metadata.UID, key)
	if err == nil {
		return o.closeBackup(ctx, r, true)
	}
	if !errors.Is(err, backupdb.ErrNotFound) {
		return err
	}
	if err := o.markBackupDeleting(ctx, inst, key); err != nil {
		return err
	}
	if err := o.deleteBackupObject(ctx, key); err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	return o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		var out []corev1.InstanceBackupRecord
		for _, b := range items {
			if b.Path != key {
				out = append(out, b)
			}
		}
		return out
	})
}
