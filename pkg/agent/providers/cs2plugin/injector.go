package cs2plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	blobExtractedOkMarker = ".gp-extracted-ok"
	stateKey              = corev1.AddonInstallModeCS2Plugin + ":state"
)

type cs2PluginState struct {
	hasMetamod bool
}

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs Metamod:Source 2.x, CounterStrikeSharp runtime, and C# plugins into CS2 game/csgo/addons.
type Injector struct{}

// Mode returns the canonical cs2_plugin addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeCS2Plugin
}

// Inject materializes a Metamod, CounterStrikeSharp, or CS2 plugin archive into ws.Root.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "game/csgo/addons")
	csgoRoot := resolveCSGORoot(targetDir)
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatZip && meta.ArchiveFormat != registryv1.ArchiveFormatTarGz {
		return fmt.Errorf("cs2_plugin addon %s requires zip or tar.gz archive, got %q", meta.Coordinate, meta.ArchiveFormat)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	pluginName := derivePluginFolderName(addon.Package)
	st := driver.GetOrInitState(ws, stateKey, func() *cs2PluginState { return &cs2PluginState{} })
	if strings.EqualFold(strings.TrimSpace(addon.Package), "metamod") || containsMetamodTree(srcDir) {
		st.hasMetamod = true
	}
	return walkAndMaterializeCS2Archive(ws, srcDir, csgoRoot, targetDir, pluginName, force)
}

// Finalize ensures game/csgo/gameinfo.gi reflects whether Metamod is currently active.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	st := driver.GetOrInitState(ws, stateKey, func() *cs2PluginState { return &cs2PluginState{} })
	if st.hasMetamod || hasMetamodOnDisk(ws.Root) {
		return patchGameInfoMetamod(ws.Root)
	}
	return unpatchGameInfoMetamod(ws.Root)
}

// Cleanup strips the Metamod search path entry from game/csgo/gameinfo.gi when all cs2_plugin addons are removed.
func (*Injector) Cleanup(root *os.Root, _ driver.AddonsManifest) {
	_ = unpatchGameInfoMetamod(root)
}

func walkAndMaterializeCS2Archive(ws *driver.WorkspaceContext, srcDir, csgoRoot, addonsDir, pluginName string, force bool) error {
	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(srcDir, srcPath)
		if rel == "" || shouldSkipMeta(rel) {
			return nil
		}
		dstRel := mapCS2ArchivePath(rel, csgoRoot, addonsDir, pluginName)
		if dstRel == "" {
			return nil
		}
		if isPreservedConfigPath(dstRel) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize cs2 plugin file %q: %w", dstRel, err)
		}
		ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, dstRel)
		return nil
	})
}

func resolveCSGORoot(addonsTargetDir string) string {
	clean := strings.Trim(driver.CleanWorkspaceRelPath(addonsTargetDir), "/")
	if before, ok := strings.CutSuffix(clean, "/addons"); ok && before != "" {
		return before
	}
	if clean == "" || clean == "addons" {
		return "game/csgo"
	}
	return clean
}

func derivePluginFolderName(pkg string) string {
	trimmed := strings.TrimSpace(pkg)
	if _, repo, ok := strings.Cut(trimmed, "--"); ok && strings.TrimSpace(repo) != "" {
		trimmed = strings.TrimSpace(repo)
	}
	if trimmed == "" || trimmed == "." {
		return "Plugin"
	}
	return filepath.Base(trimmed)
}

func mapCS2ArchivePath(rel, csgoRoot, addonsDir, pluginName string) string {
	clean := driver.CleanWorkspaceRelPath(rel)
	if clean == "" {
		return ""
	}
	if idx := findFoldSubpathIndex(clean, "game/csgo/"); idx >= 0 {
		if rest := clean[idx+len("game/csgo/"):]; rest != "" {
			return csgoRoot + "/" + rest
		}
	}
	for _, seg := range []struct{ pfx, dst string }{
		{"csgo/", csgoRoot + "/"},
		{"addons/", addonsDir + "/"},
		{"cfg/", csgoRoot + "/cfg/"},
		{"counterstrikesharp/", addonsDir + "/counterstrikesharp/"},
		{"plugins/", addonsDir + "/counterstrikesharp/plugins/"},
	} {
		if hasFoldPrefix(clean, seg.pfx) && len(clean) > len(seg.pfx) {
			return seg.dst + clean[len(seg.pfx):]
		}
	}
	return addonsDir + "/counterstrikesharp/plugins/" + pluginName + "/" + clean
}

func findFoldSubpathIndex(clean, sub string) int {
	low := strings.ToLower(clean)
	if strings.HasPrefix(low, sub) {
		return 0
	}
	if pos := strings.Index(low, "/"+sub); pos >= 0 {
		return pos + 1
	}
	return -1
}

func isPreservedConfigPath(dstRel string) bool {
	low := strings.ToLower(dstRel)
	if strings.Contains(low, "/counterstrikesharp/configs/") && !strings.HasSuffix(low, ".example.json") {
		return true
	}
	return strings.HasPrefix(low, "game/csgo/cfg/") && strings.Contains(low[len("game/csgo/cfg/"):], "/")
}

func containsMetamodTree(dir string) bool {
	for _, cand := range []string{
		filepath.Join(dir, "addons", "metamod", "bin"),
		filepath.Join(dir, "game", "csgo", "addons", "metamod", "bin"),
	} {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func materializeConfigIfAbsent(ws *driver.WorkspaceContext, srcPath, dstRel string) error {
	if _, err := ws.Root.Lstat(filepath.FromSlash(dstRel)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return materializeFile(ws, srcPath, dstRel, false)
}

func materializeFile(ws *driver.WorkspaceContext, srcPath, relDst string, force bool) error {
	return volume.MaterializeFileInRoot(srcPath, ws.Root, relDst, volume.MaterializeOptions{
		ForceReplace:   force,
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	})
}

func relSlash(rootDir, srcPath string) string {
	rel, err := filepath.Rel(rootDir, srcPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func shouldSkipMeta(rel string) bool {
	base := path.Base(rel)
	if base == blobExtractedOkMarker {
		return true
	}
	if strings.Contains(rel, "/") {
		return false
	}
	low := strings.ToLower(base)
	return low == "readme.md" || low == "changelog.md" || strings.HasPrefix(low, "license")
}

func hasFoldPrefix(s, pfx string) bool {
	return len(s) >= len(pfx) && strings.EqualFold(s[:len(pfx)], pfx)
}
