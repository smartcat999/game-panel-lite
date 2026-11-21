package bepinex

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

// Injector installs BepInEx framework packs and Thunderstore plugin mods into a BepInEx target directory.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical bepinex addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeBepInEx
}

// Inject materializes either a BepInEx runtime framework pack or a plugin mod archive into ws.Root.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "BepInEx")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest)
	if meta.ArchiveFormat != registryv1.ArchiveFormatTarGz && meta.ArchiveFormat != registryv1.ArchiveFormatZip {
		return injectSingleFile(ws, addon, meta, targetDir, force)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	if fwRoot := findBepInExFrameworkRoot(srcDir); fwRoot != "" {
		return injectFrameworkPack(ws, fwRoot, targetDir, force)
	}
	return injectModArchive(ws, srcDir, addon, targetDir, force)
}

func injectSingleFile(
	ws *driver.WorkspaceContext,
	addon corev1.InstanceAddon,
	meta *driver.CachedArtifactMeta,
	targetDir string,
	force bool,
) error {
	pkgDir := sanitizePkgDir(addon.Package)
	base := filepath.Base(driver.FirstNonEmpty(meta.FileName, pkgDir+".dll"))
	modRoot := targetDir + "/plugins/" + pkgDir
	relDst := modRoot + "/" + base
	if err := materializeFile(ws, ws.BlobFilePath(meta.HexDigest), relDst, force); err != nil {
		return fmt.Errorf("materialize bepinex addon %s: %w", meta.Coordinate, err)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, modRoot)
	return nil
}

func findBepInExFrameworkRoot(srcDir string) string {
	if hasPreloader(srcDir) {
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
		if hasPreloader(cand) {
			return cand
		}
	}
	return ""
}

func hasPreloader(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "BepInEx", "core", "BepInEx.Preloader.dll"))
	return err == nil && info.Mode().IsRegular()
}

func injectFrameworkPack(ws *driver.WorkspaceContext, fwRoot, targetDir string, force bool) error {
	configPrefix := targetDir + "/config/"
	return filepath.WalkDir(fwRoot, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(fwRoot, srcPath)
		if rel == "" || shouldSkipRootMetadata(rel) {
			return nil
		}
		dstRel := remapFrameworkPath(rel, targetDir)
		if dstRel == "" {
			return nil
		}
		if hasFoldPrefix(dstRel, configPrefix) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize bepinex framework file %q: %w", dstRel, err)
		}
		ws.Next.Files = append(ws.Next.Files, dstRel)
		return nil
	})
}

func injectModArchive(
	ws *driver.WorkspaceContext,
	srcDir string,
	addon corev1.InstanceAddon,
	targetDir string,
	force bool,
) error {
	pkgDir := sanitizePkgDir(addon.Package)
	configPrefix := targetDir + "/config/"
	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(srcDir, srcPath)
		if rel == "" || shouldSkipRootMetadata(rel) {
			return nil
		}
		dstRel, trackedDir := mapModArchivePath(rel, targetDir, pkgDir)
		if dstRel == "" {
			return nil
		}
		if hasFoldPrefix(dstRel, configPrefix) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize bepinex mod %s file %q: %w", addon.Package, dstRel, err)
		}
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, trackedDir)
		return nil
	})
}

func remapFrameworkPath(rel, targetDir string) string {
	clean := driver.CleanWorkspaceRelPath(rel)
	if clean == "" {
		return ""
	}
	if hasFoldPrefix(clean, "bepinex/") {
		rest := clean[len("bepinex/"):]
		return targetDir + "/" + rest
	}
	return clean
}

func mapModArchivePath(rel, targetDir, pkgDir string) (string, string) {
	clean := driver.CleanWorkspaceRelPath(rel)
	if clean == "" {
		return "", ""
	}
	if sub, ok := stripConfigPrefix(clean, targetDir); ok {
		return targetDir + "/config/" + sub, ""
	}
	for _, bucket := range []string{"plugins", "patchers", "monomod", "core"} {
		if sub, ok := stripBucketPrefix(clean, targetDir, bucket); ok {
			modRoot := targetDir + "/" + bucket + "/" + pkgDir
			return modRoot + "/" + sub, modRoot
		}
	}
	modRoot := targetDir + "/plugins/" + pkgDir
	return modRoot + "/" + clean, modRoot
}

func stripConfigPrefix(clean, targetDir string) (string, bool) {
	for _, pfx := range []string{targetDir + "/config/", "bepinex/config/", "config/"} {
		if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
			return clean[len(pfx):], true
		}
	}
	return "", false
}

func stripBucketPrefix(clean, targetDir, bucket string) (string, bool) {
	for _, pfx := range []string{targetDir + "/" + bucket + "/", "bepinex/" + bucket + "/", bucket + "/"} {
		if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
			return clean[len(pfx):], true
		}
	}
	return "", false
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

func shouldSkipRootMetadata(rel string) bool {
	base := path.Base(rel)
	if base == blobExtractedOkMarker {
		return true
	}
	if strings.Contains(rel, "/") {
		return false
	}
	low := strings.ToLower(base)
	switch low {
	case "manifest.json", "readme.md", "changelog.md", "icon.png":
		return true
	}
	return strings.HasPrefix(low, "license")
}

func sanitizePkgDir(raw string) string {
	clean := filepath.Base(strings.TrimSpace(raw))
	if clean == "" || clean == "." || clean == ".." {
		return "addon"
	}
	return clean
}

func hasFoldPrefix(s, pfx string) bool {
	return len(s) >= len(pfx) && strings.EqualFold(s[:len(pfx)], pfx)
}
