package instance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
)

const (
	maxIndexedInspectDirs = 64
	maxInspectDirEntries  = 12
)

func (o *operator) handleInspectBackupAction(ctx context.Context, existing *corev1.Instance, input *corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	if strings.TrimSpace(input.PresignedURL) != "" {
		return nil, fmt.Errorf("%w: presignedURL is not accepted", models.ErrBadRequest)
	}
	key, err := o.readableBackupKey(ctx, existing, input.Path)
	if err != nil {
		return nil, err
	}
	return o.handleInspectBackupWithKey(ctx, existing, key, input, maxBytes)
}

func (o *operator) handleInspectBackupWithKey(ctx context.Context, existing *corev1.Instance, key string, input *corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	rec, ok := findBackupRecordByKey(existing.Status.Backups, key)
	if !ok {
		return nil, fmt.Errorf("%w: backup record not found", models.ErrNotFound)
	}
	if rec.Phase == corev1.BackupPhaseInspectFailed {
		if !rec.Imported && rec.Snapshot != nil {
			return o.buildSnapshotFallbackInspectResponse(ctx, existing, rec, input, maxBytes)
		}
		return nil, fmt.Errorf("%w: %s", models.ErrBadRequest, nonEmptyInspectMessage(rec.Message))
	}
	if rec.Phase != "" && rec.Phase != corev1.BackupPhaseCompleted && rec.Phase != corev1.BackupPhaseInspecting && rec.Phase != corev1.BackupPhaseRestoring {
		return nil, fmt.Errorf("%w: backup archive is not ready for inspection", models.ErrConflict)
	}
	ins := rec.Inspection
	if ins == nil || ins.Phase == "" {
		if err := o.enqueueBackupInspection(ctx, existing, key); err != nil {
			return nil, err
		}
		return o.pendingInspectResponse(ctx, existing, rec, input, maxBytes), nil
	}
	switch ins.Phase {
	case corev1.BackupInspectPhasePending:
		o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
		return o.pendingInspectResponse(ctx, existing, rec, input, maxBytes), nil
	case corev1.BackupInspectPhaseFailed:
		if !rec.Imported && rec.Snapshot != nil {
			return o.buildSnapshotFallbackInspectResponse(ctx, existing, rec, input, maxBytes)
		}
		return nil, fmt.Errorf("%w: %s", models.ErrBadRequest, nonEmptyInspectMessage(ins.Message))
	case corev1.BackupInspectPhaseCompleted:
		if ins.SchemaVersion < corev1.CurrentBackupInspectSchemaVersion {
			if err := o.enqueueBackupInspection(ctx, existing, key); err != nil {
				return nil, err
			}
			return o.pendingInspectResponse(ctx, existing, rec, input, maxBytes), nil
		}
		return o.buildCompletedInspectResponse(ctx, existing, rec, input, maxBytes)
	default:
		if err := o.enqueueBackupInspection(ctx, existing, key); err != nil {
			return nil, err
		}
		return o.pendingInspectResponse(ctx, existing, rec, input, maxBytes), nil
	}
}

func (o *operator) pendingInspectResponse(ctx context.Context, existing *corev1.Instance, rec corev1.InstanceBackupRecord, input *corev1.InstanceFiles, maxBytes int64) *corev1.InstanceFiles {
	defaultTarget, _, defaultReplace := o.resolveInstanceInspectConfig(ctx, existing)
	targetPath := defaultTarget
	if resolved, err := resolveInspectTargetPath(existing, input.TargetPath, defaultTarget); err == nil {
		targetPath = resolved
	}
	replace := defaultReplace && targetPath != "."
	out := &corev1.InstanceFiles{
		TypeMeta:      metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:      existing.Metadata,
		Action:        corev1.FileActionInspectBackup,
		Path:          rec.Path,
		Phase:         corev1.BackupInspectPhasePending,
		SizeBytes:     rec.SizeBytes,
		CompletedAt:   rec.CompletedAt,
		TargetPath:    targetPath,
		ReplaceTarget: &replace,
		ManagedFiles:  managedFilesUnderTarget(existing.Spec.Files, targetPath),
		MaxBytes:      maxBytes,
	}
	o.attachBackupMetadataPreview(ctx, existing, rec, input.SourcePath, targetPath, out)
	return out
}

func (o *operator) buildSnapshotFallbackInspectResponse(ctx context.Context, existing *corev1.Instance, rec corev1.InstanceBackupRecord, input *corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	out := o.pendingInspectResponse(ctx, existing, rec, input, maxBytes)
	out.Phase = corev1.BackupInspectPhaseCompleted
	return out, nil
}

