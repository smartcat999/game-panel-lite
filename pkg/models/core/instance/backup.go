package instance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

const (
	backupReconcileLockTTL    = 10 * time.Minute
	directUploadStagingPrefix = "staging/backups/"
	minDirectUploadTTL        = 15 * time.Second
	directUploadBytesPerSec   = 256 << 10
)

func directUploadStagingKey(opID string) string {
	return directUploadStagingPrefix + strings.TrimSpace(opID)
}

type clientHostKey struct{}

// WithClientHost attaches the caller's browser/client hostname so direct S3 presigned URLs
// and node-agent directURLs resolve to a host reachable by the client.
func WithClientHost(ctx context.Context, host string) context.Context {
	h := strings.TrimSpace(host)
	if spHost, _, err := net.SplitHostPort(h); err == nil && spHost != "" {
		h = spHost
	}
	if h == "" {
		return ctx
	}
	return context.WithValue(ctx, clientHostKey{}, h)
}

func clientHostFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(clientHostKey{}).(string)
	return v
}

func backupPrefix(tenant, name string) string {
	return fmt.Sprintf("tenants/%s/instances/%s/", tenant, name)
}

func resolveNewBackupKey(tenant, name, raw string, now time.Time) (string, error) {
	prefix := backupPrefix(tenant, name)
	p := strings.TrimLeft(strings.TrimSpace(raw), "/")
	if p == "" || p == "." {
		return prefix + "backup-" + now.UTC().Format("20060102-150405") + ".tar.gz", nil
	}
	base := path.Base(p)
	if base == "" || base == "." || base == ".." || base == "/" || strings.ContainsAny(base, "\\\x00") {
		return "", fmt.Errorf("%w: invalid backup name %q", models.ErrBadRequest, raw)
	}
	return prefix + base, nil
}

func resolveExistingBackupKey(inst *corev1.Instance, raw string) (string, error) {
	key := strings.TrimLeft(strings.TrimSpace(raw), "/")
	if key == "" || key == "." {
		return "", fmt.Errorf("%w: backup object path is required", models.ErrBadRequest)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: invalid backup object path %q", models.ErrBadRequest, raw)
		}
	}
	if strings.HasPrefix(key, backupPrefix(inst.Metadata.Tenant, inst.Metadata.Name)) {
		return key, nil
	}
	for _, b := range inst.Status.Backups {
		if strings.TrimLeft(strings.TrimSpace(b.Path), "/") == key {
			return key, nil
		}
	}
	return "", fmt.Errorf("%w: backup %q does not belong to instance %s/%s", models.ErrForbidden, raw, inst.Metadata.Tenant, inst.Metadata.Name)
}

func resolveNodePublicOrInternalIP(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	if ip := strings.TrimSpace(node.Spec.ExternalIP); ip != "" {
		return ip
	}
	for _, kind := range []string{corev1.NodeAddressExternalIP, corev1.NodeAddressInternalIP} {
		for _, addr := range node.Status.Addresses {
			if addr.Type == kind && strings.TrimSpace(addr.Address) != "" {
				return strings.TrimSpace(addr.Address)
			}
		}
	}
	return ""
}

func (o *operator) presignDirectBackupDownload(ctx context.Context, inst *corev1.Instance, key string) (string, time.Time, error) {
	targetIP := o.resolveClientOrNodeIP(ctx, inst)
	attachName := path.Base(key)
	if !hasSupportedArchiveExtension(attachName) {
		attachName += ".tar.gz"
	}
	return o.s3Client().PresignGet(ctx, key, s3.PresignOptions{
		PublicHostOverride:         targetIP,
		ResponseContentDisposition: "attachment; filename=" + strconv.Quote(attachName),
	})
}

func hasSupportedArchiveExtension(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".zip")
}

func (o *operator) directUploadTTL(reserveBytes int64) time.Duration {
	maxTTL := 15 * time.Minute
	if cfg := config.Get(); cfg != nil && cfg.S3.PresignTTL > 0 {
		maxTTL = cfg.S3.PresignTTL
	}
	needed := minDirectUploadTTL
	if reserveBytes > 0 {
		needed += time.Duration((reserveBytes+directUploadBytesPerSec-1)/directUploadBytesPerSec) * time.Second
	}
	if needed > maxTTL {
		needed = maxTTL
	}
	if needed < minDirectUploadTTL {
		needed = minDirectUploadTTL
	}
	return needed
}

func (o *operator) presignDirectBackupUpload(ctx context.Context, inst *corev1.Instance, key string, ttl time.Duration) (string, error) {
	targetIP := o.resolveClientOrNodeIP(ctx, inst)
	signedURL, _, err := o.s3Client().PresignPut(ctx, key, s3.PresignOptions{
		PublicHostOverride: targetIP,
		Expires:            ttl,
	})
	return signedURL, err
}

func (o *operator) resolveClientOrNodeIP(ctx context.Context, inst *corev1.Instance) string {
	// Without an explicit external S3 public endpoint, presign against the internal S3 host
	// so browsers upload/download backups through the web server's same-origin S3 proxy.
	if strings.TrimSpace(o.s3Client().Binding().PublicEndpoint) == "" {
		return ""
	}
	targetIP := clientHostFromContext(ctx)
	if targetIP == "" && inst != nil && strings.TrimSpace(inst.Spec.NodeName) != "" {
		if node, nErr := o.nodeDB.Get(ctx, strings.TrimSpace(inst.Spec.NodeName)); nErr == nil {
			targetIP = resolveNodePublicOrInternalIP(node)
		}
	}
	return targetIP
}

