package palworld

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	blobExtractedOkMarker = ".gp-extracted-ok"
	stateKeyPalworldLua   = "palworld_lua_mods"
	modsTxtRelPath        = "Pal/Binaries/Linux/Mods/mods.txt"
	managedBeginMarker    = "; BEGIN GAMEPANEL MANAGED MODS"
	managedEndMarker      = "; END GAMEPANEL MANAGED MODS"
)

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

type palworldLuaState struct {
	mods []string
}

// Injector installs Palworld .pak/LogicMods assets, native Linux UE4SS runtime, and Lua script mods.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical palworld_mod addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModePalworldMod
}

// Inject materializes a raw .pak mod or Thunderstore/GitHub Palworld mod archive into Linux server paths.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	pakBase := driver.ResolveTargetDir(addon.TargetDir, "Pal/Content/Paks/~mods")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatZip && meta.ArchiveFormat != registryv1.ArchiveFormatTarGz {
		return injectRawPak(ws, addon, meta, pakBase, force)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	state := driver.GetOrInitState(ws, stateKeyPalworldLua, func() *palworldLuaState { return &palworldLuaState{} })
	defaultModName := sanitizeModSeg(addon.Package)
	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, wErr error) error {
		if wErr != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return wErr
		}
		rel := relSlash(srcDir, srcPath)
		if rel == "" || shouldSkipArchiveFile(rel) {
			return nil
		}
		return routeAndMaterialize(ws, state, srcPath, rel, defaultModName, pakBase, force)
	})
}

func injectRawPak(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta, pakBase string, force bool) error {
	base := filepath.Base(driver.FirstNonEmpty(meta.FileName, sanitizeModSeg(addon.Package)+".pak"))
	dstRel := pakBase + "/" + base
	if err := materializeFile(ws, ws.BlobFilePath(meta.HexDigest), dstRel, force); err != nil {
		return fmt.Errorf("materialize palworld pak %s: %w", meta.Coordinate, err)
	}
	ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, dstRel)
	return nil
}

func routeAndMaterialize(ws *driver.WorkspaceContext, state *palworldLuaState, srcPath, rel, defaultMod, pakBase string, force bool) error {
	route := classifyPalworldFile(rel, defaultMod, pakBase)
	if route.dstRel == "" {
		return nil
	}
	if route.preserveIfPresent {
		if err := materializeConfigIfAbsent(ws, srcPath, route.dstRel); err != nil {
			return err
		}
	} else if err := materializeFile(ws, srcPath, route.dstRel, force); err != nil {
		return fmt.Errorf("materialize palworld file %q: %w", route.dstRel, err)
	}
	if route.trackedDir != "" {
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, route.trackedDir)
		if route.luaModName != "" && !slices.Contains(state.mods, route.luaModName) {
			state.mods = append(state.mods, route.luaModName)
		}
		return nil
	}
	ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, route.dstRel)
	return nil
}

type fileRoute struct {
	dstRel            string
	trackedDir        string
	luaModName        string
	preserveIfPresent bool
}

func classifyPalworldFile(rel, defaultMod, pakBase string) fileRoute {
	segs := strings.Split(rel, "/")
	if idx := indexFold(segs, "logicmods"); idx >= 0 && idx+1 < len(segs) {
		rest := strings.Join(segs[idx+1:], "/")
		return fileRoute{dstRel: "Pal/Content/Paks/LogicMods/" + rest, preserveIfPresent: isConfigExt(rest)}
	}
	if idx := indexFold(segs, "~mods", "paks", "pak"); idx >= 0 && idx+1 < len(segs) {
		rest := strings.Join(segs[idx+1:], "/")
		return fileRoute{dstRel: pakBase + "/" + rest, preserveIfPresent: isConfigExt(rest)}
	}
	if r, ok := classifyUE4SSFile(segs, defaultMod); ok {
		return r
	}
	low := strings.ToLower(path.Base(rel))
	if strings.HasSuffix(low, ".pak") || strings.HasSuffix(low, ".ucas") || strings.HasSuffix(low, ".utoc") {
		return fileRoute{dstRel: pakBase + "/" + path.Base(rel)}
	}
	return fileRoute{}
}

func classifyUE4SSFile(segs []string, defaultMod string) (fileRoute, bool) {
	base := path.Base(segs[len(segs)-1])
	if strings.EqualFold(base, "libUE4SS.so") {
		return fileRoute{dstRel: "Pal/Binaries/Linux/libUE4SS.so"}, true
	}
	if strings.EqualFold(base, "UE4SS-settings.ini") {
		return fileRoute{dstRel: "Pal/Binaries/Linux/UE4SS-settings.ini", preserveIfPresent: true}, true
	}
	if idx := indexFold(segs, "mods"); idx >= 0 && idx+1 < len(segs) {
		modName := segs[idx+1]
		if len(segs) == idx+2 {
			return fileRoute{}, true // skip standalone Mods/mods.txt template
		}
		rest := normalizeLuaSubpath(strings.Join(segs[idx+2:], "/"))
		modDir := "Pal/Binaries/Linux/Mods/" + modName
		luaName := modName
		if strings.EqualFold(modName, "shared") {
			luaName = ""
		}
		return fileRoute{dstRel: modDir + "/" + rest, trackedDir: modDir, luaModName: luaName, preserveIfPresent: isConfigExt(rest)}, true
	}
	if strings.EqualFold(segs[0], "mod") && len(segs) > 1 {
		rest := normalizeLuaSubpath(strings.Join(segs[1:], "/"))
		modDir := "Pal/Binaries/Linux/Mods/" + defaultMod
		return fileRoute{dstRel: modDir + "/" + rest, trackedDir: modDir, luaModName: defaultMod, preserveIfPresent: isConfigExt(rest)}, true
	}
	return fileRoute{}, false
}