func (o *operator) enqueueBackupInspection(ctx context.Context, existing *corev1.Instance, key string) error {
	pending := &corev1.InstanceBackupInspection{Phase: corev1.BackupInspectPhasePending}
	if err := o.saveBackupInspection(ctx, existing, key, pending, true); err != nil {
		return err
	}
	o.publishInstanceEvent(ctx, mq.EventReconcile, existing)
	return nil
}

func (o *operator) buildCompletedInspectResponse(ctx context.Context, existing *corev1.Instance, rec corev1.InstanceBackupRecord, input *corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	ins := rec.Inspection
	defaultTarget, _, defaultReplace := o.resolveInstanceInspectConfig(ctx, existing)
	targetPath, err := resolveInspectTargetPath(existing, input.TargetPath, defaultTarget)
	if err != nil {
		return nil, err
	}
	recommended := ins.RecommendedSource
	if recommended == "" && !rec.Imported {
		recommended = "."
		if defaultTarget != "." && slices.Contains(ins.ArchiveDirectories, defaultTarget) {
			recommended = defaultTarget
		}
	}
	sourcePath, err := resolveInspectSourcePath(input.SourcePath, recommended)
	if err != nil {
		return nil, err
	}
	replace := defaultReplace && targetPath != "."
	out := &corev1.InstanceFiles{
		TypeMeta:           metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:           existing.Metadata,
		Action:             corev1.FileActionInspectBackup,
		Path:               rec.Path,
		Phase:              corev1.BackupInspectPhaseCompleted,
		SizeBytes:          rec.SizeBytes,
		CompletedAt:        rec.CompletedAt,
		ArchiveFormat:      ins.ArchiveFormat,
		RecommendedSource:  ins.RecommendedSource,
		SourcePath:         sourcePath,
		TargetPath:         targetPath,
		ReplaceTarget:      &replace,
		CandidateSources:   ins.CandidateSources,
		ArchiveDirectories: ins.ArchiveDirectories,
		SourceEntries:      lookupInspectionEntries(ins.DirectoryEntries, sourcePath),
		ManagedFiles:       managedFilesUnderTarget(existing.Spec.Files, targetPath),
		FileCount:          ins.FileCount,
		UncompressedBytes:  ins.UncompressedBytes,
		MaxBytes:           maxBytes,
	}
	o.attachBackupMetadataPreview(ctx, existing, rec, sourcePath, targetPath, out)
	return out, nil
}

func (o *operator) attachBackupMetadataPreview(ctx context.Context, existing *corev1.Instance, rec corev1.InstanceBackupRecord, sourcePath, targetPath string, out *corev1.InstanceFiles) {
	appObj := o.resolveAppForInstance(ctx, existing)
	out.CurrentServerVersion = resolveInstanceServerVersion(existing)
	if rec.Snapshot != nil && rec.Snapshot.ServerVersion != "" {
		out.ServerVersion = rec.Snapshot.ServerVersion
	}
	if !rec.Imported && rec.Snapshot != nil && len(rec.Snapshot.ConfigFields) > 0 {
		out.ConfigPreview = appmodel.BuildSnapshotConfigPreview(appObj, existing, rec.Snapshot.ConfigFields)
	} else if rec.Inspection != nil {
		out.ConfigPreview = appmodel.BuildBackupConfigPreview(appObj, existing, rec.Inspection.ExtractedFields, sourcePath, targetPath)
	}
	var snapAddons, archAddons []corev1.InstanceBackupAddon
	if rec.Snapshot != nil {
		snapAddons = rec.Snapshot.Addons
	}
	if rec.Inspection != nil {
		archAddons = rec.Inspection.ExtractedAddons
	}
	if len(snapAddons) > 0 || len(archAddons) > 0 {
		out.AddonPreview = appmodel.BuildBackupAddonPreview(appObj, existing, snapAddons, archAddons, sourcePath)
	}
}

func resolveInstanceServerVersion(inst *corev1.Instance) string {
	if inst == nil {
		return ""
	}
	if inst.Spec.ServerArtifact != nil && strings.TrimSpace(inst.Spec.ServerArtifact.Version) != "" {
		return strings.TrimSpace(inst.Spec.ServerArtifact.Version)
	}
	if v := strings.TrimSpace(inst.Metadata.Annotations[corev1.AnnotationInstanceAppVersion]); v != "" {
		return v
	}
	if inst.Spec.Env != nil {
		return strings.TrimSpace(inst.Spec.Env["VERSION"])
	}
	return ""
}