func (o *operator) deleteBackupObject(ctx context.Context, key string) error {
	cli := o.s3Client()
	if !cli.IsConfigured() {
		return errors.New("backup cleanup requires configured object storage")
	}
	if err := cli.Delete(ctx, key); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("delete backup %s: %w", key, err)
	}
	return nil
}

func (o *operator) publishTenantReconcile(ctx context.Context, tenant string) {
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenant, 0, ""))
	}
}

// requestBackupAction persists a declarative backup command (`PendingQuota`), projects it into
// status.backups, and wakes up the instance reconciler via MQ without blocking on S3 or node-agent.
func (o *operator) requestBackupAction(ctx context.Context, existing *corev1.Instance, spec corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	if strings.TrimSpace(spec.PresignedURL) != "" {
		return nil, fmt.Errorf("%w: presignedURL is not accepted; backups are stored in platform object storage", models.ErrBadRequest)
	}
	if !o.s3Client().IsConfigured() {
		return nil, fmt.Errorf("%w: platform object storage (s3) is not configured", models.ErrBadRequest)
	}
	r, err := o.prepareBackup(ctx, existing, spec, maxBytes)
	if err != nil {
		return nil, backupConflict(err)
	}
	if r.Phase == backupdb.Deleting || r.Phase == backupdb.Deleted {
		return backupResponse(existing, r), fmt.Errorf("%w: backup admission gate is closed", models.ErrConflict)
	}
	if err := o.projectBackup(ctx, r); err != nil {
		return backupResponse(existing, r), err
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, existing.Metadata.UID, backupTaskID(r.Command.OperationID), "已创建备份任务，等待配额校验与节点调度...")
	o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
	return backupResponse(existing, r), nil
}

func (o *operator) handleUploadBackupAction(ctx context.Context, existing *corev1.Instance, spec corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	if !o.s3Client().IsConfigured() {
		return nil, fmt.Errorf("%w: platform object storage (s3) is not configured", models.ErrBadRequest)
	}
	reserveBytes := maxBytes
	if spec.SizeBytes > 0 {
		if spec.SizeBytes > maxBytes {
			return nil, fmt.Errorf("%w: archive size (%d bytes) exceeds maximum allowed backup size (%d bytes)", models.ErrBadRequest, spec.SizeBytes, maxBytes)
		}
		reserveBytes = spec.SizeBytes
	}
	r, err := o.prepareBackup(ctx, existing, spec, reserveBytes)
	if err != nil {
		return nil, backupConflict(err)
	}
	if !r.Command.DirectUpload {
		return nil, fmt.Errorf("%w: operation %s is not a direct backup upload", models.ErrConflict, r.Command.OperationID)
	}
	res, err := o.issueOrReuseDirectBackupUpload(ctx, existing, r)
	if err != nil {
		o.abortUnissuedDirectUpload(ctx, existing, r.Command.OperationID)
		return nil, err
	}
	return res, nil
}

func (o *operator) abortUnissuedDirectUpload(ctx context.Context, inst *corev1.Instance, opID string) {
	if o == nil || o.backupDB == nil || strings.TrimSpace(opID) == "" {
		return
	}
	cur, err := o.backupDB.Get(ctx, opID)
	if err != nil || !cur.Command.DirectUpload || cur.UploadExpiresAt != nil {
		return
	}
	if cur.Phase != backupdb.PendingQuota && cur.Phase != backupdb.Ready {
		return
	}
	if err := o.closeBackup(ctx, cur, false); err == nil && inst != nil {
		o.publishTenantReconcile(ctx, inst.Metadata.Tenant)
	}
}

func (o *operator) issueOrReuseDirectBackupUpload(ctx context.Context, existing *corev1.Instance, r backupdb.Record) (*corev1.InstanceFiles, error) {
	stagingKey := directUploadStagingKey(r.Command.OperationID)
	for range 3 {
		if r.Phase == backupdb.PendingQuota {
			next, err := o.fundBackup(ctx, existing, r)
			if errors.Is(err, backupdb.ErrConflict) {
				if r, err = o.backupDB.Get(ctx, r.Command.OperationID); err != nil {
					return nil, err
				}
				continue
			}
			if err != nil {
				return nil, backupConflict(err)
			}
			r = next
		}
		switch r.Phase {
		case backupdb.Ready:
			ttl := o.directUploadTTL(r.Command.MaxBytes)
			exp := o.now().Add(ttl)
			directURL, err := o.presignDirectBackupUpload(ctx, existing, stagingKey, ttl)
			if err != nil {
				return nil, fmt.Errorf("%w: presign s3 upload url: %v", models.ErrBadRequest, err)
			}
			next, err := o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Uploading, UploadExpiresAt: &exp})
			if errors.Is(err, backupdb.ErrConflict) {
				if r, err = o.backupDB.Get(ctx, r.Command.OperationID); err != nil {
					return nil, err
				}
				continue
			}
			if err != nil {
				return nil, backupConflict(err)
			}
			return o.finishDirectUploadURL(ctx, existing, next, directURL, exp)
		case backupdb.Uploading:
			return o.reuseUploadingDirectBackupURL(ctx, existing, r, stagingKey)
		default:
			return nil, fmt.Errorf("%w: backup upload URL cannot be issued in phase %s", models.ErrConflict, r.Phase)
		}
	}
	return nil, fmt.Errorf("%w: backup upload state changed repeatedly", models.ErrConflict)
}

