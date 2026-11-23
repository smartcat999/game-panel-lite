package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	serverArtifactManifestFile = "server-artifact.json"
	addonsManifestFile         = "addons-manifest.json"
)

type serverArtifactManifest struct {
	Coordinate string   `json:"coordinate"`
	Digest     string   `json:"digest"`
	Files      []string `json:"files"`
}

type addonsManifest = driver.AddonsManifest

// BlobWorkspaceMaterializer implements runtime.WorkspaceMaterializer backed by <WorkDir>/.blob-cache.
type BlobWorkspaceMaterializer struct {
	WorkDir  string
	ClientFn func() (client.Interface, error)
}

func (m *BlobWorkspaceMaterializer) Prepare(ctx context.Context, in *corev1.Instance) (string, error) {
	coords := collectInstanceArtifactCoordinates(in)
	if len(coords) == 0 {
		return "", nil
	}
	if m.ClientFn == nil {
		return "", errors.New("registry client provider is not configured")
	}
	cli, err := m.ClientFn()
	if err != nil {
		return "", fmt.Errorf("connect registry API: %w", err)
	}
	firstSyncMsg := ""
	for _, coord := range coords {
		syncMsg, _, err := EnsureArtifactCached(ctx, m.WorkDir, cli, coord)
		if err != nil {
			return "", err
		}
		if syncMsg != "" && firstSyncMsg == "" {
			firstSyncMsg = syncMsg
		}
	}
	return firstSyncMsg, nil
}

func (m *BlobWorkspaceMaterializer) Materialize(ctx context.Context, hostDir string, in *corev1.Instance) error {
	if in == nil || strings.TrimSpace(hostDir) == "" {
		return nil
	}
	cleanHost := filepath.Clean(hostDir)
	if err := os.MkdirAll(cleanHost, 0o755); err != nil {
		return err
	}
	wsRoot, err := os.OpenRoot(cleanHost)
	if err != nil {
		return err
	}
	defer func() { _ = wsRoot.Close() }()

	stateRoot, err := m.openStateRoot(cleanHost, in.Metadata.UID)
	if err != nil {
		return err
	}
	defer func() { _ = stateRoot.Close() }()

	requireReflink := volume.RequireReflinkFromContext(ctx)
	storageInfo := EnsureBlobCacheStorageInfo(m.WorkDir)
	disableReflink := !requireReflink && !storageInfo.ProbedAt.IsZero() && !storageInfo.ReflinkSupported
	managed := managedInstanceFileSet(in.Spec.Files)
	prevAddonFiles := toKeepSet(readAddonsManifest(stateRoot).Files)
	serverManifest, serverReplaced, err := m.materializeServerArtifact(wsRoot, stateRoot, in.Spec.ServerArtifact, managed, prevAddonFiles, requireReflink, disableReflink)
	if err != nil {
		return err
	}
	removedFiles, err := m.materializeAddons(wsRoot, stateRoot, in.Spec.Addons, serverReplaced, requireReflink, disableReflink)
	if err != nil {
		return err
	}
	return m.restoreUnlinkedServerFiles(wsRoot, in.Spec.ServerArtifact, managed, removedFiles, serverManifest.Files, requireReflink, disableReflink)
}

func (m *BlobWorkspaceMaterializer) openStateRoot(cleanHost, uid string) (*os.Root, error) {
	stateID, err := volume.ResolveInstanceStateID(m.WorkDir, cleanHost, uid)
	if err != nil {
		return nil, err
	}
	stateBase := strings.TrimSpace(m.WorkDir)
	if stateBase == "" {
		stateBase = filepath.Join(filepath.Dir(cleanHost), ".gp-state")
	}
	return volume.OpenInstanceStateRoot(stateBase, stateID, true)
}

func managedInstanceFileSet(files []corev1.InstanceFile) map[string]bool {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string]bool, len(files))
	for _, f := range files {
		if clean := cleanWorkspaceRelPath(f.Path); clean != "" {
			out[clean] = true
		}
	}
	return out
}

