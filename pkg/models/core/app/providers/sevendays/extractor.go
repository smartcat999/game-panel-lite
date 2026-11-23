package sevendays

import (
	"path"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

const (
	defaultTargetDir = "Mods"
	harmonyFolder    = "0_TFP_Harmony"
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeSevenDaysMod }
func (*extractor) Priority() int { return 39 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, p := range collectCandidatePaths(ctx) {
		if _, _, ok := parseModInfoPath(p, "mods/"); ok {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeSevenDaysMod, TargetDir: defaultTargetDir}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	repo, rawTarget := "", ""
	if binding != nil {
		repo, rawTarget = binding.Repository, binding.TargetDir
	}
	targetDir := driver.FirstNonEmpty(rawTarget, defaultTargetDir)
	prefix := strings.ToLower(strings.Trim(targetDir, "/")) + "/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, p := range collectCandidatePaths(ctx) {
		modName, archPath, ok := parseModInfoPath(p, prefix)
		if !ok || seen[strings.ToLower(modName)] {
			continue
		}
		seen[strings.ToLower(modName)] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: modName, Title: modName,
			InstallMode: corev1.AddonInstallModeSevenDaysMod, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	return out
}

func collectCandidatePaths(ctx *driver.ExtractContext) []string {
	if ctx == nil {
		return nil
	}
	out := make([]string, 0, len(ctx.RelBinaryPaths())+len(ctx.RelTextFiles()))
	out = append(out, ctx.RelBinaryPaths()...)
	for k := range ctx.RelTextFiles() {
		out = append(out, k)
	}
	return out
}

func parseModInfoPath(p, prefix string) (modName, archPath string, ok bool) {
	low := strings.ToLower(p)
	if !strings.HasPrefix(low, prefix) || strings.ToLower(path.Base(p)) != "modinfo.xml" {
		return "", "", false
	}
	rest := p[len(prefix):]
	seg, file, hasSub := strings.Cut(rest, "/")
	if !hasSub || strings.Contains(file, "/") || seg == "" || strings.EqualFold(seg, harmonyFolder) {
		return "", "", false
	}
	return seg, strings.TrimSuffix(prefix, "/") + "/" + seg, true
}