func (o *operator) latestInstanceForBackupInspect(ctx context.Context, inst *corev1.Instance) *corev1.Instance {
	if o.db == nil || inst == nil {
		return inst
	}
	latest, err := o.db.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name)
	if err != nil || latest == nil || latest.Metadata.UID != inst.Metadata.UID {
		return inst
	}
	return latest
}

func (o *operator) reconcilePendingBackupInspections(ctx context.Context, inst *corev1.Instance) error {
	if inst == nil {
		return nil
	}
	var errs []error
	for _, b := range inst.Status.Backups {
		if b.Phase != "" && b.Phase != corev1.BackupPhaseCompleted && b.Phase != corev1.BackupPhaseInspecting {
			continue
		}
		if b.Inspection == nil || b.Inspection.Phase != corev1.BackupInspectPhasePending {
			continue
		}
		if err := o.inspectSingleBackupAsync(ctx, inst, b); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (o *operator) inspectSingleBackupAsync(ctx context.Context, inst *corev1.Instance, b corev1.InstanceBackupRecord) error {
	cli := o.s3Client()
	if !cli.IsConfigured() {
		return fmt.Errorf("platform object storage (s3) is not configured")
	}
	maxBytes, err := resolveFileActionMaxBytes(inst, corev1.InstanceFiles{Action: corev1.FileActionInspectBackup})
	if err != nil {
		return o.saveFailedInspection(ctx, inst, b, formatInspectArchiveError(err))
	}
	defaultTarget, anchors, _ := o.resolveInstanceInspectConfig(ctx, inst)
	body, _, err := cli.Get(ctx, b.Path)
	if err != nil {
		if errors.Is(err, s3.ErrNotFound) {
			return o.saveFailedInspection(ctx, inst, b, "对象存储中未找到目标备份归档")
		}
		return fmt.Errorf("fetch backup archive for inspection: %w", err)
	}
	defer func() { _ = body.Close() }()
	res, err := volume.InspectArchiveStream(ctx, body, maxBytes, volume.ArchiveInspectOptions{
		DefaultTarget: defaultTarget,
		AnchorFiles:   anchors,
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return o.saveFailedInspection(ctx, inst, b, formatInspectArchiveError(err))
	}
	completed := o.buildCompletedBackupInspection(ctx, inst, res, defaultTarget)
	if err := o.saveBackupInspection(ctx, inst, b.Path, completed, false); err != nil {
		return err
	}
	if strings.TrimSpace(b.TaskID) != "" {
		AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, b.TaskID, fmt.Sprintf("归档解析完成（%s · %d 个文件）", strings.ToUpper(res.Format), res.FileCount))
		FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, b.TaskID)
	}
	return nil
}

func (o *operator) buildCompletedBackupInspection(ctx context.Context, inst *corev1.Instance, res *volume.ArchiveInspection, defaultTarget string) *corev1.InstanceBackupInspection {
	now := time.Now().UTC()
	appObj := o.resolveAppForInstance(ctx, inst)
	return &corev1.InstanceBackupInspection{
		SchemaVersion:      corev1.CurrentBackupInspectSchemaVersion,
		Phase:              corev1.BackupInspectPhaseCompleted,
		InspectedAt:        &now,
		ArchiveFormat:      res.Format,
		FileCount:          res.FileCount,
		UncompressedBytes:  res.UncompressedBytes,
		RecommendedSource:  res.RecommendedSource,
		CandidateSources:   res.CandidateSources,
		ArchiveDirectories: res.Directories,
		DirectoryEntries:   buildInspectionDirectoryEntries(res),
		ExtractedFields:    appmodel.ExtractArchiveConfigFields(appObj, inst.Metadata.UID, res.TextFiles, defaultTarget),
		ExtractedAddons:    appmodel.ExtractArchiveAddons(appObj, res.Files, res.TextFiles),
	}
}

func formatInspectArchiveError(err error) string {
	if errors.Is(err, volume.ErrFileQuotaExceeded) {
		return "归档解压体积超出允许上限，请清理冗余文件后重试"
	}
	raw := strings.TrimSpace(err.Error())
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "unsupported backup archive format"):
		return "不支持的压缩格式，请上传有效的 .tar.gz 或 .zip 归档"
	case strings.Contains(lower, "invalid zip") || strings.Contains(lower, "invalid gzip") || strings.Contains(lower, "read tar"):
		return "归档文件已损坏或格式无效: " + raw
	default:
		return "解析备份归档失败: " + raw
	}
}

