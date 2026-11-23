package corekeeper

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	blobExtractedOkMarker = ".gp-extracted-ok"
	modManifestName       = "ModManifest.json"
	bepInExDir            = "BepInEx"
)

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs Core Keeper ModSDK packages (ModManifest.json) and Thunderstore BepInEx framework/plugins.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical corekeeper_mod addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeCoreKeeperMod
}

// Inject detects ModManifest.json vs BepInEx archives and materializes files into the Core Keeper workspace.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	nativeTarget := driver.ResolveTargetDir(addon.TargetDir, "CoreKeeperServer_Data/StreamingAssets/Mods")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatZip && meta.ArchiveFormat != registryv1.ArchiveFormatTarGz {
		return injectSingleBepInExPlugin(ws, addon, meta, force)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	if nativeRoots := findModManifestDirs(srcDir); len(nativeRoots) > 0 {
		return injectNativeMods(ws, srcDir, nativeRoots, addon, nativeTarget, force)
	}
	if fwRoot := findBepInExFrameworkRoot(srcDir); fwRoot != "" {
		return injectBepInExFramework(ws, fwRoot, force)
	}
	return injectBepInExModArchive(ws, srcDir, addon, force)
}

func findModManifestDirs(srcDir string) []string {
	var found []string
	_ = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), modManifestName) {
			if rel, rErr := filepath.Rel(srcDir, filepath.Dir(p)); rErr == nil {
				found = append(found, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(found)
	return found
}

func injectNativeMods(ws *driver.WorkspaceContext, srcDir string, modRoots []string, addon corev1.InstanceAddon, nativeTarget string, force bool) error {
	for _, relRoot := range modRoots {
		srcSub := srcDir
		modName := sanitizePkgSeg(addon.Package)
		if relRoot != "." {
			srcSub = filepath.Join(srcDir, filepath.FromSlash(relRoot))
			modName = sanitizePkgSeg(path.Base(relRoot))
		}
		dstDir := nativeTarget + "/" + modName
		if err := copyNativeModDir(ws, srcSub, dstDir, force); err != nil {
			return fmt.Errorf("materialize core keeper mod %s to %s: %w", addon.Package, dstDir, err)
		}
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, dstDir)
	}
	return nil
}

func copyNativeModDir(ws *driver.WorkspaceContext, srcSub, dstDir string, force bool) error {
	return filepath.WalkDir(srcSub, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(srcSub, srcPath)
		clean := driver.CleanWorkspaceRelPath(rel)
		if clean == "" || path.Base(clean) == blobExtractedOkMarker {
			return nil
		}
		if strings.EqualFold(path.Base(clean), modManifestName) {
			clean = strings.TrimPrefix(path.Join(path.Dir(clean), modManifestName), "./")
		}
		return materializeFile(ws, srcPath, dstDir+"/"+clean, force)
	})
}

func injectSingleBepInExPlugin(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta, force bool) error {
	pkgDir := sanitizePkgSeg(addon.Package)
	base := filepath.Base(driver.FirstNonEmpty(meta.FileName, pkgDir+".dll"))
	modRoot := bepInExDir + "/plugins/" + pkgDir
	if err := materializeFile(ws, ws.BlobFilePath(meta.HexDigest), modRoot+"/"+base, force); err != nil {
		return fmt.Errorf("materialize core keeper bepinex addon %s: %w", meta.Coordinate, err)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, modRoot)
	return nil
}

func findBepInExFrameworkRoot(srcDir string) string {
	if hasBepInExPreloader(srcDir) {
		return srcDir
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return ""
	}
	for _, ent := range entries {
		if ent.IsDir() && ent.Name() != "." && ent.Name() != ".." {
			if cand := filepath.Join(srcDir, ent.Name()); hasBepInExPreloader(cand) {
				return cand
			}
		}
	}
	return ""
}

func hasBepInExPreloader(dir string) bool {
	for _, name := range []string{"BepInEx.Preloader.dll", "BepInEx.Preloader.Core.dll"} {
		info, err := os.Stat(filepath.Join(dir, bepInExDir, "core", name))
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func injectBepInExFramework(ws *driver.WorkspaceContext, fwRoot string, force bool) error {
	cfgPfx := bepInExDir + "/config/"
	return filepath.WalkDir(fwRoot, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := driver.CleanWorkspaceRelPath(relSlash(fwRoot, srcPath))
		if rel == "" || shouldSkipRootMeta(rel) {
			return nil
		}
		dstRel := rel
		if hasFoldPrefix(rel, "bepinex/") {
			dstRel = bepInExDir + "/" + rel[len("bepinex/"):]
		}
		if hasFoldPrefix(dstRel, cfgPfx) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize core keeper bepinex framework file %q: %w", dstRel, err)
		}
		ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, dstRel)
		return nil
	})
}

func injectBepInExModArchive(ws *driver.WorkspaceContext, srcDir string, addon corev1.InstanceAddon, force bool) error {
	pkgDir := sanitizePkgSeg(addon.Package)
	cfgPfx := bepInExDir + "/config/"
	return filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return err
		}
		rel := relSlash(srcDir, srcPath)
		if rel == "" || shouldSkipRootMeta(rel) {
			return nil
		}
		dstRel, trackedDir := mapBepInExArchivePath(rel, pkgDir)
		if dstRel == "" {
			return nil
		}
		if hasFoldPrefix(dstRel, cfgPfx) {
			return materializeConfigIfAbsent(ws, srcPath, dstRel)
		}
		if err := materializeFile(ws, srcPath, dstRel, force); err != nil {
			return fmt.Errorf("materialize core keeper mod %s file %q: %w", addon.Package, dstRel, err)
		}
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, trackedDir)
		return nil
	})
}

func mapBepInExArchivePath(rel, pkgDir string) (string, string) {
	clean := driver.CleanWorkspaceRelPath(rel)
	if clean == "" {
		return "", ""
	}
	for _, pfx := range []string{"bepinex/config/", "config/"} {
		if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
			return bepInExDir + "/config/" + clean[len(pfx):], ""
		}
	}
	for _, bucket := range []string{"plugins", "patchers", "monomod", "core"} {
		for _, pfx := range []string{"bepinex/" + bucket + "/", bucket + "/"} {
			if hasFoldPrefix(clean, pfx) && len(clean) > len(pfx) {
				modRoot := bepInExDir + "/" + bucket + "/" + pkgDir
				return modRoot + "/" + clean[len(pfx):], modRoot
			}
		}
	}
	modRoot := bepInExDir + "/plugins/" + pkgDir
	return modRoot + "/" + clean, modRoot
}

func shouldSkipRootMeta(rel string) bool {
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

func relSlash(rootDir, srcPath string) string {
	rel, err := filepath.Rel(rootDir, srcPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func sanitizePkgSeg(raw string) string {
	clean := filepath.Base(strings.TrimSpace(raw))
	if clean == "" || clean == "." || clean == ".." {
		return "addon"
	}
	return clean
}

func hasFoldPrefix(s, pfx string) bool {
	return len(s) >= len(pfx) && strings.EqualFold(s[:len(pfx)], pfx)
}