func (o *operator) reuseUploadingDirectBackupURL(ctx context.Context, existing *corev1.Instance, r backupdb.Record, stagingKey string) (*corev1.InstanceFiles, error) {
	if r.UploadExpiresAt == nil {
		return nil, fmt.Errorf("%w: backup upload deadline is missing", models.ErrConflict)
	}
	frozenExp := r.UploadExpiresAt.UTC()
	remaining := frozenExp.Sub(o.now())
	if remaining <= time.Second {
		return nil, fmt.Errorf("%w: backup upload window has expired", models.ErrConflict)
	}
	directURL, err := o.presignDirectBackupUpload(ctx, existing, stagingKey, remaining)
	if err != nil {
		return nil, fmt.Errorf("%w: presign s3 upload url: %v", models.ErrBadRequest, err)
	}
	return o.finishDirectUploadURL(ctx, existing, r, directURL, frozenExp)
}

func (o *operator) finishDirectUploadURL(ctx context.Context, existing *corev1.Instance, r backupdb.Record, directURL string, exp time.Time) (*corev1.InstanceFiles, error) {
	if err := o.projectBackup(ctx, r); err != nil {
		return nil, err
	}
	out := backupResponse(existing, r)
	out.Action = corev1.FileActionUploadBackup
	out.DirectURL = directURL
	out.ExpiresAt = &exp
	return out, nil
}

func (o *operator) handleCompleteUploadBackupAction(ctx context.Context, existing *corev1.Instance, spec corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	for range 6 {
		r, found, err := o.lookupExistingBackupCommand(ctx, existing, spec)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: backup upload operation not found", models.ErrNotFound)
		}
		switch r.Phase {
		case backupdb.Completed:
			out := backupResponse(existing, r)
			out.Action = corev1.FileActionCompleteUploadBackup
			return out, nil
		case backupdb.Uploading:
			r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Unknown})
			if errors.Is(err, backupdb.ErrConflict) {
				continue
			}
			if err != nil {
				return nil, backupConflict(err)
			}
			if err := o.projectBackup(ctx, r); err != nil {
				return nil, err
			}
			AppendInstanceTaskLog(ctx, o.taskLogs, existing.Metadata.UID, backupTaskID(r.Command.OperationID), "客户端上传已提交，等待后台校验与配额结算...")
		case backupdb.Unknown:
		default:
			return nil, fmt.Errorf("%w: backup upload cannot be completed from phase %s", models.ErrConflict, r.Phase)
		}
		o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
		out := backupResponse(existing, r)
		out.Action = corev1.FileActionCompleteUploadBackup
		return out, nil
	}
	return nil, fmt.Errorf("%w: backup upload state changed repeatedly", models.ErrConflict)
}

// requestDeleteBackupAction transitions the backup record to `Deleting`, projects it into
// status.backups, and wakes up the instance reconciler via MQ to perform S3 cleanup and quota refund.
func (o *operator) requestDeleteBackupAction(ctx context.Context, existing *corev1.Instance, spec corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	key, err := resolveExistingBackupKey(existing, spec.Path)
	if err != nil {
		return nil, err
	}
	if err := o.markBackupDeleting(ctx, existing, key); err != nil {
		return nil, err
	}
	o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
	return &corev1.InstanceFiles{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata: existing.Metadata,
		Action:   spec.Action,
		Path:     key,
		Phase:    string(backupdb.Deleting),
	}, nil
}

func (o *operator) markBackupDeleting(ctx context.Context, inst *corev1.Instance, key string) error {
	if o.backupDB == nil {
		return fmt.Errorf("backup command store is required")
	}
	for _, b := range inst.Status.Backups {
		if b.Path == key && b.Phase == corev1.BackupPhaseRestoring {
			return fmt.Errorf("%w: cannot delete backup while restore is in progress", models.ErrConflict)
		}
	}
	r, err := o.backupDB.GetByKey(ctx, inst.Metadata.UID, key)
	if err == nil {
		if r.Phase != backupdb.Deleted && r.Phase != backupdb.Deleting {
			r, err = o.backupDB.Transition(ctx, r.Command.OperationID, r.Version, backupdb.Update{Phase: backupdb.Deleting})
			if err != nil {
				return backupConflict(err)
			}
		}
		return o.projectBackup(ctx, r)
	}
	if !errors.Is(err, backupdb.ErrNotFound) {
		return err
	}
	found := false
	for _, b := range inst.Status.Backups {
		if b.Path == key {
			if b.AllocationID != "" || b.OperationID != "" || b.UID != "" {
				return fmt.Errorf("%w: recorded backup command is missing", models.ErrConflict)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%w: backup object is not recorded", models.ErrNotFound)
	}
	return o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for n := range items {
			if items[n].Path == key {
				items[n].Phase = string(backupdb.Deleting)
			}
		}
		return items
	})
}

