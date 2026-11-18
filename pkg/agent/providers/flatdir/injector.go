package flatdir

import (
	"fmt"
	"path/filepath"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs archives or single binaries/jars directly into a target directory (default "plugins").
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical flat_dir addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeFlatDir
}

// Inject materializes the cached addon payload into the flat target directory inside ws.Root.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "plugins")
	if meta.ArchiveFormat == registryv1.ArchiveFormatTarGz || meta.ArchiveFormat == registryv1.ArchiveFormatZip {
		return injectArchiveTree(ws, meta, targetDir)
	}
	return injectSingleFile(ws, addon, meta, targetDir)
}

func injectArchiveTree(ws *driver.WorkspaceContext, meta *driver.CachedArtifactMeta, targetDir string) error {
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	materialized, err := volume.MaterializeTreeInRoot(srcDir, ws.Root, targetDir, volume.MaterializeOptions{
		ForceReplace:   ws.ForceReplaceAddon(meta.Coordinate, meta.Digest),
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	})
	if err != nil {
		return err
	}
	for _, rel := range materialized {
		ws.Next.Files = append(ws.Next.Files, targetDir+"/"+rel)
	}
	return nil
}

func injectSingleFile(
	ws *driver.WorkspaceContext,
	addon corev1.InstanceAddon,
	meta *driver.CachedArtifactMeta,
	targetDir string,
) error {
	ext := filepath.Ext(meta.FileName)
	if ext == "" {
		ext = ".jar"
	}
	fileName := fmt.Sprintf("%s-%s%s", addon.Package, addon.Version, ext)
	relFile := targetDir + "/" + filepath.Base(fileName)
	srcBlob := ws.BlobFilePath(meta.HexDigest)
	if err := volume.MaterializeFileInRoot(srcBlob, ws.Root, relFile, volume.MaterializeOptions{
		ForceReplace:   ws.ForceReplaceAddon(meta.Coordinate, meta.Digest),
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	}); err != nil {
		return fmt.Errorf("materialize flat_dir addon %s: %w", meta.Coordinate, err)
	}
	ws.Next.Files = append(ws.Next.Files, relFile)
	return nil
}