func (o *operator) saveFailedInspection(ctx context.Context, inst *corev1.Instance, b corev1.InstanceBackupRecord, msg string) error {
	now := time.Now().UTC()
	failed := &corev1.InstanceBackupInspection{
		Phase:       corev1.BackupInspectPhaseFailed,
		Message:     strings.TrimSpace(msg),
		InspectedAt: &now,
	}
	err := o.saveBackupInspection(ctx, inst, b.Path, failed, false)
	if err == nil && strings.TrimSpace(b.TaskID) != "" {
		AppendInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, b.TaskID, "归档解析失败: "+nonEmptyInspectMessage(failed.Message))
		FinishInstanceTaskLog(ctx, o.taskLogs, inst.Metadata.UID, b.TaskID)
	}
	return err
}

func (o *operator) saveBackupInspection(ctx context.Context, inst *corev1.Instance, key string, ins *corev1.InstanceBackupInspection, onlyIfUnset bool) error {
	apply := func(items []corev1.InstanceBackupRecord) []corev1.InstanceBackupRecord {
		for i := range items {
			if items[i].Path != key {
				continue
			}
			if onlyIfUnset && !needsBackupInspectEnqueue(items[i].Inspection) {
				continue
			}
			applySavedInspectionRecord(&items[i], ins)
		}
		return items
	}
	apply(inst.Status.Backups)
	if o.db == nil {
		return nil
	}
	return o.mutateBackups(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, apply)
}

func applySavedInspectionRecord(item *corev1.InstanceBackupRecord, ins *corev1.InstanceBackupInspection) {
	item.Inspection = ins
	if ins == nil || (!item.Imported && item.Phase != corev1.BackupPhaseInspecting) {
		return
	}
	if item.Phase == corev1.BackupPhaseDeleting || item.Phase == corev1.BackupPhaseRestoring {
		return
	}
	switch ins.Phase {
	case corev1.BackupInspectPhasePending:
		item.Phase = corev1.BackupPhaseInspecting
		item.Stage = corev1.BackupStageInspecting
		item.Progress = 90
		item.Message = backupInspectingMessage
	case corev1.BackupInspectPhaseCompleted:
		item.Phase = corev1.BackupPhaseCompleted
		item.Stage = corev1.BackupStageCompleted
		item.Progress = 100
		item.Message = ""
	case corev1.BackupInspectPhaseFailed:
		item.Phase = corev1.BackupPhaseInspectFailed
		item.Stage = corev1.BackupStageFailed
		item.Progress = 100
		item.Message = nonEmptyInspectMessage(ins.Message)
	}
}

func needsBackupInspectEnqueue(ins *corev1.InstanceBackupInspection) bool {
	if ins == nil || ins.Phase == "" {
		return true
	}
	return ins.Phase == corev1.BackupInspectPhaseCompleted && ins.SchemaVersion < corev1.CurrentBackupInspectSchemaVersion
}

