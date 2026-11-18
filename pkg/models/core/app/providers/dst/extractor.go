package dst

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

var (
	dstModBlockRe     = regexp.MustCompile(`\[\s*["'](workshop-(\d+))["']\s*\]\s*=\s*\{`)
	dstPlainBlockRe   = regexp.MustCompile(`(?m)\b(workshop-(\d+))\s*=\s*\{`)
	dstSetupCallRe    = regexp.MustCompile(`ServerModSetup\s*\(\s*["'](\d+)["']\s*\)`)
	dstModInfoNameRe  = regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"'\r\n]+)["']`)
	dstModInfoVerRe   = regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"'\r\n]+)["']`)
	luaEnabledFalseRe = regexp.MustCompile(`\benabled\s*=\s*false\b`)
	luaConfigOptsRe   = regexp.MustCompile(`\bconfiguration_options\s*=\s*\{`)
	luaOptionKeyRe    = regexp.MustCompile(`(?:^|[,;\r\n])\s*(?:\[\s*["'][^"']+["']\s*\]|[A-Za-z_][A-Za-z0-9_]*)\s*=`)
)

func init() {
	driver.RegisterAddonExtractor(&extractor{})
}

type extractor struct {
	driver.BaseExtractor
}

func (*extractor) Mode() string  { return corev1.AddonInstallModeDSTLocalMods }
func (*extractor) Priority() int { return 10 }

func (*extractor) NormalizePackageID(pkg string) string {
	return driver.StripWorkshopPrefix(pkg)
}

func (*extractor) InferBinding(ctx *driver.ExtractContext) *corev1.AppAddonBinding {
	for p := range ctx.RelTextFiles() {
		base := strings.ToLower(path.Base(p))
		if base == "modoverrides.lua" || base == "dedicated_server_mods_setup.lua" {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeDSTLocalMods, TargetDir: "mods"}
		}
		if base == "modinfo.lua" && extractDSTWorkshopDirID(p) != "" {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeDSTLocalMods, TargetDir: "mods"}
		}
	}
	for _, f := range ctx.RelBinaryPaths() {
		if extractDSTWorkshopDirID(f) != "" {
			return &corev1.AppAddonBinding{InstallMode: corev1.AddonInstallModeDSTLocalMods, TargetDir: "mods"}
		}
	}
	return nil
}