func normalizeLuaSubpath(rest string) string {
	parts := strings.Split(rest, "/")
	for i, p := range parts {
		switch strings.ToLower(p) {
		case "scripts":
			parts[i] = "scripts"
		case "enabled.txt":
			parts[i] = "enabled.txt"
		case "libs":
			parts[i] = "libs"
		}
	}
	return strings.Join(parts, "/")
}

// Finalize merges active GamePanel-managed Lua mod entries into Pal/Binaries/Linux/Mods/mods.txt.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	state := driver.GetOrInitState(ws, stateKeyPalworldLua, func() *palworldLuaState { return &palworldLuaState{} })
	return syncModsTxtManagedBlock(ws.Root, state.mods)
}

// Cleanup strips the GamePanel-managed block from Pal/Binaries/Linux/Mods/mods.txt when all Palworld mods are removed.
func (*Injector) Cleanup(root *os.Root, _ driver.AddonsManifest) {
	_ = syncModsTxtManagedBlock(root, nil)
}

func syncModsTxtManagedBlock(root *os.Root, mods []string) error {
	if root == nil {
		return nil
	}
	sorted := slices.Clone(mods)
	slices.Sort(sorted)
	raw, err := volume.ReadFileInRoot(root, modsTxtRelPath)
	if err != nil {
		if !os.IsNotExist(err) || len(sorted) == 0 {
			return nil
		}
	}
	next := rewriteManagedBlock(string(raw), sorted)
	if strings.TrimSpace(next) == "" && len(sorted) == 0 {
		_ = volume.RemoveAllInRoot(root, modsTxtRelPath)
		return nil
	}
	return volume.WriteFileAtomicInRoot(root, modsTxtRelPath, []byte(next), 0o644)
}

func rewriteManagedBlock(existing string, sortedMods []string) string {
	var preserved []string
	inBlock := false
	for _, ln := range strings.Split(strings.ReplaceAll(existing, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == managedBeginMarker {
			inBlock = true
			continue
		}
		if trimmed == managedEndMarker {
			inBlock = false
			continue
		}
		if !inBlock && trimmed != "" {
			preserved = append(preserved, ln)
		}
	}
	if len(sortedMods) > 0 {
		preserved = append(preserved, managedBeginMarker)
		for _, m := range sortedMods {
			preserved = append(preserved, m+" : 1")
		}
		preserved = append(preserved, managedEndMarker)
	}
	if len(preserved) == 0 {
		return ""
	}
	return strings.Join(preserved, "\n") + "\n"
}

func shouldSkipArchiveFile(rel string) bool {
	base := path.Base(rel)
	if base == blobExtractedOkMarker {
		return true
	}
	low := strings.ToLower(base)
	for _, ext := range []string{".dll", ".exe", ".pdb"} {
		if strings.HasSuffix(low, ext) {
			return true
		}
	}
	if strings.Contains(rel, "/") {
		return false
	}
	switch low {
	case "manifest.json", "readme.md", "changelog.md", "icon.png":
		return true
	}
	return strings.HasPrefix(low, "license")
}

func indexFold(segs []string, targets ...string) int {
	for i, s := range segs {
		for _, t := range targets {
			if strings.EqualFold(s, t) {
				return i
			}
		}
	}
	return -1
}

func isConfigExt(rel string) bool {
	low := strings.ToLower(rel)
	return strings.HasSuffix(low, ".json") || strings.HasSuffix(low, ".ini") || strings.HasSuffix(low, ".cfg")
}

func sanitizeModSeg(raw string) string {
	clean := filepath.Base(strings.TrimSpace(raw))
	if clean == "" || clean == "." || clean == ".." {
		return "Mod"
	}
	return clean
}

func relSlash(rootDir, srcPath string) string {
	rel, err := filepath.Rel(rootDir, srcPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func materializeConfigIfAbsent(ws *driver.WorkspaceContext, srcPath, dstRel string) error {
	if _, err := ws.Root.Lstat(filepath.FromSlash(dstRel)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return materializeFile(ws, srcPath, dstRel, false)
}

func materializeFile(ws *driver.WorkspaceContext, srcPath, dstRel string, force bool) error {
	return volume.MaterializeFileInRoot(srcPath, ws.Root, dstRel, volume.MaterializeOptions{
		ForceReplace: force, RequireReflink: ws.RequireReflink, DisableReflink: ws.DisableReflink,
	})
}