func (o *operator) handleDownloadBackupAction(ctx context.Context, existing *corev1.Instance, input *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	if input.PresignedURL != "" {
		return nil, fmt.Errorf("%w: presignedURL is not accepted", models.ErrBadRequest)
	}
	key, err := o.readableBackupKey(ctx, existing, input.Path)
	if err != nil {
		return nil, err
	}
	if !o.s3Client().IsConfigured() {
		return nil, fmt.Errorf("%w: platform object storage (s3) is not configured", models.ErrBadRequest)
	}
	directURL, exp, err := o.presignDirectBackupDownload(ctx, existing, key)
	if err != nil {
		return nil, fmt.Errorf("%w: presign s3 download url: %v", models.ErrBadRequest, err)
	}
	return &corev1.InstanceFiles{
		TypeMeta:  metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:  existing.Metadata,
		Action:    input.Action,
		Path:      key,
		DirectURL: directURL,
		ExpiresAt: &exp,
	}, nil
}

// requestRestoreBackupAction validates the backup key, mapping choices, and active binding, marks the backup phase
// as `Restoring` in status.backups, and wakes up the instance reconciler via MQ.
func (o *operator) requestRestoreBackupAction(ctx context.Context, existing *corev1.Instance, input *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	key, sourceRel, targetRel, replaceTarget, err := o.validateRestoreBackupRequest(ctx, existing, input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	taskID := restoreTaskID(key, now)
	if err := o.mutateBackups(ctx, existing.Metadata.Tenant, existing.Metadata.Name, existing.Metadata.UID, func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for n := range items {
			if items[n].Path == key {
				applyRestoreRecordStart(&items[n], taskID, sourceRel, targetRel, replaceTarget, now)
			}
		}
		return items
	}); err != nil {
		return nil, err
	}
	AppendInstanceTaskLog(ctx, o.taskLogs, existing.Metadata.UID, taskID, "已发起存档恢复任务，准备连接服务器所在节点...")
	o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
	return &corev1.InstanceFiles{
		TypeMeta:      metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:      existing.Metadata,
		Action:        corev1.FileActionRestore,
		Path:          key,
		SourcePath:    sourceRel,
		TargetPath:    targetRel,
		ReplaceTarget: &replaceTarget,
		Phase:         corev1.BackupPhaseRestoring,
	}, nil
}

func (o *operator) validateRestoreBackupRequest(ctx context.Context, existing *corev1.Instance, input *corev1.InstanceFiles) (string, string, string, bool, error) {
	if input.PresignedURL != "" {
		return "", "", "", false, fmt.Errorf("%w: presignedURL is not accepted", models.ErrBadRequest)
	}
	key, err := o.readableBackupKey(ctx, existing, input.Path)
	if err != nil {
		return "", "", "", false, err
	}
	if err := validateBackupRecordRestorable(existing, key); err != nil {
		return "", "", "", false, err
	}
	sourceRel, targetRel, replaceTarget, err := resolveRestoreMapping(existing, input.SourcePath, input.TargetPath, input.ReplaceTarget)
	if err != nil {
		return "", "", "", false, err
	}
	if !o.s3Client().IsConfigured() {
		return "", "", "", false, fmt.Errorf("%w: platform object storage (s3) is not configured", models.ErrBadRequest)
	}
	if _, err := o.fileExecutionClaims(ctx, existing); err != nil {
		return "", "", "", false, err
	}
	if err := o.ensureNoConflictingBackupForRestore(ctx, existing, key); err != nil {
		return "", "", "", false, err
	}
	return key, sourceRel, targetRel, replaceTarget, nil
}

func validateBackupRecordRestorable(inst *corev1.Instance, key string) error {
	rec, ok := findBackupRecordByKey(inst.Status.Backups, key)
	if !ok {
		return nil
	}
	switch rec.Phase {
	case corev1.BackupPhaseInspecting:
		return fmt.Errorf("%w: backup archive is still being inspected", models.ErrConflict)
	case corev1.BackupPhaseInspectFailed:
		return fmt.Errorf("%w: %s", models.ErrBadRequest, nonEmptyInspectMessage(rec.Message))
	}
	return nil
}

func applyRestoreRecordStart(rec *corev1.InstanceBackupRecord, taskID, sourceRel, targetRel string, replaceTarget bool, now time.Time) {
	rec.Phase = corev1.BackupPhaseRestoring
	rec.TaskID = taskID
	rec.Stage = corev1.BackupStageDownloading
	rec.Progress = 5
	rec.ProcessedBytes = 0
	rec.TotalBytes = rec.SizeBytes
	rec.SpeedBytesPerSec = 0
	rec.StartedAt = &now
	rec.Message = "准备恢复存档"
	rec.RestoreSourcePath = sourceRel
	rec.RestoreTargetPath = targetRel
	rpl := replaceTarget
	rec.RestoreReplace = &rpl
	rec.LastRestoreTaskID = taskID
	rec.LastRestoreResult = corev1.RestoreResultRunning
	rec.LastRestoreMessage = ""
}

