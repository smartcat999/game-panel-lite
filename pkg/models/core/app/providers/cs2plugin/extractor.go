package cs2plugin

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

func (*extractor) Mode() string  { return corev1.AddonInstallModeCS2Plugin }
func (*extractor) Priority() int { return 38 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		if strings.HasPrefix(low, "game/csgo/addons/metamod/") ||
			(strings.HasPrefix(low, "game/csgo/addons/counterstrikesharp/plugins/") && strings.HasSuffix(low, ".dll")) {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeCS2Plugin, TargetDir: "game/csgo/addons"}
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
	targetDir := driver.FirstNonEmpty(rawTarget, "game/csgo/addons")
	normTarget := strings.Trim(strings.ToLower(targetDir), "/")
	pluginsPrefix := normTarget + "/counterstrikesharp/plugins/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	if hasMetamodMarker(ctx.RelBinaryPaths(), normTarget) {
		seen["metamod"] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: "metamod", Title: "Metamod:Source",
			InstallMode: corev1.AddonInstallModeCS2Plugin, TargetDir: targetDir, ArchivePath: normTarget + "/metamod",
		})
	}
	for _, f := range ctx.RelBinaryPaths() {
		pkg, archPath, ok := parseCSSPluginPath(f, pluginsPrefix)
		if !ok || seen[strings.ToLower(pkg)] {
			continue
		}
		seen[strings.ToLower(pkg)] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: pkg, Title: pkg,
			InstallMode: corev1.AddonInstallModeCS2Plugin, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	return out
}

func hasMetamodMarker(paths []string, normTarget string) bool {
	pfx := normTarget + "/metamod/"
	for _, f := range paths {
		if strings.HasPrefix(strings.ToLower(f), pfx) {
			return true
		}
	}
	return false
}

func parseCSSPluginPath(f, pluginsPrefix string) (pkg, archPath string, ok bool) {
	low := strings.ToLower(f)
	if !strings.HasPrefix(low, pluginsPrefix) || !strings.HasSuffix(low, ".dll") {
		return "", "", false
	}
	rest := f[len(pluginsPrefix):]
	seg, _, hasSub := strings.Cut(rest, "/")
	if !hasSub || seg == "" {
		return "", "", false
	}
	return seg, path.Dir(f), true
}
