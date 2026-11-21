package bepinex

import (
	"path"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeBepInEx }
func (*extractor) Priority() int { return 35 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		if low == "bepinex/core/bepinex.preloader.dll" ||
			(strings.HasPrefix(low, "bepinex/plugins/") && strings.HasSuffix(low, ".dll")) {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeBepInEx, TargetDir: "BepInEx"}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	repo, rawTarget := "", ""
	if binding != nil {
		repo = binding.Repository
		rawTarget = binding.TargetDir
	}
	targetDir := driver.FirstNonEmpty(rawTarget, "BepInEx")
	pluginsPrefix := strings.ToLower(strings.Trim(targetDir, "/")) + "/plugins/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		pkg, archPath, ok := parsePluginPath(f, pluginsPrefix)
		if !ok || seen[pkg] {
			continue
		}
		seen[pkg] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: pkg, Title: pkg,
			InstallMode: corev1.AddonInstallModeBepInEx, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	return out
}

func parsePluginPath(f, pluginsPrefix string) (string, string, bool) {
	low := strings.ToLower(f)
	if !strings.HasPrefix(low, pluginsPrefix) || !strings.HasSuffix(low, ".dll") {
		return "", "", false
	}
	rest := f[len(pluginsPrefix):]
	if firstSeg, _, hasSub := strings.Cut(rest, "/"); hasSub && firstSeg != "" {
		return firstSeg, strings.TrimSuffix(pluginsPrefix, "/") + "/" + firstSeg, true
	}
	stem := strings.TrimSuffix(path.Base(rest), path.Ext(rest))
	if stem == "" {
		return "", "", false
	}
	return stem, path.Dir(f), true
}