func (o *operator) ensureNoConflictingBackupForRestore(ctx context.Context, inst *corev1.Instance, restoreKey string) error {
	for _, b := range inst.Status.Backups {
		if b.Phase == corev1.BackupPhaseRestoring && b.Path != restoreKey {
			return fmt.Errorf("%w: another backup restore is already in progress", models.ErrConflict)
		}
		if b.Phase == string(backupdb.PendingQuota) || b.Phase == string(backupdb.Ready) || b.Phase == string(backupdb.Uploading) || b.Phase == string(backupdb.Unknown) {
			return fmt.Errorf("%w: cannot restore while a backup archive is in progress", models.ErrConflict)
		}
	}
	if o.backupDB == nil {
		return nil
	}
	records, err := o.backupDB.ListInstance(ctx, inst.Metadata.UID)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Phase == backupdb.PendingQuota || r.Phase == backupdb.Ready || r.Phase == backupdb.Uploading || r.Phase == backupdb.Unknown {
			return fmt.Errorf("%w: cannot restore while a backup archive is in progress", models.ErrConflict)
		}
	}
	return nil
}

func (o *operator) readableBackupKey(ctx context.Context, inst *corev1.Instance, raw string) (string, error) {
	key, err := resolveExistingBackupKey(inst, raw)
	if err != nil {
		return "", err
	}
	if o.backupDB == nil {
		return "", fmt.Errorf("backup command store is required")
	}
	if r, err := o.backupDB.GetByKey(ctx, inst.Metadata.UID, key); err == nil {
		if r.Phase != backupdb.Completed {
			return "", fmt.Errorf("%w: backup transfer is not complete", models.ErrConflict)
		}
		if _, err := o.backupS3Client(r); err != nil {
			return "", err
		}
		return key, nil
	} else if !errors.Is(err, backupdb.ErrNotFound) {
		return "", err
	}
	for _, b := range inst.Status.Backups {
		if b.Path == key && b.OperationID == "" && b.AllocationID == "" && b.UID == "" &&
			(b.Phase == "" || b.Phase == string(backupdb.Completed) || b.Phase == corev1.BackupPhaseRestoring ||
				b.Phase == corev1.BackupPhaseInspecting || b.Phase == corev1.BackupPhaseInspectFailed) {
			return key, nil
		}
	}
	return "", fmt.Errorf("%w: backup object is not recorded", models.ErrNotFound)
}

func (o *operator) mutateBackups(ctx context.Context, tenant, name, expectedUID string, fn func([]corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord) error {
	if expectedUID == "" {
		return db.ErrConflict
	}
	for range 8 {
		cur, err := o.db.Get(ctx, tenant, name)
		if err != nil {
			return err
		}
		if cur.Metadata.UID != expectedUID {
			return db.ErrConflict
		}
		previous := append([]corev1.InstanceBackupRecord(nil), cur.Status.Backups...)
		next := fn(previous)
		if cur.Metadata.DeletionTimestamp != nil && len(next) > len(cur.Status.Backups) {
			return db.ErrConflict
		}
		if reflect.DeepEqual(cur.Status.Backups, next) {
			return nil
		}
		updated, err := o.db.UpdateBackups(ctx, tenant, name, cur.Metadata.UID, cur.Metadata.StatusVersion, next)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		if err == nil {
			o.publishBackupStatusEvent(ctx, updated)
		}
		return err
	}
	return fmt.Errorf("%w: backup status changed repeatedly", db.ErrConflict)
}

func (o *operator) publishBackupStatusEvent(ctx context.Context, inst *corev1.Instance) {
	if o.mq == nil || inst == nil {
		return
	}
	ev := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion)
	ev.Attributes = map[string]string{"statusOnly": "true", "prevPhase": string(inst.Status.Phase), "hasUsage": "false"}
	_ = o.mq.Publish(ctx, mq.TopicInstance, ev)
}

func backupConflict(err error) error {
	if errors.Is(err, backupdb.ErrConflict) || errors.Is(err, corev1.AdmissionDenied) || errors.Is(err, corev1.Closed) || errors.Is(err, corev1.IdentityConflict) || errors.Is(err, corev1.RevisionChanged) {
		return fmt.Errorf("%w: %v", models.ErrConflict, err)
	}
	return err
}

func (o *operator) backupS3Client(r backupdb.Record) (s3.Interface, error) {
	cli := o.s3Client()
	if !cli.IsConfigured() {
		return nil, fmt.Errorf("%w: object storage is not configured", models.ErrBadRequest)
	}
	if cli.Binding() != r.Command.Storage {
		return nil, fmt.Errorf("%w: original backup storage target has changed", models.ErrConflict)
	}
	return cli, nil
}

func backupID(uid, request string) string {
	sum := sha256.Sum256([]byte(uid + "\n" + request))
	return "backup:" + hex.EncodeToString(sum[:])
}

func (o *operator) validateAdmittingBackupTenant(ctx context.Context, inst *corev1.Instance) (*corev1.Tenant, corev1.Limits, error) {
	if o.backupDB == nil || o.quotaDB == nil || inst.Status.ResourceControl == nil {
		return nil, corev1.Limits{}, fmt.Errorf("%w: backup resource stores and instance identity are required", models.ErrConflict)
	}
	tenant, err := o.tenantDB.Get(ctx, inst.Metadata.Tenant)
	if err != nil {
		return nil, corev1.Limits{}, err
	}
	if tenant.Metadata.UID != inst.Status.ResourceControl.TenantUID || tenant.Metadata.DeletionTimestamp != nil || tenant.Status.Phase == corev1.TenantPhaseSuspended || tenant.Status.Phase == corev1.TenantPhaseTerminating {
		return nil, corev1.Limits{}, fmt.Errorf("%w: tenant is not admitting backups", models.ErrConflict)
	}
	limits, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota)
	if err != nil {
		return nil, corev1.Limits{}, err
	}
	return tenant, limits, nil
}

