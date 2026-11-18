package tmod

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const stateKey = corev1.AddonInstallModeTModLatest + ":dirs"

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs the newest .tmod archive binary and maintains tModLoader enabled.json lists.
type Injector struct{}

// Mode returns the canonical tmod_latest addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeTModLatest
}

// Inject materializes the latest .tmod binary from the cached artifact into targetDir (default "Mods").
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	targetDir := driver.ResolveTargetDir(addon.TargetDir, "Mods")
	srcTMod, modBase, err := selectLatestTModFile(ws, addon, meta)
	if err != nil {
		return err
	}
	relFile := targetDir + "/" + modBase
	if err := volume.MaterializeFileInRoot(srcTMod, ws.Root, relFile, volume.MaterializeOptions{
		ForceReplace:   ws.ForceReplaceAddon(meta.Coordinate, meta.Digest),
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	}); err != nil {
		return fmt.Errorf("materialize tmod addon %s: %w", meta.Coordinate, err)
	}
	ws.Next.Files = append(ws.Next.Files, relFile)
	ws.Next.TModTargets = driver.AppendUniqueStr(ws.Next.TModTargets, targetDir)
	dirs := driver.GetOrInitState(ws, stateKey, func() map[string]bool { return make(map[string]bool) })
	dirs[targetDir] = true
	return nil
}

// Finalize writes enabled.json for every target directory populated during the current sync.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	dirs := driver.GetOrInitState(ws, stateKey, func() map[string]bool { return make(map[string]bool) })
	for _, dir := range ws.Next.TModTargets {
		if dirs[dir] {
			if err := writeTModEnabledJSON(ws.Root, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// Cleanup rewrites enabled.json for previously managed target directories after all tmod addons are removed.
func (*Injector) Cleanup(root *os.Root, prev driver.AddonsManifest) {
	if root == nil {
		return
	}
	dirs := prev.TModTargets
	if len(dirs) == 0 {
		dirs = []string{"Mods"}
	}
	for _, dir := range dirs {
		_ = writeTModEnabledJSON(root, dir)
	}
}

func selectLatestTModFile(
	ws *driver.WorkspaceContext,
	addon corev1.InstanceAddon,
	meta *driver.CachedArtifactMeta,
) (string, string, error) {
	if meta.ArchiveFormat != registryv1.ArchiveFormatTarGz && meta.ArchiveFormat != registryv1.ArchiveFormatZip {
		base := filepath.Base(meta.FileName)
		if !strings.HasSuffix(strings.ToLower(base), ".tmod") {
			base = addon.Package + ".tmod"
		}
		return ws.BlobFilePath(meta.HexDigest), base, nil
	}
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	cands := collectTModCandidates(srcDir)
	if len(cands) == 0 {
		return "", "", fmt.Errorf("tmodloader addon %s contains no .tmod file", meta.Coordinate)
	}
	sort.Slice(cands, func(i, j int) bool {
		dirI := filepath.ToSlash(filepath.Dir(cands[i].rel))
		dirJ := filepath.ToSlash(filepath.Dir(cands[j].rel))
		return corev1.CompareSemanticSegmentsDesc(dirI, dirJ) < 0
	})
	return cands[0].path, cands[0].base, nil
}

type tmodCandidate struct {
	path string
	rel  string
	base string
}

func collectTModCandidates(srcDir string) []tmodCandidate {
	var cands []tmodCandidate
	_ = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Type().IsRegular() && strings.HasSuffix(strings.ToLower(d.Name()), ".tmod") {
			rel, _ := filepath.Rel(srcDir, p)
			cands = append(cands, tmodCandidate{path: p, rel: filepath.ToSlash(rel), base: d.Name()})
		}
		return nil
	})
	return cands
}

func writeTModEnabledJSON(root *os.Root, targetDir string) error {
	safeDir, err := volume.EnsureSafeDirInRoot(root, targetDir)
	if err != nil {
		return err
	}
	entries, err := volume.ListRegularFilesInRoot(root, safeDir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, name := range entries {
		if strings.HasSuffix(strings.ToLower(name), ".tmod") {
			names = driver.AppendUniqueStr(names, strings.TrimSuffix(name, filepath.Ext(name)))
		}
	}
	sort.Strings(names)
	raw, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return err
	}
	return volume.WriteFileAtomicInRoot(root, safeDir+"/enabled.json", append(raw, '\n'), 0o644)
}