func collectInstanceArtifactCoordinates(in *corev1.Instance) []string {
	if in == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(c string) {
		c = strings.TrimSpace(c)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if in.Spec.ServerArtifact != nil {
		add(in.Spec.ServerArtifact.Coordinate())
	}
	for i := range in.Spec.Addons {
		if in.Spec.Addons[i].Disabled {
			continue
		}
		add(in.Spec.Addons[i].Coordinate())
	}
	return out
}

func (m *BlobWorkspaceMaterializer) materializeServerArtifact(
	wsRoot, stateRoot *os.Root,
	ref *corev1.ArtifactRef,
	managed, prevAddonFiles map[string]bool,
	requireReflink, disableReflink bool,
) (serverArtifactManifest, bool, error) {
	prev := readServerManifest(stateRoot)
	if ref == nil || ref.Coordinate() == "" {
		if len(prev.Files) > 0 {
			unlinkWorkspaceFilesFiltered(wsRoot, prev.Files, managed)
			_ = stateRoot.Remove(serverArtifactManifestFile)
		}
		return serverArtifactManifest{}, len(prev.Files) > 0, nil
	}
	meta, err := readCachedArtifactMeta(m.WorkDir, ref.Coordinate())
	if err != nil {
		return prev, false, err
	}
	forceReplace := prev.Coordinate != meta.Coordinate || prev.Digest != meta.Digest || len(prev.Files) == 0
	if prev.Coordinate != "" && (prev.Coordinate != meta.Coordinate || prev.Digest != meta.Digest) {
		unlinkWorkspaceFilesFiltered(wsRoot, prev.Files, managed)
	}
	skip := managed
	if !forceReplace && len(prevAddonFiles) > 0 {
		skip = mergeBoolSets(managed, prevAddonFiles)
	}
	files, err := m.materializeServerPayload(wsRoot, ref, meta, skip, requireReflink, disableReflink, forceReplace)
	if err != nil {
		return prev, forceReplace, err
	}
	if !forceReplace {
		files = preserveSkippedAddonServerFiles(files, prev.Files, managed, prevAddonFiles)
	}
	manifest := serverArtifactManifest{Coordinate: meta.Coordinate, Digest: meta.Digest, Files: files}
	return manifest, forceReplace, volume.WriteJSONAtomicInRoot(stateRoot, serverArtifactManifestFile, manifest)
}

func (m *BlobWorkspaceMaterializer) restoreUnlinkedServerFiles(
	wsRoot *os.Root,
	ref *corev1.ArtifactRef,
	managed map[string]bool,
	removedFiles, serverFiles []string,
	requireReflink, disableReflink bool,
) error {
	if ref == nil || ref.Coordinate() == "" || len(removedFiles) == 0 || len(serverFiles) == 0 {
		return nil
	}
	restoreSet := intersectBoolSets(toKeepSet(removedFiles), toKeepSet(serverFiles))
	if len(restoreSet) == 0 {
		return nil
	}
	meta, err := readCachedArtifactMeta(m.WorkDir, ref.Coordinate())
	if err != nil {
		return err
	}
	skip := buildRestoreSkipSet(serverFiles, restoreSet, managed)
	_, err = m.materializeServerPayload(wsRoot, ref, meta, skip, requireReflink, disableReflink, true)
	return err
}

func mergeBoolSets(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for k, v := range a {
		if v {
			out[k] = true
		}
	}
	for k, v := range b {
		if v {
			out[k] = true
		}
	}
	return out
}

func intersectBoolSets(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

func buildRestoreSkipSet(serverFiles []string, restoreSet, managed map[string]bool) map[string]bool {
	skip := make(map[string]bool, len(serverFiles)+len(managed))
	for _, f := range serverFiles {
		if !restoreSet[f] || managed[f] {
			skip[f] = true
		}
	}
	for f, v := range managed {
		if v {
			skip[f] = true
		}
	}
	return skip
}

func preserveSkippedAddonServerFiles(files, prevFiles []string, managed, prevAddonFiles map[string]bool) []string {
	if len(prevAddonFiles) == 0 || len(prevFiles) == 0 {
		return files
	}
	seen := toKeepSet(files)
	for _, rel := range prevFiles {
		clean := cleanWorkspaceRelPath(rel)
		if clean != "" && prevAddonFiles[clean] && !managed[clean] && !seen[clean] {
			seen[clean] = true
			files = append(files, clean)
		}
	}
	return files
}

func (m *BlobWorkspaceMaterializer) materializeServerPayload(
	wsRoot *os.Root,
	ref *corev1.ArtifactRef,
	meta *CachedArtifactMeta,
	managed map[string]bool,
	requireReflink, disableReflink, forceReplace bool,
) ([]string, error) {
	if meta.ArchiveFormat == registryv1.ArchiveFormatTarGz || meta.ArchiveFormat == registryv1.ArchiveFormatZip {
		return m.materializeServerArchive(wsRoot, ref, meta, managed, requireReflink, disableReflink, forceReplace)
	}
	srcBlob := blobFilePath(m.WorkDir, meta.HexDigest)
	relTarget := cleanWorkspaceRelPath(firstNonEmpty(ref.TargetPath, meta.TargetPath, meta.FileName))
	if relTarget == "" {
		relTarget = firstNonEmpty(meta.FileName, ref.Package+".bin")
	}
	if managed[relTarget] {
		return nil, nil
	}
	opts := volume.MaterializeOptions{ForceReplace: forceReplace, RequireReflink: requireReflink, DisableReflink: disableReflink}
	if err := volume.MaterializeFileInRoot(srcBlob, wsRoot, relTarget, opts); err != nil {
		return nil, fmt.Errorf("materialize server binary %s to %s: %w", meta.Coordinate, relTarget, err)
	}
	files := []string{relTarget}
	if strings.HasSuffix(strings.ToLower(relTarget), ".jar") && relTarget != "server.jar" && !managed["server.jar"] {
		if err := volume.MaterializeFileInRoot(srcBlob, wsRoot, "server.jar", opts); err == nil {
			files = append(files, "server.jar")
		} else if errors.Is(err, volume.ErrReflinkUnsupported) {
			return nil, err
		}
	}
	return files, nil
}

func (m *BlobWorkspaceMaterializer) materializeServerArchive(
	wsRoot *os.Root,
	ref *corev1.ArtifactRef,
	meta *CachedArtifactMeta,
	managed map[string]bool,
	requireReflink, disableReflink, forceReplace bool,
) ([]string, error) {
	srcDir := blobExtractedDir(m.WorkDir, meta.HexDigest)
	relTarget := cleanWorkspaceRelPath(firstNonEmpty(ref.TargetPath, meta.TargetPath))
	opts := volume.MaterializeOptions{
		ForceReplace:   forceReplace,
		RequireReflink: requireReflink,
		DisableReflink: disableReflink,
		SkipRel: func(rel string) bool {
			fullRel := cleanWorkspaceRelPath(rel)
			if relTarget != "" && fullRel != "" {
				fullRel = relTarget + "/" + fullRel
			}
			return managed[fullRel]
		},
	}
	materialized, err := volume.MaterializeTreeInRoot(srcDir, wsRoot, relTarget, opts)
	if err != nil {
		return nil, fmt.Errorf("materialize server archive %s: %w", meta.Coordinate, err)
	}
	if relTarget != "" {
		for i, p := range materialized {
			materialized[i] = relTarget + "/" + p
		}
	}
	return materialized, nil
}

func cleanWorkspaceRelPath(raw string) string {
	return driver.CleanWorkspaceRelPath(raw)
}

func unlinkWorkspaceFiles(wsRoot *os.Root, relFiles []string) {
	unlinkWorkspaceFilesFiltered(wsRoot, relFiles, nil)
}

func unlinkWorkspaceFilesFiltered(wsRoot *os.Root, relFiles []string, skip map[string]bool) {
	if wsRoot == nil {
		return
	}
	for _, rel := range relFiles {
		clean := cleanWorkspaceRelPath(rel)
		if clean == "" || skip[clean] {
			continue
		}
		_ = wsRoot.Remove(filepath.FromSlash(clean))
	}
}

func unlinkWorkspaceDirs(wsRoot *os.Root, relDirs []string) {
	if wsRoot == nil {
		return
	}
	for _, rel := range relDirs {
		clean := cleanWorkspaceRelPath(rel)
		if clean == "" {
			continue
		}
		_ = wsRoot.RemoveAll(filepath.FromSlash(clean))
	}
}

func toKeepSet(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]bool, len(list))
	for _, raw := range list {
		if clean := cleanWorkspaceRelPath(raw); clean != "" {
			set[clean] = true
		}
	}
	return set
}

