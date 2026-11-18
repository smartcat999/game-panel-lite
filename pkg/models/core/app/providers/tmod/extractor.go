package tmod

import (
	"encoding/json"
	"path"
	"sort"
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

func (*extractor) Mode() string  { return corev1.AddonInstallModeTModLatest }
func (*extractor) Priority() int { return 30 }

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for p := range ctx.RelTextFiles() {
		if strings.EqualFold(path.Base(p), "enabled.json") {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeTModLatest, TargetDir: "Mods"}
		}
	}
	for _, f := range ctx.RelBinaryPaths() {
		if strings.HasSuffix(strings.ToLower(f), ".tmod") {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeTModLatest, TargetDir: "Mods"}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	repo, targetDir := "", "Mods"
	if binding != nil {
		repo = binding.Repository
		targetDir = driver.FirstNonEmpty(binding.TargetDir, "Mods")
	}
	enabledByDir := parseTModEnabledJSONs(ctx.RelTextFiles())
	seen := make(map[string]bool)
	var out []corev1.InstanceBackupAddon
	for _, f := range ctx.RelBinaryPaths() {
		if !strings.HasSuffix(strings.ToLower(f), ".tmod") {
			continue
		}
		dir := path.Dir(f)
		stem := strings.TrimSuffix(path.Base(f), path.Ext(f))
		enabledSet, hasEnabledFile := enabledByDir[dir]
		disabled := hasEnabledFile && !enabledSet[stem]
		key := dir + "\x00" + stem
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: stem, Title: stem, InstallMode: corev1.AddonInstallModeTModLatest,
			TargetDir: targetDir, Disabled: disabled, ArchivePath: dir,
		})
	}
	return appendRemainingEnabledTMods(out, repo, targetDir, enabledByDir, seen)
}

func parseTModEnabledJSONs(textFiles map[string]string) map[string]map[string]bool {
	res := make(map[string]map[string]bool)
	for _, archPath := range driver.SortedTextFileKeys(textFiles) {
		if !strings.EqualFold(path.Base(archPath), "enabled.json") {
			continue
		}
		var names []string
		if json.Unmarshal([]byte(textFiles[archPath]), &names) != nil {
			continue
		}
		dir := path.Dir(archPath)
		set := make(map[string]bool, len(names))
		for _, n := range names {
			if trimmed := strings.TrimSpace(n); trimmed != "" {
				set[trimmed] = true
			}
		}
		res[dir] = set
	}
	return res
}

func appendRemainingEnabledTMods(dst []corev1.InstanceBackupAddon, repo, targetDir string, enabledByDir map[string]map[string]bool, seen map[string]bool) []corev1.InstanceBackupAddon {
	dirs := make([]string, 0, len(enabledByDir))
	for d := range enabledByDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		for _, stem := range driver.SortedBoolKeys(enabledByDir[dir]) {
			if seen[dir+"\x00"+stem] {
				continue
			}
			dst = append(dst, corev1.InstanceBackupAddon{
				Repository: repo, Package: stem, Title: stem, InstallMode: corev1.AddonInstallModeTModLatest,
				TargetDir: targetDir, ArchivePath: dir,
			})
		}
	}
	return dst
}