func (o *operator) wakeTenantPolicySync(ctx context.Context, tenantName string) {
	if o.mq == nil || tenantName == "" {
		return
	}
	wake := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenantName, 0, "")
	wake.Topic = mq.TopicTenant
	_ = o.mq.Publish(ctx, mq.TopicTenant, wake)
}

func (o *operator) readBackupQuotaForAPI(ctx context.Context, inst *corev1.Instance) (quotadb.Account, error) {
	tenant, limits, err := o.validateAdmittingBackupTenant(ctx, inst)
	if err != nil {
		return quotadb.Account{}, err
	}
	acct, err := o.quotaDB.GetAccount(ctx, tenant.Metadata.UID)
	if errors.Is(err, quotadb.ErrNotFound) || (err == nil && acct.PolicyVersion == 0) {
		o.wakeTenantPolicySync(ctx, tenant.Metadata.Name)
		return quotadb.Account{TenantUID: tenant.Metadata.UID, Limits: limits}, nil
	}
	if err != nil {
		return quotadb.Account{}, err
	}
	if rv, rvErr := version(tenant.Metadata.ResourceVersion); rvErr == nil && acct.UpstreamVersion < rv {
		o.wakeTenantPolicySync(ctx, tenant.Metadata.Name)
	}
	return acct, nil
}

func (o *operator) requireBackupQuota(ctx context.Context, inst *corev1.Instance) (quotadb.Account, error) {
	tenant, limits, err := o.validateAdmittingBackupTenant(ctx, inst)
	if err != nil {
		return quotadb.Account{}, err
	}
	rv, err := version(tenant.Metadata.ResourceVersion)
	if err != nil {
		return quotadb.Account{}, err
	}
	acct, err := o.quotaDB.RequireAccountAt(ctx, tenant.Metadata.UID, rv, quotadb.QuotaPolicy{Limits: limits})
	if errors.Is(err, quotadb.ErrPolicySyncPending) {
		o.wakeTenantPolicySync(ctx, tenant.Metadata.Name)
	}
	return acct, err
}

func (o *operator) prepareBackup(ctx context.Context, inst *corev1.Instance, spec corev1.InstanceFiles, maxBytes int64) (backupdb.Record, error) {
	if o.backupDB == nil {
		return backupdb.Record{}, fmt.Errorf("backup command store is required")
	}
	if r, found, err := o.lookupExistingBackupCommand(ctx, inst, spec); err != nil || found {
		return r, err
	}
	key, err := resolveNewBackupKey(inst.Metadata.Tenant, inst.Metadata.Name, spec.Path, o.now())
	if err != nil {
		return backupdb.Record{}, err
	}
	requestPath := ""
	if strings.TrimSpace(spec.Path) != "" && strings.TrimSpace(spec.Path) != "." {
		requestPath = path.Base(key)
	}
	requestID := strings.TrimSpace(spec.OperationID)
	if requestID == "" {
		if requestPath != "" {
			requestID = "path:" + requestPath
		} else {
			requestID = "request:" + db.NewUID()
		}
	}
	id := backupID(inst.Metadata.UID, requestID)
	if r, err := o.backupDB.Get(ctx, id); err == nil {
		return r, matchBackupRequest(inst, spec, r)
	} else if !errors.Is(err, backupdb.ErrNotFound) {
		return r, err
	}
	return o.registerNewBackupCommand(ctx, inst, spec, id, requestPath, maxBytes)
}

