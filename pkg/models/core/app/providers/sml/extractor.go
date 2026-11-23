package sml

import (
	"path"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

const modsPrefix = "factorygame/mods/"

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeSML }
func (*extractor) Priority() int { return 37 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		if _, _, ok := parseSMLModPath(f, modsPrefix); ok {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeSML, TargetDir: "FactoryGame/Mods"}
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
	targetDir := driver.FirstNonEmpty(rawTarget, "FactoryGame/Mods")
	prefix := strings.ToLower(strings.Trim(targetDir, "/")) + "/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		modRef, archPath, ok := parseSMLModPath(f, prefix)
		if !ok || seen[strings.ToLower(modRef)] {
			continue
		}
		seen[strings.ToLower(modRef)] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: modRef, Title: modRef,
			InstallMode: corev1.AddonInstallModeSML, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	return out
}

func parseSMLModPath(f, prefix string) (modRef, archPath string, ok bool) {
	low := strings.ToLower(f)
	if !strings.HasPrefix(low, prefix) || !strings.HasSuffix(low, ".uplugin") {
		return "", "", false
	}
	rest := f[len(prefix):]
	seg, file, hasSub := strings.Cut(rest, "/")
	if !hasSub || strings.Contains(file, "/") || seg == "" {
		return "", "", false
	}
	stem := strings.TrimSuffix(path.Base(file), path.Ext(file))
	return driver.FirstNonEmpty(stem, seg), strings.TrimSuffix(prefix, "/") + "/" + seg, true
}
