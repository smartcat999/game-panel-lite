package sevendays

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
	canonicalModInfoName  = "ModInfo.xml"
	harmonyModFolder      = "0_TFP_Harmony"
)

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs 7 Days to Die modlets into Mods/<ModFolder>/ with exact ModInfo.xml casing for Linux.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical 7dtd_mod addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeSevenDaysMod
}

// Inject materializes a 7DTD modlet archive into <targetDir>/<ModFolder>/.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "Mods")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatZip && meta.ArchiveFormat != registryv1.ArchiveFormatTarGz {
		return fmt.Errorf("7dtd addon %s requires zip or tar.gz archive, got %q", meta.Coordinate, meta.ArchiveFormat)
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	modRoots := findModInfoDirs(srcDir)
	if len(modRoots) > 0 {
		return injectDetectedModRoots(ws, srcDir, modRoots, addon, targetDir, force)
	}
	return injectFallbackTree(ws, srcDir, addon, targetDir, force)
}

func findModInfoDirs(srcDir string) []string {
	var found []string
	_ = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != srcDir && containsFoundAncestor(srcDir, p, found) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), canonicalModInfoName) {
			rel, rErr := filepath.Rel(srcDir, filepath.Dir(p))
			if rErr == nil {
				found = append(found, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(found)
	return found
}

func containsFoundAncestor(root, cur string, found []string) bool {
	rel, err := filepath.Rel(root, cur)
	if err != nil {
		return false
	}
	slash := filepath.ToSlash(rel)
	for _, f := range found {
		if f == "." || slash == f || strings.HasPrefix(slash, f+"/") {
			return true
		}
	}
	return false
}

func injectDetectedModRoots(
	ws *driver.WorkspaceContext,
	srcDir string,
	modRoots []string,
	addon corev1.InstanceAddon,
	targetDir string,
	force bool,
) error {
	for _, relRoot := range modRoots {
		srcSub := srcDir
		modName := sanitizeModDirName(addon.Package)
		if relRoot != "." {
			srcSub = filepath.Join(srcDir, filepath.FromSlash(relRoot))
			modName = sanitizeModDirName(path.Base(relRoot))
		}
		if strings.EqualFold(modName, harmonyModFolder) {
			continue
		}
		dstDir := targetDir + "/" + modName
		if err := copyModDir(ws, srcSub, dstDir, force); err != nil {
			return fmt.Errorf("materialize 7dtd mod %s to %s: %w", addon.Package, dstDir, err)
		}
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, dstDir)
	}
	return nil
}

func injectFallbackTree(ws *driver.WorkspaceContext, srcDir string, addon corev1.InstanceAddon, targetDir string, force bool) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	hasDir := false
	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == "." || ent.Name() == ".." || strings.EqualFold(ent.Name(), harmonyModFolder) {
			continue
		}
		hasDir = true
		dstDir := targetDir + "/" + sanitizeModDirName(ent.Name())
		if err := copyModDir(ws, filepath.Join(srcDir, ent.Name()), dstDir, force); err != nil {
			return fmt.Errorf("materialize 7dtd mod dir %s: %w", ent.Name(), err)
		}
		ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, dstDir)
	}
	if hasDir {
		return nil
	}
	dstDir := targetDir + "/" + sanitizeModDirName(addon.Package)
	if err := copyModDir(ws, srcDir, dstDir, force); err != nil {
		return fmt.Errorf("materialize 7dtd mod %s: %w", addon.Package, err)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, dstDir)
	return nil
}

func copyModDir(ws *driver.WorkspaceContext, srcSub, dstDir string, force bool) error {
	count := 0
	err := filepath.WalkDir(srcSub, func(srcPath string, d fs.DirEntry, wErr error) error {
		if wErr != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return wErr
		}
		rel, rErr := filepath.Rel(srcSub, srcPath)
		if rErr != nil {
			return nil
		}
		clean := driver.CleanWorkspaceRelPath(filepath.ToSlash(rel))
		if clean == "" || path.Base(clean) == blobExtractedOkMarker {
			return nil
		}
		if strings.EqualFold(path.Base(clean), canonicalModInfoName) {
			clean = path.Join(path.Dir(clean), canonicalModInfoName)
			clean = strings.TrimPrefix(clean, "./")
		}
		dstRel := dstDir + "/" + clean
		if err := volume.MaterializeFileInRoot(srcPath, ws.Root, dstRel, volume.MaterializeOptions{
			ForceReplace: force, RequireReflink: ws.RequireReflink, DisableReflink: ws.DisableReflink,
		}); err != nil {
			return err
		}
		count++
		return nil
	})
	if err == nil && count == 0 {
		return fmt.Errorf("mod archive %s contained zero files", dstDir)
	}
	return err
}

func sanitizeModDirName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if _, repo, ok := strings.Cut(trimmed, "--"); ok && strings.TrimSpace(repo) != "" {
		trimmed = repo
	}
	clean := filepath.Base(trimmed)
	if clean == "" || clean == "." || clean == ".." {
		return "Mod"
	}
	return clean
}