func (o *operator) lookupExistingBackupCommand(ctx context.Context, inst *corev1.Instance, spec corev1.InstanceFiles) (backupdb.Record, bool, error) {
	requestID := strings.TrimSpace(spec.OperationID)
	if requestID == "" {
		key := strings.TrimLeft(strings.TrimSpace(spec.Path), "/")
		if key != "" && key != "." {
			if r, err := o.backupDB.GetByKey(ctx, inst.Metadata.UID, key); err == nil {
				return r, true, matchBackupRequest(inst, spec, r)
			} else if !errors.Is(err, backupdb.ErrNotFound) {
				return r, true, err
			}
			derivedID := backupID(inst.Metadata.UID, "path:"+path.Base(key))
			if r, err := o.backupDB.Get(ctx, derivedID); err == nil {
				return r, true, matchBackupRequest(inst, spec, r)
			} else if !errors.Is(err, backupdb.ErrNotFound) {
				return r, true, err
			}
		}
		return backupdb.Record{}, false, nil
	}
	if err := corev1.ValidateIdentifier("operationID", requestID); err != nil {
		return backupdb.Record{}, true, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if r, err := o.backupDB.Get(ctx, requestID); err == nil {
		return r, true, matchBackupRequest(inst, spec, r)
	} else if !errors.Is(err, backupdb.ErrNotFound) {
		return r, true, err
	}
	derivedID := backupID(inst.Metadata.UID, requestID)
	if derivedID != requestID {
		if r, err := o.backupDB.Get(ctx, derivedID); err == nil {
			return r, true, matchBackupRequest(inst, spec, r)
		} else if !errors.Is(err, backupdb.ErrNotFound) {
			return r, true, err
		}
	}
	return backupdb.Record{}, false, nil
}

func (o *operator) registerNewBackupCommand(ctx context.Context, inst *corev1.Instance, spec corev1.InstanceFiles, id, requestPath string, maxBytes int64) (backupdb.Record, error) {
	if err := o.ensureNoActiveBackupOrRestore(ctx, inst, id); err != nil {
		return backupdb.Record{}, err
	}
	claims, err := o.fileExecutionClaims(ctx, inst)
	if err != nil {
		return backupdb.Record{}, err
	}
	account, err := o.readBackupQuotaForAPI(ctx, inst)
	if err != nil {
		return backupdb.Record{}, err
	}
	if !account.Limits.BackupStorage.Unlimited {
		limit := account.Limits.BackupStorage.Value
		if limit == 0 {
			return backupdb.Record{}, fmt.Errorf("%w: tenant backup storage quota is zero", models.ErrConflict)
		}
		if spec.MaxBytes == 0 && spec.SizeBytes == 0 {
			if avail := limit - account.Allocated.BackupStorageBytes; avail > 0 && maxBytes > avail {
				maxBytes = avail
			}
			if maxBytes > limit {
				maxBytes = limit
			}
		}
	}
	base := requestPath
	if base == "" {
		base = "archive.tar.gz"
	}
	key := backupPrefix(inst.Metadata.Tenant, inst.Metadata.Name) + "backup-" + strings.TrimPrefix(id, "backup:") + "-" + base
	c := backupdb.Command{
		OperationID: id, BackupUID: id, AllocationID: "quota:" + id,
		Tenant: inst.Metadata.Tenant, Owner: owner(inst), ObjectKey: key, RequestPath: requestPath,
		MaxBytes: maxBytes, RequestedMaxBytes: spec.MaxBytes, RateLimit: spec.RateLimitBytesPerSec, NodeName: claims.NodeName,
		Binding:          pgutil.Binding{NodeUID: claims.NodeUID, AttemptID: claims.AttemptID, PlacementEpoch: claims.Epoch},
		ApprovedRevision: claims.ApprovedRevision,
		Storage:          o.s3Client().Binding(),
		DirectUpload:     spec.Action == corev1.FileActionUploadBackup,
	}
	return o.backupDB.Register(ctx, c)
}

func (o *operator) ensureNoActiveBackupOrRestore(ctx context.Context, inst *corev1.Instance, opID string) error {
	for _, b := range inst.Status.Backups {
		if b.Phase == corev1.BackupPhaseRestoring {
			return fmt.Errorf("%w: cannot start backup while a restore is in progress", models.ErrConflict)
		}
		if b.OperationID != opID && (b.Phase == string(backupdb.PendingQuota) || b.Phase == string(backupdb.Ready) || b.Phase == string(backupdb.Uploading)) {
			return fmt.Errorf("%w: another backup archive is already in progress", models.ErrConflict)
		}
	}
	records, err := o.backupDB.ListInstance(ctx, inst.Metadata.UID)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Command.OperationID != opID && (r.Phase == backupdb.PendingQuota || r.Phase == backupdb.Ready || r.Phase == backupdb.Uploading) {
			return fmt.Errorf("%w: another backup archive is already in progress", models.ErrConflict)
		}
	}
	return nil
}

func matchBackupRequest(inst *corev1.Instance, spec corev1.InstanceFiles, r backupdb.Record) error {
	c := r.Command
	if c.Owner.InstanceUID != inst.Metadata.UID || c.Tenant != inst.Metadata.Tenant || c.Owner.InstanceName != inst.Metadata.Name {
		return fmt.Errorf("%w: backup operation belongs to another instance", models.ErrForbidden)
	}
	if raw := strings.TrimSpace(spec.Path); raw != "" && raw != "." && raw != c.ObjectKey {
		key, err := resolveNewBackupKey(c.Tenant, c.Owner.InstanceName, raw, time.Time{})
		if err != nil || path.Base(key) != c.RequestPath {
			return fmt.Errorf("%w: backup operation path is immutable", models.ErrConflict)
		}
	}
	if spec.MaxBytes != c.RequestedMaxBytes && spec.MaxBytes != c.MaxBytes || spec.RateLimitBytesPerSec != c.RateLimit {
		return fmt.Errorf("%w: backup transfer budget is immutable", models.ErrConflict)
	}
	return nil
}

func backupQuotaCommand(r backupdb.Record, kind corev1.Kind) pgutil.Command {
	c := pgutil.Command{
		OperationID: "quota:" + string(kind) + ":" + r.Command.OperationID, Domain: corev1.QuotaDomain,
		AccountID: r.Command.Owner.TenantUID, AllocationID: r.Command.AllocationID, Owner: r.Command.Owner, Kind: kind,
	}
	if kind == corev1.Grant {
		c.Quantity = corev1.Resources{BackupsCount: 1, BackupStorageBytes: r.Command.MaxBytes}
		c.ExpectedVersion, c.ResourceRevision = 1, 1
	} else {
		c.CloseIntentID = "delete:" + r.Command.BackupUID
	}
	return c
}