func (*extractor) Extract(ctx *driver.ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon {
	repo, targetDir := "", "mods"
	if binding != nil {
		repo = binding.Repository
		targetDir = driver.FirstNonEmpty(binding.TargetDir, "mods")
	}
	textFiles := ctx.RelTextFiles()
	files := ctx.RelBinaryPaths()
	merged := make(map[string]*corev1.InstanceBackupAddon)
	var order []string
	upsert := func(item corev1.InstanceBackupAddon) {
		key := item.ArchivePath + "\x00" + item.Package
		if prev, ok := merged[key]; ok {
			mergeDSTAddonRecord(prev, item)
			return
		}
		cp := item
		merged[key] = &cp
		order = append(order, key)
	}
	parseDSTModOverrides(repo, targetDir, textFiles, upsert)
	parseDSTModSetupAndInfo(repo, targetDir, files, textFiles, upsert, merged)
	out := make([]corev1.InstanceBackupAddon, 0, len(order))
	for _, k := range order {
		out = append(out, *merged[k])
	}
	return out
}

func (e *extractor) MergeDuplicate(existing *corev1.InstanceBackupAddon, incoming corev1.InstanceBackupAddon) {
	if existing == nil {
		return
	}
	existing.Repository = driver.FirstNonEmpty(existing.Repository, incoming.Repository)
	existing.Title = driver.FirstNonEmpty(existing.Title, incoming.Title)
	existing.Version = driver.FirstNonEmpty(existing.Version, incoming.Version)
	existing.TargetDir = driver.FirstNonEmpty(existing.TargetDir, incoming.TargetDir)
	existing.InstallMode = driver.FirstNonEmpty(existing.InstallMode, incoming.InstallMode)
	mergeDSTAddonRecord(existing, incoming)
}

func parseDSTModOverrides(repo, targetDir string, textFiles map[string]string, upsert func(corev1.InstanceBackupAddon)) {
	for _, archPath := range driver.SortedTextFileKeys(textFiles) {
		if !strings.EqualFold(path.Base(archPath), "modoverrides.lua") {
			continue
		}
		clusterRoot, scope := splitDSTOverrideScope(archPath)
		for _, blk := range parseDSTOverrideBlocks(textFiles[archPath]) {
			var scopes []string
			if scope != "" {
				scopes = []string{scope}
			}
			upsert(corev1.InstanceBackupAddon{
				Repository: repo, Package: blk.id, InstallMode: corev1.AddonInstallModeDSTLocalMods,
				TargetDir: targetDir, Disabled: blk.disabled, ConfigCount: blk.configCount,
				Scopes: scopes, ArchivePath: clusterRoot,
			})
		}
	}
}

func parseDSTModSetupAndInfo(repo, targetDir string, files []string, textFiles map[string]string, upsert func(corev1.InstanceBackupAddon), merged map[string]*corev1.InstanceBackupAddon) {
	infoByID := collectDSTModInfoMeta(textFiles)
	seenPkg := collectKnownDSTPackages(merged)
	for _, archPath := range driver.SortedTextFileKeys(textFiles) {
		if !strings.EqualFold(path.Base(archPath), "dedicated_server_mods_setup.lua") {
			continue
		}
		dir := path.Dir(archPath)
		for _, m := range dstSetupCallRe.FindAllStringSubmatch(textFiles[archPath], -1) {
			seenPkg[m[1]] = true
			upsert(corev1.InstanceBackupAddon{
				Repository: repo, Package: m[1], InstallMode: corev1.AddonInstallModeDSTLocalMods,
				TargetDir: targetDir, ArchivePath: dir,
			})
		}
	}
	for _, f := range files {
		if wid := extractDSTWorkshopDirID(f); wid != "" && !seenPkg[wid] {
			seenPkg[wid] = true
			upsert(corev1.InstanceBackupAddon{
				Repository: repo, Package: wid, InstallMode: corev1.AddonInstallModeDSTLocalMods,
				TargetDir: targetDir, ArchivePath: ".",
			})
		}
	}
	enrichDSTModInfo(merged, infoByID)
}

func collectKnownDSTPackages(merged map[string]*corev1.InstanceBackupAddon) map[string]bool {
	seen := make(map[string]bool, len(merged))
	for _, rec := range merged {
		seen[rec.Package] = true
	}
	return seen
}

func enrichDSTModInfo(merged map[string]*corev1.InstanceBackupAddon, infoByID map[string][2]string) {
	for _, rec := range merged {
		if meta, ok := infoByID[rec.Package]; ok {
			if rec.Title == "" {
				rec.Title = meta[0]
			}
			if rec.Version == "" {
				rec.Version = meta[1]
			}
		}
	}
}

func mergeDSTAddonRecord(prev *corev1.InstanceBackupAddon, next corev1.InstanceBackupAddon) {
	for _, s := range next.Scopes {
		if !driver.ContainsStringFold(prev.Scopes, s) {
			prev.Scopes = append(prev.Scopes, s)
		}
	}
	if len(prev.Scopes) > 1 {
		sort.Slice(prev.Scopes, func(i, j int) bool {
			return dstScopeRank(prev.Scopes[i]) < dstScopeRank(prev.Scopes[j])
		})
	}
	if next.ConfigCount > prev.ConfigCount {
		prev.ConfigCount = next.ConfigCount
	}
	switch {
	case len(prev.Scopes) > 0 && len(next.Scopes) > 0:
		prev.Disabled = prev.Disabled && next.Disabled
	case len(prev.Scopes) == 0 && len(next.Scopes) > 0:
		prev.Disabled = next.Disabled
	case len(prev.Scopes) == 0 && len(next.Scopes) == 0 && !next.Disabled:
		prev.Disabled = false
	}
}

func dstScopeRank(s string) string {
	switch strings.ToLower(s) {
	case "master":
		return "0:" + s
	case "caves":
		return "1:" + s
	default:
		return "2:" + s
	}
}

func splitDSTOverrideScope(archPath string) (string, string) {
	norm := driver.NormalizeRelConfigPath(archPath)
	parent := path.Dir(norm)
	base := path.Base(parent)
	switch {
	case strings.EqualFold(base, "Master"):
		return path.Dir(parent), "Master"
	case strings.EqualFold(base, "Caves"):
		return path.Dir(parent), "Caves"
	default:
		return parent, ""
	}
}

type dstModBlock struct {
	id          string
	disabled    bool
	configCount int
}

func parseDSTOverrideBlocks(content string) []dstModBlock {
	matches := findDSTBlockStarts(content)
	if len(matches) == 0 {
		return nil
	}
	out := make([]dstModBlock, 0, len(matches))
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		if seen[m.id] {
			continue
		}
		seen[m.id] = true
		body, ok := driver.SliceBalancedDelimBody(content[m.bodyStart:], '{', '}')
		if !ok {
			body = content[m.bodyStart:]
		}
		out = append(out, dstModBlock{
			id:          m.id,
			disabled:    luaEnabledFalseRe.MatchString(body),
			configCount: countLuaConfigOptions(body),
		})
	}
	return out
}