func unlinkMissingDirs(wsRoot *os.Root, prevDirs, nextDirs []string) {
	if wsRoot == nil || len(prevDirs) == 0 {
		return
	}
	keep := toKeepSet(nextDirs)
	var stale []string
	for _, rel := range prevDirs {
		if clean := cleanWorkspaceRelPath(rel); clean != "" && !keep[clean] {
			stale = append(stale, clean)
		}
	}
	unlinkWorkspaceDirs(wsRoot, stale)
}

func readServerManifest(stateRoot *os.Root) serverArtifactManifest {
	var m serverArtifactManifest
	if raw, err := volume.ReadOptionalFileInRoot(stateRoot, serverArtifactManifestFile); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

func readAddonsManifest(stateRoot *os.Root) addonsManifest {
	var m addonsManifest
	if raw, err := volume.ReadOptionalFileInRoot(stateRoot, addonsManifestFile); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

func (m *BlobWorkspaceMaterializer) materializeAddons(
	wsRoot, stateRoot *os.Root,
	addons []corev1.InstanceAddon,
	serverReplaced, requireReflink, disableReflink bool,
) ([]string, error) {
	prev := readAddonsManifest(stateRoot)
	if len(addons) == 0 {
		unlinkWorkspaceFiles(wsRoot, prev.Files)
		unlinkWorkspaceDirs(wsRoot, prev.Dirs)
		if len(prev.Modes) > 0 {
			cleanupRemovedAddonModes(wsRoot, prev, nil)
			_ = stateRoot.Remove(addonsManifestFile)
		}
		return prev.Files, nil
	}

	next := addonsManifest{Digests: make(map[string]string, len(addons))}
	ws := &driver.WorkspaceContext{
		Root:             wsRoot,
		RequireReflink:   requireReflink,
		DisableReflink:   disableReflink,
		ServerReplaced:   serverReplaced,
		Prev:             prev,
		Next:             &next,
		BlobFilePath:     func(hexDigest string) string { return blobFilePath(m.WorkDir, hexDigest) },
		BlobExtractedDir: func(hexDigest string) string { return blobExtractedDir(m.WorkDir, hexDigest) },
		State:            make(map[string]any),
	}
	for i := range addons {
		if err := m.injectConfiguredAddon(ws, addons[i]); err != nil {
			return nil, err
		}
	}
	keepFiles := toKeepSet(next.Files)
	removed := diffMissingFiles(prev.Files, keepFiles)
	unlinkWorkspaceFilesFiltered(wsRoot, prev.Files, keepFiles)
	unlinkMissingDirs(wsRoot, prev.Dirs, next.Dirs)
	if err := finalizeActiveAddonModes(ws); err != nil {
		return nil, err
	}
	cleanupRemovedAddonModes(wsRoot, prev, next.Modes)
	return removed, volume.WriteJSONAtomicInRoot(stateRoot, addonsManifestFile, next)
}

func diffMissingFiles(prevFiles []string, keep map[string]bool) []string {
	var removed []string
	for _, rel := range prevFiles {
		if clean := cleanWorkspaceRelPath(rel); clean != "" && !keep[clean] {
			removed = append(removed, clean)
		}
	}
	return removed
}

func (m *BlobWorkspaceMaterializer) injectConfiguredAddon(ws *driver.WorkspaceContext, addon corev1.InstanceAddon) error {
	if addon.Disabled {
		return nil
	}
	meta, err := readCachedArtifactMeta(m.WorkDir, addon.Coordinate())
	if err != nil {
		return err
	}
	ws.Next.Coordinates = append(ws.Next.Coordinates, meta.Coordinate)
	mode := firstNonEmpty(addon.InstallMode, corev1.AddonInstallModeFlatDir)
	inj, ok := driver.LookupAddonInjector(mode)
	if !ok {
		return fmt.Errorf("unsupported addon installMode %q", mode)
	}
	if !slices.Contains(ws.Next.Modes, mode) {
		ws.Next.Modes = append(ws.Next.Modes, mode)
	}
	if err := inj.Inject(ws, addon, meta); err != nil {
		return err
	}
	if ws.Next.Digests == nil {
		ws.Next.Digests = make(map[string]string)
	}
	if meta.Coordinate != "" && meta.Digest != "" {
		ws.Next.Digests[meta.Coordinate] = meta.Digest
	}
	return nil
}

func finalizeActiveAddonModes(ws *driver.WorkspaceContext) error {
	for _, mode := range ws.Next.Modes {
		inj, ok := driver.LookupAddonInjector(mode)
		if !ok {
			continue
		}
		if err := inj.Finalize(ws); err != nil {
			return err
		}
	}
	return nil
}

func cleanupRemovedAddonModes(wsRoot *os.Root, prev addonsManifest, activeModes []string) {
	for _, mode := range prev.Modes {
		if containsFold(activeModes, mode) {
			continue
		}
		if inj, ok := driver.LookupAddonInjector(mode); ok {
			inj.Cleanup(wsRoot, prev)
		}
	}
}

func containsFold(list []string, target string) bool {
	trimmed := strings.TrimSpace(target)
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), trimmed) {
			return true
		}
	}
	return false
}