func backupProjection(r backupdb.Record) corev1.InstanceBackupRecord {
	b := corev1.InstanceBackupRecord{
		UID: r.Command.BackupUID, OperationID: r.Command.OperationID, AllocationID: r.Command.AllocationID,
		TaskID: backupTaskID(r.Command.OperationID), Imported: r.Command.DirectUpload,
		Phase: string(r.Phase), ReservedBytes: r.Command.MaxBytes, Path: r.Command.ObjectKey,
	}
	if r.Result != nil {
		b.SizeBytes, b.SHA256 = r.Result.SizeBytes, r.Result.SHA256
		if r.Phase == backupdb.Completed {
			b.ReservedBytes = r.Result.SizeBytes
		}
		t := r.Result.CompletedAt
		b.CompletedAt = &t
	}
	return b
}

func (o *operator) projectBackup(ctx context.Context, r backupdb.Record) error {
	for range 8 {
		current, err := o.db.Get(ctx, r.Command.Tenant, r.Command.Owner.InstanceName)
		if err != nil {
			return err
		}
		if current.Metadata.UID != r.Command.Owner.InstanceUID {
			return db.ErrConflict
		}
		latest, err := o.backupDB.Get(ctx, r.Command.OperationID)
		if err != nil {
			return err
		}
		next, ok := buildProjectedBackupList(current, latest)
		if !ok {
			return db.ErrConflict
		}
		o.ensureNativeBackupSnapshot(ctx, current, latest, next)
		if reflect.DeepEqual(current.Status.Backups, next) {
			return nil
		}
		updated, err := o.db.UpdateBackups(ctx, r.Command.Tenant, r.Command.Owner.InstanceName, current.Metadata.UID, current.Metadata.StatusVersion, next)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		if err == nil {
			o.publishBackupStatusEvent(ctx, updated)
		}
		return err
	}
	return db.ErrConflict
}

func buildProjectedBackupList(current *corev1.Instance, latest backupdb.Record) ([]corev1.InstanceBackupRecord, bool) {
	b := backupProjection(latest)
	var next []corev1.InstanceBackupRecord
	found := false
	for _, old := range current.Status.Backups {
		if old.OperationID == b.OperationID || old.UID == b.UID || old.Path == b.Path {
			found = true
			if latest.Phase != backupdb.Deleted {
				merged := b
				oldCopy := old
				mergeBackupTelemetry(&oldCopy, &merged, latest.Phase)
				next = append(next, merged)
			}
		} else {
			next = append(next, old)
		}
	}
	if !found && latest.Phase != backupdb.Deleted {
		if isUnissuedDeletingDirectUpload(latest) {
			return current.Status.Backups, true
		}
		if current.Metadata.DeletionTimestamp != nil {
			return nil, false
		}
		mergeBackupTelemetry(nil, &b, latest.Phase)
		next = append([]corev1.InstanceBackupRecord{b}, next...)
	}
	return next, true
}

func isUnissuedDeletingDirectUpload(r backupdb.Record) bool {
	return r.Command.DirectUpload && r.Phase == backupdb.Deleting && r.UploadExpiresAt == nil && r.Result == nil
}

func (o *operator) ensureNativeBackupSnapshot(ctx context.Context, current *corev1.Instance, latest backupdb.Record, next []corev1.InstanceBackupRecord) {
	if current == nil || latest.Command.DirectUpload || latest.Phase == backupdb.Deleted {
		return
	}
	if latest.Phase != backupdb.PendingQuota && latest.Phase != backupdb.Ready && latest.Phase != backupdb.Uploading {
		return
	}
	for i := range next {
		if next[i].OperationID == latest.Command.OperationID && !next[i].Imported && next[i].Snapshot == nil {
			next[i].Snapshot = o.captureInstanceBackupSnapshot(ctx, current)
		}
	}
}

func (o *operator) captureInstanceBackupSnapshot(ctx context.Context, inst *corev1.Instance) *corev1.InstanceBackupSnapshot {
	if inst == nil {
		return nil
	}
	now := time.Now().UTC()
	appObj := o.resolveAppForInstance(ctx, inst)
	fields := appmodel.SnapshotInstanceConfigFields(appObj, inst)
	addons := appmodel.SnapshotInstanceAddons(inst.Spec.Addons)
	return &corev1.InstanceBackupSnapshot{
		CapturedAt:    &now,
		AppName:       o.resolveAppName(inst),
		ServerVersion: resolveInstanceServerVersion(inst),
		ConfigCount:   len(fields),
		AddonCount:    len(addons),
		Addons:        addons,
		ConfigFields:  fields,
	}
}

func backupResponse(inst *corev1.Instance, r backupdb.Record) *corev1.InstanceFiles {
	out := &corev1.InstanceFiles{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"}, Metadata: inst.Metadata,
		Action: corev1.FileActionBackup, Path: r.Command.ObjectKey, OperationID: r.Command.OperationID,
		MaxBytes: r.Command.MaxBytes, RateLimitBytesPerSec: r.Command.RateLimit, Phase: string(r.Phase),
	}
	if r.Result != nil {
		out.SizeBytes, out.SHA256 = r.Result.SizeBytes, r.Result.SHA256
		t := r.Result.CompletedAt
		out.CompletedAt = &t
	}
	return out
}
