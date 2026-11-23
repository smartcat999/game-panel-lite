package sml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs Satisfactory Mod Loader (SML) and .smod archives into FactoryGame/Mods/<ModRef>.
type Injector struct {
	driver.BaseInjector
}

// Mode returns the canonical sml addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeSML
}

// Inject materializes an SML or .smod mod package into <targetDir>/<ModReference>/.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "FactoryGame/Mods")
	force := ws.ForceReplaceAddon(meta.Coordinate, meta.Digest) || ws.ServerReplaced
	if meta.ArchiveFormat != registryv1.ArchiveFormatZip && meta.ArchiveFormat != registryv1.ArchiveFormatTarGz {
		return fmt.Errorf("sml addon %s requires zip/smod archive, got %q", meta.Coordinate, meta.ArchiveFormat)
	}
	extracted := ws.BlobExtractedDir(meta.HexDigest)
	modSrcDir, modRef := detectSMLModRoot(extracted, addon.Package)
	dstDir := targetDir + "/" + modRef
	opts := volume.MaterializeOptions{
		ForceReplace:   force,
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	}
	materialized, err := volume.MaterializeTreeInRoot(modSrcDir, ws.Root, dstDir, opts)
	if err != nil {
		return fmt.Errorf("materialize sml mod %s to %s: %w", meta.Coordinate, dstDir, err)
	}
	if len(materialized) == 0 {
		return fmt.Errorf("sml mod %s extracted zero files", meta.Coordinate)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, dstDir)
	return nil
}

func detectSMLModRoot(extractedDir, fallbackPkg string) (rootDir, modRef string) {
	if stem := findDirectUPluginStem(extractedDir); stem != "" {
		return extractedDir, stem
	}
	entries, err := os.ReadDir(extractedDir)
	if err == nil {
		for _, ent := range entries {
			if !ent.IsDir() || ent.Name() == "." || ent.Name() == ".." {
				continue
			}
			cand := filepath.Join(extractedDir, ent.Name())
			if stem := findDirectUPluginStem(cand); stem != "" {
				return cand, stem
			}
		}
	}
	name := strings.TrimSpace(filepath.Base(fallbackPkg))
	if name == "" || name == "." {
		name = "Mod"
	}
	return extractedDir, name
}

func findDirectUPluginStem(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".uplugin") {
			continue
		}
		stem := strings.TrimSuffix(ent.Name(), filepath.Ext(ent.Name()))
		if strings.TrimSpace(stem) != "" {
			return strings.TrimSpace(stem)
		}
	}
	return ""
}
