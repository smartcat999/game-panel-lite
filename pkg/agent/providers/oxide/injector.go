package oxide

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

const blobExtractedOkMarker = ".gp-extracted-ok"

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs uMod/Oxide frameworks and C# plugins into an Oxide target directory.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical oxide addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeOxide
}

// Inject materializes either an Oxide framework zip archive or a plugin file into ws.Root.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "oxide")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatTarGz && meta.ArchiveFormat != registryv1.ArchiveFormatZip {
		return injectSinglePlugin(ws, addon, meta, targetDir, force)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	if fwRoot := findOxideFrameworkRoot(srcDir); fwRoot != "" {
		return injectFrameworkRoot(ws, fwRoot, force)
	}
	return injectPluginArchive(ws, srcDir, targetDir, force)
}

func injectSinglePlugin(
	ws *driver.WorkspaceContext,
	addon corev1.InstanceAddon,
	meta *driver.CachedArtifactMeta,
	targetDir string,
	force bool,
) error {
	base := filepath.Base(driver.FirstNonEmpty(meta.FileName, ensurePluginExt(addon.Package)))
	relDst := targetDir + "/plugins/" + base
	if err := materializeFile(ws, ws.BlobFilePath(meta.HexDigest), relDst, force); err != nil {
		return fmt.Errorf("materialize oxide plugin %s: %w", meta.Coordinate, err)
	}
	ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, relDst)
	return nil
}

func findOxideFrameworkRoot(srcDir string) string {
	if hasOxideCore(srcDir) {
		return srcDir
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return ""
	}
	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == "." || ent.Name() == ".." {
			continue
		}
		cand := filepath.Join(srcDir, ent.Name())
		if hasOxideCore(cand) {
			return cand
		}
	}
	return ""
}

func hasOxideCore(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasSuffix(ent.Name(), "_Data") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, ent.Name(), "Managed", "Oxide.Core.dll"))
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func injectFrameworkRoot(ws *driver.WorkspaceContext, fwRoot string, force bool) error {
	return filepath.WalkDir(fwRoot, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := driver.CleanWorkspaceRelPath(relSlash(fwRoot, srcPath))
		if rel == "" || path.Base(rel) == blobExtractedOkMarker {
			return nil
		}
		if err := materializeFile(ws, srcPath, rel, force); err != nil {
			return fmt.Errorf("materialize oxide framework file %q: %w", rel, err)
		}
		ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, rel)
		return nil
	})
}

func injectPluginArchive(ws *driver.WorkspaceContext, srcDir, targetDir string, force bool) error {
	configPrefix := targetDir + "/config/"
	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(srcDir, srcPath)
		if rel == "" || shouldSkipArchiveMeta(rel) {
			return nil
		}
		dstRel := mapArchivePluginPath(rel, targetDir)
		if dstRel == "" {
			return nil
		}
		if hasFoldPrefix(dstRel, configPrefix) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize oxide archive file %q: %w", dstRel, err)
		}
		ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, dstRel)
		return nil
	})
}

func mapArchivePluginPath(rel, targetDir string) string {
	clean := driver.CleanWorkspaceRelPath(rel)
	if clean == "" {
		return ""
	}
	for _, pfx := range []string{targetDir + "/config/", "oxide/config/", "config/"} {
		if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
			return targetDir + "/config/" + clean[len(pfx):]
		}
	}
	for _, pfx := range []string{targetDir + "/plugins/", "oxide/plugins/", "plugins/"} {
		if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
			return targetDir + "/plugins/" + clean[len(pfx):]
		}
	}
	return targetDir + "/plugins/" + clean
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

func shouldSkipArchiveMeta(rel string) bool {
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

func ensurePluginExt(pkg string) string {
	trimmed := strings.TrimSpace(pkg)
	if trimmed == "" {
		return "Plugin.cs"
	}
	if filepath.Ext(trimmed) != "" {
		return trimmed
	}
	return trimmed + ".cs"
}

func hasFoldPrefix(s, pfx string) bool {
	return len(s) >= len(pfx) && strings.EqualFold(s[:len(pfx)], pfx)
}
