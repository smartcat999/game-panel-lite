package workshoptree

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

var (
	pzWorkshopLineRe   = regexp.MustCompile(`(?m)^\s*WorkshopItems\s*=\s*([^\r\n]*)`)
	pzModInfoNameRe    = regexp.MustCompile(`(?mi)^\s*name\s*=\s*([^\r\n]+)`)
	acfInstalledItemRe = regexp.MustCompile(`"(\d{6,})"\s*\{`)
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeWorkshopTree }
func (*extractor) Priority() int { return 20 }

func (*extractor) NormalizePackageID(pkg string) string {
	return driver.StripWorkshopPrefix(pkg)
}

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for p := range ctx.RelTextFiles() {
		lower := strings.ToLower(p)
		if (strings.Contains(lower, "zomboid/server/") && strings.HasSuffix(lower, ".ini")) ||
			(strings.Contains(lower, "steamapps/workshop/appworkshop_") && strings.HasSuffix(lower, ".acf")) {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeWorkshopTree}
		}
	}
	for _, f := range ctx.RelBinaryPaths() {
		lower := strings.ToLower(f)
		if strings.Contains(lower, "steamapps/workshop/content/") || strings.Contains(lower, "steamapps/workshop/appworkshop_") {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeWorkshopTree}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	repo, targetDir := "", ""
	if binding != nil {
		repo = binding.Repository
		targetDir = binding.TargetDir
	}
	textFiles := ctx.RelTextFiles()
	files := ctx.RelBinaryPaths()
	seen := make(map[string]bool)
	titles := collectPZModInfoTitles(textFiles)
	var out []corev1.InstanceBackupAddon
	add := func(id, title, archPath string) {
		if id == "" || seen[archPath+"\x00"+id] {
			return
		}
		seen[archPath+"\x00"+id] = true
		out = append(out, corev1.InstanceBackupAddon{
			Repository: repo, Package: id, Title: driver.FirstNonEmpty(title, titles[id]),
			InstallMode: corev1.AddonInstallModeWorkshopTree, TargetDir: targetDir, ArchivePath: archPath,
		})
	}
	scanPZTextWorkshops(textFiles, add)
	scanPZWorkshopDirs(files, add)
	return out
}

func collectPZModInfoTitles(textFiles map[string]string) map[string]string {
	titles := make(map[string]string)
	for p, content := range textFiles {
		if !strings.EqualFold(path.Base(p), "mod.info") {
			continue
		}
		idx := strings.Index(p, "steamapps/workshop/content/")
		if idx < 0 {
			continue
		}
		parts := strings.Split(p[idx:], "/")
		if len(parts) >= 5 {
			if m := pzModInfoNameRe.FindStringSubmatch(content); len(m) >= 2 && titles[parts[4]] == "" {
				titles[parts[4]] = strings.TrimSpace(m[1])
			}
		}
	}
	return titles
}

func scanPZTextWorkshops(textFiles map[string]string, add func(string, string, string)) {
	for _, p := range driver.SortedTextFileKeys(textFiles) {
		lower := strings.ToLower(p)
		switch {
		case strings.HasSuffix(lower, ".ini"):
			root := stripPZServerRoot(path.Dir(p))
			for _, m := range pzWorkshopLineRe.FindAllStringSubmatch(textFiles[p], -1) {
				for _, tok := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
					add(strings.TrimSpace(tok), "", root)
				}
			}
		case strings.Contains(lower, "appworkshop_") && strings.HasSuffix(lower, ".acf"):
			root := "."
			if idx := strings.Index(lower, "steamapps/"); idx > 0 {
				root = driver.FirstNonEmpty(strings.TrimSuffix(p[:idx], "/"), ".")
			}
			for _, m := range acfInstalledItemRe.FindAllStringSubmatch(textFiles[p], -1) {
				add(m[1], "", root)
			}
		}
	}
}

func scanPZWorkshopDirs(files []string, add func(string, string, string)) {
	for _, f := range files {
		idx := strings.Index(f, "steamapps/workshop/content/")
		if idx < 0 {
			continue
		}
		root := driver.FirstNonEmpty(strings.TrimSuffix(f[:idx], "/"), ".")
		parts := strings.Split(f[idx:], "/")
		if len(parts) >= 5 {
			if _, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
				add(parts[4], "", root)
			}
		}
	}
}

func stripPZServerRoot(dir string) string {
	norm := driver.NormalizeRelConfigPath(dir)
	lower := strings.ToLower(norm)
	switch {
	case lower == "zomboid/server":
		return "."
	case strings.HasSuffix(lower, "/zomboid/server"):
		return norm[:len(norm)-len("/zomboid/server")]
	case norm == "":
		return "."
	default:
		return norm
	}
}
