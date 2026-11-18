package flatdir

import (
	"path"
	"regexp"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

var flatJarVerSplitRe = regexp.MustCompile(`^(.+?)-(\d[\w.+\-]*)$`)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeFlatDir }
func (*extractor) Priority() int { return 40 }

func isFlatAddonExt(p string) bool {
	low := strings.ToLower(p)
	return strings.HasSuffix(low, ".jar") || strings.HasSuffix(low, ".vpk")
}

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for _, f := range ctx.RelBinaryPaths() {
		low := strings.ToLower(f)
		dirBase := strings.ToLower(path.Base(path.Dir(f)))
		switch {
		case strings.HasSuffix(low, ".jar") && (dirBase == "plugins" || dirBase == "mods"):
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: dirBase}
		case strings.HasSuffix(low, ".vpk") && dirBase == "addons":
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: path.Dir(f)}
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
	targetDir := driver.FirstNonEmpty(rawTarget, "plugins")
	dirName := strings.Trim(path.Base(targetDir), "/")
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		if !isFlatAddonExt(f) || !strings.EqualFold(path.Base(path.Dir(f)), dirName) {
			continue
		}
		base := strings.TrimSuffix(path.Base(f), path.Ext(f))
		pkg, ver := base, ""
		if m := flatJarVerSplitRe.FindStringSubmatch(base); len(m) == 3 {
			pkg, ver = m[1], m[2]
		}
		key := path.Dir(f) + "\x00" + pkg
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: pkg, Title: pkg, Version: ver,
			InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: targetDir, ArchivePath: path.Dir(f),
		})
	}
	return out
}