type dstBlockStart struct {
	id        string
	bodyStart int
}

func findDSTBlockStarts(content string) []dstBlockStart {
	type posStart struct {
		pos       int
		id        string
		bodyStart int
	}
	var all []posStart
	for _, re := range []*regexp.Regexp{dstModBlockRe, dstPlainBlockRe} {
		for _, idx := range re.FindAllStringSubmatchIndex(content, -1) {
			all = append(all, posStart{pos: idx[0], id: content[idx[4]:idx[5]], bodyStart: idx[1]})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].pos < all[j].pos })
	out := make([]dstBlockStart, 0, len(all))
	for _, p := range all {
		out = append(out, dstBlockStart{id: p.id, bodyStart: p.bodyStart})
	}
	return out
}

func countLuaConfigOptions(body string) int {
	loc := luaConfigOptsRe.FindStringIndex(body)
	if loc == nil {
		return 0
	}
	sub, ok := driver.SliceBalancedDelimBody(body[loc[1]:], '{', '}')
	if !ok {
		return 0
	}
	return len(luaOptionKeyRe.FindAllStringIndex(sub, -1))
}

func collectDSTModInfoMeta(textFiles map[string]string) map[string][2]string {
	out := make(map[string][2]string)
	for archPath, content := range textFiles {
		if !strings.EqualFold(path.Base(archPath), "modinfo.lua") {
			continue
		}
		id := extractDSTWorkshopDirID(archPath)
		if id == "" {
			continue
		}
		name, ver := "", ""
		if m := dstModInfoNameRe.FindStringSubmatch(content); len(m) >= 2 {
			name = strings.TrimSpace(m[1])
		}
		if m := dstModInfoVerRe.FindStringSubmatch(content); len(m) >= 2 {
			ver = strings.TrimSpace(m[1])
		}
		out[id] = [2]string{name, ver}
	}
	return out
}

func extractDSTWorkshopDirID(relPath string) string {
	for _, seg := range strings.Split(relPath, "/") {
		if strings.HasPrefix(strings.ToLower(seg), "workshop-") {
			id := seg[len("workshop-"):]
			if _, err := strconv.ParseInt(id, 10, 64); err == nil {
				return id
			}
		}
	}
	return ""
}
