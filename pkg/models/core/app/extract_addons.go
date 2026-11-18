package app

import (
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

// SnapshotInstanceConfigFields captures secret-safe declarative ConfigSchema values directly
// from a live Instance at backup creation time.
func SnapshotInstanceConfigFields(appObj *corev1.App, inst *corev1.Instance) []corev1.InstanceBackupExtractedField {
	if appObj == nil || inst == nil || len(appObj.ConfigSchema) == 0 {
		return nil
	}
	curVals := resolveCurrentPreviewValues(appObj, inst)
	out := make([]corev1.InstanceBackupExtractedField, 0, len(appObj.ConfigSchema))
	for _, f := range appObj.ConfigSchema {
		rawVal, ok := curVals[f.Name]
		secret := isSecretConfigField(f)
		if (!ok || strings.TrimSpace(rawVal) == "") && !secret {
			continue
		}
		tplPath := fieldPrimaryTemplatePath(appObj, f.Name)
		out = append(out, buildExtractedFieldItem(f, rawVal, tplPath, tplPath, inst.Metadata.UID))
		if len(out) >= maxExtractedBackupFields {
			break
		}
	}
	return out
}

// BuildSnapshotConfigPreview builds config comparison items from point-in-time Snapshot
// fields against the Instance's active configuration.
func BuildSnapshotConfigPreview(appObj *corev1.App, inst *corev1.Instance, fields []corev1.InstanceBackupExtractedField) []corev1.InstanceBackupConfigItem {
	if appObj == nil || len(appObj.ConfigSchema) == 0 || len(fields) == 0 {
		return nil
	}
	byName := make(map[string]corev1.InstanceBackupExtractedField, len(fields))
	for _, f := range fields {
		if _, exists := byName[f.Name]; !exists {
			byName[f.Name] = f
		}
	}
	curVals := resolveCurrentPreviewValues(appObj, inst)
	managedSet := buildManagedFileSet(inst)
	uid := ""
	if inst != nil {
		uid = inst.Metadata.UID
	}
	return selectPreviewItems(appObj.ConfigSchema, byName, curVals, managedSet, uid)
}

// SnapshotInstanceAddons clones live Spec.Addons metadata into backup-safe records.
func SnapshotInstanceAddons(addons []corev1.InstanceAddon) []corev1.InstanceBackupAddon {
	if len(addons) == 0 {
		return nil
	}
	out := make([]corev1.InstanceBackupAddon, 0, len(addons))
	for _, a := range addons {
		if strings.TrimSpace(a.Package) == "" {
			continue
		}
		out = append(out, corev1.InstanceBackupAddon{
			Repository:  a.Repository,
			Package:     a.Package,
			Version:     a.Version,
			Title:       a.Title,
			InstallMode: a.InstallMode,
			TargetDir:   a.TargetDir,
			Disabled:    a.Disabled,
			ConfigCount: len(a.Config),
		})
	}
	return out
}

// ExtractArchiveAddons discovers installed mods or plugins from archive entries and small text files
// by delegating to the registered AddonExtractor for App.AddonBinding (or inferred layout).
func ExtractArchiveAddons(appObj *corev1.App, files []string, textFiles map[string]string) []corev1.InstanceBackupAddon {
	ctx := &driver.ExtractContext{TextFiles: textFiles, BinaryPaths: files, SourcePath: "."}
	binding := resolveArchiveAddonBinding(appObj, ctx)
	if binding == nil || binding.InstallMode == "" {
		return nil
	}
	ext, ok := driver.LookupAddonExtractor(binding.InstallMode)
	if !ok {
		return nil
	}
	items := ext.Extract(ctx, binding)
	if len(items) > driver.MaxExtractedBackupAddons {
		return items[:driver.MaxExtractedBackupAddons]
	}
	return items
}

// BuildBackupAddonPreview compares snapshot or archive-extracted addons against current Spec.Addons.
func BuildBackupAddonPreview(appObj *corev1.App, inst *corev1.Instance, snapshotAddons, extractedAddons []corev1.InstanceBackupAddon, sourcePath string) []corev1.InstanceBackupAddonItem {
	defaultRepo, mode := bindingRepoAndMode(appObj)
	selected := selectBackupAddonRecords(snapshotAddons, extractedAddons, sourcePath, mode)
	if len(selected) == 0 {
		return nil
	}
	curByPkg := indexCurrentInstanceAddons(inst, mode)
	out := make([]corev1.InstanceBackupAddonItem, 0, len(selected))
	for _, b := range selected {
		out = append(out, buildAddonPreviewItem(b, curByPkg, defaultRepo, mode))
	}
	return out
}

func fieldPrimaryTemplatePath(appObj *corev1.App, fieldName string) string {
	for _, ft := range appObj.FileTemplates {
		if strings.Contains(ft.Content, fieldName) && lineMentionsField(ft.Content, fieldName) {
			return driver.NormalizeRelConfigPath(ft.Path)
		}
	}
	return ""
}

func resolveArchiveAddonBinding(appObj *corev1.App, ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	if appObj != nil && appObj.AddonBinding != nil && appObj.AddonBinding.InstallMode != "" {
		cp := *appObj.AddonBinding
		return &cp
	}
	return driver.InferAddonBinding(ctx)
}

func selectBackupAddonRecords(snapshotAddons, extractedAddons []corev1.InstanceBackupAddon, sourcePath, defaultMode string) []corev1.InstanceBackupAddon {
	if len(snapshotAddons) > 0 {
		return snapshotAddons
	}
	if len(extractedAddons) == 0 {
		return nil
	}
	normSrc := "."
	if strings.TrimSpace(sourcePath) != "" {
		if norm, err := metav1.NormalizeArchiveEntryPath(sourcePath, true); err == nil {
			normSrc = norm
		}
	}
	if normSrc == "." {
		return dedupeBackupAddons(extractedAddons, defaultMode)
	}
	var scoped []corev1.InstanceBackupAddon
	for _, a := range extractedAddons {
		ap := driver.FirstNonEmpty(a.ArchivePath, ".")
		if ap == "." || ap == normSrc || strings.HasPrefix(ap, normSrc+"/") {
			scoped = append(scoped, a)
		}
	}
	if len(scoped) > 0 {
		return dedupeBackupAddons(scoped, defaultMode)
	}
	return dedupeBackupAddons(extractedAddons, defaultMode)
}

func dedupeBackupAddons(items []corev1.InstanceBackupAddon, defaultMode string) []corev1.InstanceBackupAddon {
	out := make([]corev1.InstanceBackupAddon, 0, len(items))
	idxByPkg := make(map[string]int, len(items))
	for _, item := range items {
		effMode := driver.FirstNonEmpty(item.InstallMode, defaultMode)
		ext, ok := driver.LookupAddonExtractor(effMode)
		pkgID := strings.TrimSpace(item.Package)
		if ok {
			pkgID = ext.NormalizePackageID(item.Package)
		}
		k := strings.ToLower(pkgID)
		if pos, exists := idxByPkg[k]; exists {
			if ok {
				ext.MergeDuplicate(&out[pos], item)
			} else {
				driver.BaseExtractor{}.MergeDuplicate(&out[pos], item)
			}
			continue
		}
		idxByPkg[k] = len(out)
		out = append(out, item)
	}
	return out
}

func bindingRepoAndMode(appObj *corev1.App) (string, string) {
	if appObj != nil && appObj.AddonBinding != nil {
		return appObj.AddonBinding.Repository, appObj.AddonBinding.InstallMode
	}
	return "", ""
}

func indexCurrentInstanceAddons(inst *corev1.Instance, mode string) map[string]corev1.InstanceAddon {
	if inst == nil || len(inst.Spec.Addons) == 0 {
		return nil
	}
	out := make(map[string]corev1.InstanceAddon, len(inst.Spec.Addons))
	for _, a := range inst.Spec.Addons {
		out[normalizeBackupAddonID(a.Package, mode)] = a
	}
	return out
}

func buildAddonPreviewItem(b corev1.InstanceBackupAddon, curByPkg map[string]corev1.InstanceAddon, defaultRepo, mode string) corev1.InstanceBackupAddonItem {
	effMode := driver.FirstNonEmpty(b.InstallMode, mode)
	normPkg := normalizeBackupAddonID(b.Package, effMode)
	cur, installed := curByPkg[normPkg]
	title := driver.FirstNonEmpty(b.Title, cur.Title)
	repo := driver.FirstNonEmpty(b.Repository, cur.Repository, defaultRepo)
	bVer, curVer := comparableAddonVersion(b.Version), comparableAddonVersion(cur.Version)
	diff := !installed || b.Disabled != cur.Disabled || (bVer != "" && curVer != "" && bVer != curVer)
	return corev1.InstanceBackupAddonItem{
		Repository: repo, Package: normPkg, Title: title,
		BackupVersion: b.Version, CurrentVersion: cur.Version,
		BackupDisabled: b.Disabled, CurrentDisabled: cur.Disabled,
		InstalledNow: installed, Different: diff,
		ConfigCount: b.ConfigCount, Scopes: append([]string(nil), b.Scopes...),
	}
}

func comparableAddonVersion(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" || trimmed == "1" || strings.EqualFold(trimmed, "latest") {
		return ""
	}
	return trimmed
}

func normalizeBackupAddonID(raw, mode string) string {
	if ext, ok := driver.LookupAddonExtractor(mode); ok {
		return ext.NormalizePackageID(raw)
	}
	return strings.TrimSpace(raw)
}
