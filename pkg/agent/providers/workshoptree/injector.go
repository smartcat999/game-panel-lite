package workshoptree

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const stateKey = corev1.AddonInstallModeWorkshopTree + ":pz"

type pzModConfig struct {
	config map[string]string
	schema []registryv1.AddonConfigField
}

type pzAddonState struct {
	workshopIDs []string
	modIDs      []string
	mapFolders  []string
	configs     []pzModConfig
}

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs Steam Workshop item directory trees (Project Zomboid layout) and updates ACF/INI configs.
type Injector struct{}

// Mode returns the canonical workshop_tree addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeWorkshopTree
}

// Inject materializes an extracted workshop directory and records WorkshopItems, Mods, and Map folders.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "steamapps/workshop/content/108600")
	wid := filepath.Base(strings.TrimSpace(addon.Package))
	itemSubDir := targetDir + "/" + wid
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	materialized, err := volume.MaterializeTreeInRoot(srcDir, ws.Root, itemSubDir, volume.MaterializeOptions{
		ForceReplace:   ws.ForceReplaceAddon(meta.Coordinate, meta.Digest),
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	})
	if err != nil {
		return fmt.Errorf("materialize workshop_tree mod %s: %w", meta.Coordinate, err)
	}
	if len(materialized) == 0 {
		return fmt.Errorf("workshop_tree mod %s extracted zero files", meta.Coordinate)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, itemSubDir)
	recordPZMetadata(ws, wid, srcDir, addon, meta)
	return nil
}

// Finalize writes appworkshop_108600.acf and synchronizes WorkshopItems/Mods/Map in Zomboid Server INIs.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	pz := driver.GetOrInitState(ws, stateKey, func() *pzAddonState { return &pzAddonState{} })
	return writePZWorkshopConfigs(ws.Root, ws.Prev.MapFolders, pz)
}

// Cleanup clears WorkshopItems/Mods and removes stale addon Map entries from Zomboid Server INIs.
func (*Injector) Cleanup(root *os.Root, prev driver.AddonsManifest) {
	if root == nil {
		return
	}
	_ = writePZWorkshopConfigs(root, prev.MapFolders, &pzAddonState{})
}

func recordPZMetadata(ws *driver.WorkspaceContext, wid, srcDir string, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) {
	pz := driver.GetOrInitState(ws, stateKey, func() *pzAddonState { return &pzAddonState{} })
	pz.workshopIDs = driver.AppendUniqueStr(pz.workshopIDs, wid)
	if len(addon.Config) > 0 {
		pz.configs = append(pz.configs, pzModConfig{
			config: maps.Clone(addon.Config),
			schema: meta.ConfigSchema,
		})
	}
	modIDs, mapFolders := meta.ModIDs, meta.MapFolders
	if len(modIDs) == 0 && len(mapFolders) == 0 {
		modIDs, mapFolders = scanPZExtractedDir(srcDir)
	}
	for _, id := range modIDs {
		pz.modIDs = driver.AppendUniqueStr(pz.modIDs, id)
	}
	for _, mf := range mapFolders {
		pz.mapFolders = driver.AppendUniqueStr(pz.mapFolders, mf)
		ws.Next.MapFolders = driver.AppendUniqueStr(ws.Next.MapFolders, mf)
	}
}

func writePZWorkshopConfigs(root *os.Root, prevMaps []string, pz *pzAddonState) error {
	if err := volume.WriteFileAtomicInRoot(root, "steamapps/workshop/appworkshop_108600.acf", buildPZWorkshopACF(pz.workshopIDs), 0o644); err != nil {
		return err
	}
	serverRelDir, err := volume.EnsureSafeDirInRoot(root, "Zomboid/Server")
	if err != nil {
		return err
	}
	iniRelFiles, err := listServerINIFiles(root, serverRelDir)
	if err != nil {
		return err
	}
	for _, iniRelPath := range iniRelFiles {
		if err := patchPZServerINI(root, iniRelPath, prevMaps, pz); err != nil {
			return err
		}
	}
	return writePZSandboxVarsConfigs(root, serverRelDir, iniRelFiles, pz.configs)
}