func buildInspectionDirectoryEntries(res *volume.ArchiveInspection) map[string][]string {
	out := make(map[string][]string)
	addDir := func(dir string) {
		if dir == "" {
			return
		}
		if _, seen := out[dir]; seen {
			return
		}
		if entries := volume.ListArchiveImmediateEntries(res.Files, res.Directories, dir, maxInspectDirEntries); len(entries) > 0 {
			out[dir] = entries
		}
	}
	addDir(".")
	addDir(res.RecommendedSource)
	for _, dir := range res.CandidateSources {
		addDir(dir)
	}
	count := 0
	for _, dir := range res.Directories {
		if count >= maxIndexedInspectDirs {
			break
		}
		addDir(dir)
		count++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (o *operator) resolveAppForInstance(ctx context.Context, inst *corev1.Instance) *corev1.App {
	appName := o.resolveAppName(inst)
	if appName == "" {
		return nil
	}
	if o.appDB != nil {
		return o.getAppCached(ctx, appName, nil)
	}
	for _, a := range appmodel.BuiltInApps() {
		if a.Metadata.Name == appName {
			return a
		}
	}
	return nil
}

func findBackupRecordByKey(backups []corev1.InstanceBackupRecord, key string) (corev1.InstanceBackupRecord, bool) {
	for _, b := range backups {
		if b.Path == key {
			return b, true
		}
	}
	return corev1.InstanceBackupRecord{}, false
}

func resolveInspectTargetPath(inst *corev1.Instance, rawTarget, defaultTarget string) (string, error) {
	if strings.TrimSpace(rawTarget) == "" {
		return defaultTarget, nil
	}
	return resolveEffectiveRestoreTarget(inst, rawTarget)
}

func resolveInspectSourcePath(rawSource, recommended string) (string, error) {
	if strings.TrimSpace(rawSource) == "" {
		return recommended, nil
	}
	src, err := metav1.NormalizeArchiveEntryPath(rawSource, true)
	if err != nil {
		return "", fmt.Errorf("%w: invalid inspect sourcePath: %v", models.ErrBadRequest, err)
	}
	if volume.IsReservedMetadataPath(src) {
		return "", fmt.Errorf("%w: reserved workspace path is not allowed", models.ErrBadRequest)
	}
	return src, nil
}

func lookupInspectionEntries(entries map[string][]string, sourcePath string) []string {
	if len(entries) == 0 || strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	return append([]string(nil), entries[sourcePath]...)
}

func nonEmptyInspectMessage(msg string) string {
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	return "inspect backup archive failed"
}

func (o *operator) resolveInstanceInspectConfig(ctx context.Context, existing *corev1.Instance) (string, []string, bool) {
	var subtree string
	var anchors []string
	var replaceOnRestore bool
	if existing != nil && existing.Spec.BackupPolicy != nil {
		subtree = existing.Spec.BackupPolicy.Subtree
		anchors = existing.Spec.BackupPolicy.AnchorFiles
		replaceOnRestore = existing.Spec.BackupPolicy.ReplaceOnRestore
	}
	if len(anchors) == 0 || (existing != nil && existing.Spec.BackupPolicy == nil) {
		if appObj := o.resolveAppForInstance(ctx, existing); appObj != nil && appObj.StoragePolicy != nil {
			if len(anchors) == 0 {
				anchors = appObj.StoragePolicy.AnchorFiles
			}
			if existing != nil && existing.Spec.BackupPolicy == nil {
				subtree = appObj.StoragePolicy.BackupSubtree
				replaceOnRestore = appObj.StoragePolicy.ReplaceOnRestore
			}
		}
	}
	defaultTarget := resolveDefaultRestoreTarget(subtree)
	defaultReplace := replaceOnRestore && defaultTarget != "."
	return defaultTarget, anchors, defaultReplace
}

func resolveDefaultRestoreTarget(subtree string) string {
	var parts []string
	for _, seg := range strings.Split(subtree, ",") {
		trimmed := strings.TrimSpace(seg)
		if trimmed == "" || trimmed == "." || trimmed == "/" {
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) != 1 {
		return "."
	}
	norm, err := metav1.NormalizeArchiveEntryPath(parts[0], true)
	if err != nil || norm == "" || volume.IsReservedMetadataPath(norm) {
		return "."
	}
	return norm
}

func resolveEffectiveRestoreTarget(inst *corev1.Instance, rawTarget string) (string, error) {
	if strings.TrimSpace(rawTarget) == "" {
		subtree := ""
		if inst != nil && inst.Spec.BackupPolicy != nil {
			subtree = inst.Spec.BackupPolicy.Subtree
		}
		return resolveDefaultRestoreTarget(subtree), nil
	}
	targetRel, err := metav1.NormalizeArchiveEntryPath(rawTarget, true)
	if err != nil {
		return "", fmt.Errorf("%w: invalid restore targetPath: %v", models.ErrBadRequest, err)
	}
	if volume.IsReservedMetadataPath(targetRel) {
		return "", fmt.Errorf("%w: reserved workspace path is not allowed", models.ErrBadRequest)
	}
	return targetRel, nil
}

func resolveRestoreMapping(inst *corev1.Instance, rawSource, rawTarget string, reqReplace *bool) (string, string, bool, error) {
	targetRel, err := resolveEffectiveRestoreTarget(inst, rawTarget)
	if err != nil {
		return "", "", false, err
	}
	var sourceRel string
	if strings.TrimSpace(rawSource) == "" && strings.TrimSpace(rawTarget) == "" {
		sourceRel = targetRel
	} else {
		sourceRel, err = metav1.NormalizeArchiveEntryPath(rawSource, true)
		if err != nil {
			return "", "", false, fmt.Errorf("%w: invalid restore sourcePath: %v", models.ErrBadRequest, err)
		}
		if volume.IsReservedMetadataPath(sourceRel) {
			return "", "", false, fmt.Errorf("%w: reserved workspace path is not allowed", models.ErrBadRequest)
		}
	}
	replace := inst != nil && inst.Spec.BackupPolicy != nil && inst.Spec.BackupPolicy.ReplaceOnRestore
	if reqReplace != nil {
		replace = *reqReplace
	}
	if targetRel == "." {
		replace = false
	}
	return sourceRel, targetRel, replace, nil
}

func managedFilesUnderTarget(files []corev1.InstanceFile, target string) []string {
	if len(files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, f := range files {
		p, err := metav1.NormalizeArchiveEntryPath(f.Path, false)
		if err != nil {
			continue
		}
		if target == "." || p == target || strings.HasPrefix(p, target+"/") {
			seen[p] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
