package corekeeper

import (
	"path"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

const (
	defaultTargetDir = "CoreKeeperServer_Data/StreamingAssets/Mods"
	bepPluginsPrefix = "bepinex/plugins/"
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeCoreKeeperMod }
func (*extractor) Priority() int { return 41 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	pfx := strings.ToLower(defaultTargetDir) + "/"
	for _, p := range collectCandidatePaths(ctx) {
		if _, _, ok := parseNativeModPath(p, pfx); ok {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeCoreKeeperMod, TargetDir: defaultTargetDir}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	primaryRepo, nativeRepo, targetDir := resolveReposAndTarget(binding)
	nativePfx := strings.ToLower(strings.Trim(targetDir, "/")) + "/"
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, p := range collectCandidatePaths(ctx) {
		if pkg, archPath, ok := parseNativeModPath(p, nativePfx); ok && !seen["n:"+strings.ToLower(pkg)] {
			seen["n:"+strings.ToLower(pkg)] = true
			out = append(out, corev1.InstanceBackupAddon{
				Repository: nativeRepo, Package: pkg, Title: pkg,
				InstallMode: corev1.AddonInstallModeCoreKeeperMod, TargetDir: targetDir, ArchivePath: archPath,
			})
		}
	}
	for _, f := range ctx.RelBinaryPaths() {
		if pkg, archPath, ok := parseBepInExPath(f); ok && !seen["b:"+strings.ToLower(pkg)] {
			seen["b:"+strings.ToLower(pkg)] = true
			out = append(out, corev1.InstanceBackupAddon{
				Repository: primaryRepo, Package: pkg, Title: pkg,
				InstallMode: corev1.AddonInstallModeCoreKeeperMod, TargetDir: "BepInEx", ArchivePath: archPath,
			})
		}
	}
	return out
}

func resolveReposAndTarget(binding *corev1.AppAddonBinding) (primaryRepo, nativeRepo, targetDir string) {
	targetDir = defaultTargetDir
	if binding == nil {
		return "", "", targetDir
	}
	primaryRepo = binding.Repository
	nativeRepo = binding.Repository
	if len(binding.Additional) > 0 && binding.Additional[0].Repository != "" {
		nativeRepo = binding.Additional[0].Repository
	}
	targetDir = driver.FirstNonEmpty(binding.TargetDir, defaultTargetDir)
	return primaryRepo, nativeRepo, targetDir
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

func parseNativeModPath(p, pfx string) (pkg, archPath string, ok bool) {
	low := strings.ToLower(p)
	if !strings.HasPrefix(low, pfx) || strings.ToLower(path.Base(p)) != "modmanifest.json" {
		return "", "", false
	}
	rest := p[len(pfx):]
	seg, file, hasSub := strings.Cut(rest, "/")
	if !hasSub || strings.Contains(file, "/") || seg == "" {
		return "", "", false
	}
	return seg, strings.TrimSuffix(pfx, "/") + "/" + seg, true
}

func parseBepInExPath(f string) (pkg, archPath string, ok bool) {
	low := strings.ToLower(f)
	if !strings.HasPrefix(low, bepPluginsPrefix) || !strings.HasSuffix(low, ".dll") {
		return "", "", false
	}
	rest := f[len(bepPluginsPrefix):]
	if firstSeg, _, hasSub := strings.Cut(rest, "/"); hasSub && firstSeg != "" {
		return firstSeg, strings.TrimSuffix(bepPluginsPrefix, "/") + "/" + firstSeg, true
	}
	stem := strings.TrimSuffix(path.Base(rest), path.Ext(rest))
	return stem, path.Dir(f), stem != ""
}