func listServerINIFiles(root *os.Root, serverRelDir string) ([]string, error) {
	entries, err := volume.ListRegularFilesInRoot(root, serverRelDir)
	if err != nil {
		return nil, err
	}
	var iniFiles []string
	for _, name := range entries {
		if strings.HasSuffix(strings.ToLower(name), ".ini") {
			iniFiles = append(iniFiles, serverRelDir+"/"+name)
		}
	}
	if len(iniFiles) == 0 {
		return []string{serverRelDir + "/servertest.ini"}, nil
	}
	return iniFiles, nil
}

func buildPZWorkshopACF(workshopIDs []string) []byte {
	var b strings.Builder
	b.WriteString("\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"108600\"\n\t\"WorkshopItemsInstalled\"\n\t{\n")
	for _, id := range workshopIDs {
		fmt.Fprintf(&b, "\t\t%q\n\t\t{\n\t\t\t\"size\"\t\t\"1\"\n\t\t}\n", id)
	}
	b.WriteString("\t}\n}\n")
	return []byte(b.String())
}

func patchPZServerINI(root *os.Root, iniRelPath string, prevMaps []string, pz *pzAddonState) error {
	raw, err := volume.ReadOptionalFileInRoot(root, iniRelPath)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	wVal := strings.Join(pz.workshopIDs, ";")
	mVal := strings.Join(pz.modIDs, ";")
	hasW, hasM, hasMap := false, false, false
	for i, line := range lines {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "WorkshopItems":
			lines[i], hasW = "WorkshopItems="+wVal, true
		case "Mods":
			lines[i], hasM = "Mods="+mVal, true
		case "Map":
			lines[i], hasMap = "Map="+mergePZMapValue(v, prevMaps, pz.mapFolders), true
		}
	}
	if !hasMap {
		lines = append(lines, "Map="+mergePZMapValue("Muldraugh, KY", prevMaps, pz.mapFolders))
	}
	if !hasW {
		lines = append(lines, "WorkshopItems="+wVal)
	}
	if !hasM {
		lines = append(lines, "Mods="+mVal)
	}
	return volume.WriteFileAtomicInRoot(root, iniRelPath, []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n"), 0o644)
}

func mergePZMapValue(existing string, prevMaps, nextMaps []string) string {
	var base []string
	for _, part := range strings.Split(existing, ";") {
		p := strings.TrimSpace(part)
		if p != "" && !slices.Contains(prevMaps, p) && !slices.Contains(nextMaps, p) {
			base = driver.AppendUniqueStr(base, p)
		}
	}
	if len(base) == 0 {
		base = []string{"Muldraugh, KY"}
	}
	merged := append([]string(nil), nextMaps...)
	for _, b := range base {
		merged = driver.AppendUniqueStr(merged, b)
	}
	return strings.Join(merged, ";")
}

func scanPZExtractedDir(srcDir string) ([]string, []string) {
	var modIDs, mapFolders []string
	_ = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		slash, _ := filepath.Rel(srcDir, p)
		slash = filepath.ToSlash(slash)
		if !d.IsDir() && d.Type().IsRegular() && strings.EqualFold(d.Name(), "mod.info") {
			modIDs = parsePZModInfoFile(p, modIDs)
		}
		if d.IsDir() && strings.Contains(strings.ToLower(slash), "/media/maps/") {
			mapFolders = appendPZMapFolder(strings.Split(slash, "/"), mapFolders)
		}
		return nil
	})
	return modIDs, mapFolders
}

func parsePZModInfoFile(path string, modIDs []string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return modIDs
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(ln, "="); ok && strings.EqualFold(strings.TrimSpace(k), "id") {
			modIDs = driver.AppendUniqueStr(modIDs, strings.TrimSpace(v))
		}
	}
	return modIDs
}

func appendPZMapFolder(parts, mapFolders []string) []string {
	for idx := 0; idx+2 < len(parts); idx++ {
		if strings.EqualFold(parts[idx], "media") && strings.EqualFold(parts[idx+1], "maps") && idx+2 == len(parts)-1 {
			mapFolders = driver.AppendUniqueStr(mapFolders, parts[idx+2])
		}
	}
	return mapFolders
}
