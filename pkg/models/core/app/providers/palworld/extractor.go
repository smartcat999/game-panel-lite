package palworld

import (
	"path"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

const (
	defaultTargetDir = "Pal/Content/Paks/~mods"
	modsPfx          = "pal/content/paks/~mods/"
	logicModsPfx     = "pal/content/paks/logicmods/"
	luaModsPfx       = "pal/binaries/linux/mods/"
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModePalworldMod }
func (*extractor) Priority() int { return 40 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		if ((strings.HasPrefix(low, modsPfx) || strings.HasPrefix(low, logicModsPfx)) && strings.HasSuffix(low, ".pak")) ||
			low == "pal/binaries/linux/libue4ss.so" {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModePalworldMod, TargetDir: defaultTargetDir}
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
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		pkg, archPath, ok := parsePalworldModPath(f)
		if !ok || seen[strings.ToLower(pkg)] {
			continue
		}
		seen[strings.ToLower(pkg)] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: pkg, Title: pkg,
			InstallMode: corev1.AddonInstallModePalworldMod, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	return out
}

func parsePalworldModPath(f string) (pkg, archPath string, ok bool) {
	low := strings.ToLower(f)
	for _, pfx := range []string{modsPfx, logicModsPfx} {
		if strings.HasPrefix(low, pfx) && strings.HasSuffix(low, ".pak") {
			stem := strings.TrimSuffix(path.Base(f), path.Ext(f))
			stem = strings.TrimSuffix(stem, "_P")
			return stem, f, stem != ""
		}
	}
	if strings.HasPrefix(low, luaModsPfx) {
		rest := f[len(luaModsPfx):]
		seg, sub, hasSub := strings.Cut(rest, "/")
		if hasSub && seg != "" && !strings.EqualFold(seg, "shared") && strings.HasPrefix(strings.ToLower(sub), "scripts/") {
			return seg, strings.TrimSuffix(luaModsPfx, "/") + "/" + seg, true
		}
	}
	return "", "", false
}
