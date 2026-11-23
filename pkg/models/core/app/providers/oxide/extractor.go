package oxide

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

func (*extractor) Mode() string  { return corev1.AddonInstallModeOxide }
func (*extractor) Priority() int { return 36 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		if strings.HasSuffix(low, "_data/managed/oxide.core.dll") ||
			(strings.HasPrefix(low, "oxide/plugins/") && strings.HasSuffix(low, ".cs")) {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeOxide, TargetDir: "oxide"}
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
	targetDir := driver.FirstNonEmpty(rawTarget, "oxide")
	pluginsPrefix := strings.ToLower(strings.Trim(targetDir, "/")) + "/plugins/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		if !strings.HasPrefix(low, pluginsPrefix) || !strings.HasSuffix(low, ".cs") {
			continue
		}
		base := path.Base(f)
		stem := strings.TrimSuffix(base, path.Ext(base))
		if stem == "" || seen[stem] {
			continue
		}
		seen[stem] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: stem, Title: stem,
			InstallMode: corev1.AddonInstallModeOxide, TargetDir: targetDir, ArchivePath: f,
		})
	}
	return out
}
