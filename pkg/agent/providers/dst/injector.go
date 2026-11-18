package dst

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const stateKey = corev1.AddonInstallModeDSTLocalMods + ":state"

type dstModEntry struct {
	id     string
	config map[string]string
	schema []registryv1.AddonConfigField
}

type dstState struct {
	entries []dstModEntry
}

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector installs Don't Starve Together workshop directories and generates Lua override configs.
type Injector struct{}

// Mode returns the canonical dst_local_mods addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeDSTLocalMods
}

// Inject materializes an extracted workshop directory under <targetDir>/workshop-<id> (default "mods").
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	st := driver.GetOrInitState(ws, stateKey, func() *dstState { return &dstState{} })
	recordDSTEntry(st, strings.TrimSpace(addon.Package), addon.Config, meta.ConfigSchema)

	targetDir := driver.ResolveTargetDir(addon.TargetDir, "mods")
	modSubDir := targetDir + "/workshop-" + filepath.Base(strings.TrimSpace(addon.Package))
	srcDir := ws.BlobExtractedDir(meta.HexDigest)
	materialized, err := volume.MaterializeTreeInRoot(srcDir, ws.Root, modSubDir, volume.MaterializeOptions{
		ForceReplace:   ws.ForceReplaceAddon(meta.Coordinate, meta.Digest),
		RequireReflink: ws.RequireReflink,
		DisableReflink: ws.DisableReflink,
	})
	if err != nil {
		return fmt.Errorf("materialize dst workshop mod %s: %w", meta.Coordinate, err)
	}
	if len(materialized) == 0 {
		return fmt.Errorf("dst workshop mod %s extracted zero files", meta.Coordinate)
	}
	ws.Next.Dirs = driver.AppendUniqueStr(ws.Next.Dirs, modSubDir)
	return nil
}

// Finalize writes Master/Caves modoverrides.lua and offline workshop setup markers.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	st := driver.GetOrInitState(ws, stateKey, func() *dstState { return &dstState{} })
	return writeDSTModConfigs(ws.Root, st.entries)
}

// Cleanup clears active mod entries from Master/Caves modoverrides.lua when DST addons are removed.
func (*Injector) Cleanup(root *os.Root, _ driver.AddonsManifest) {
	if root == nil {
		return
	}
	_ = writeDSTModConfigs(root, nil)
}

func recordDSTEntry(st *dstState, id string, cfg map[string]string, schema []registryv1.AddonConfigField) {
	if id == "" {
		return
	}
	entry := dstModEntry{
		id:     id,
		config: maps.Clone(cfg),
		schema: schema,
	}
	for i := range st.entries {
		if st.entries[i].id == id {
			st.entries[i] = entry
			return
		}
	}
	st.entries = append(st.entries, entry)
}

func writeDSTModConfigs(root *os.Root, entries []dstModEntry) error {
	if err := volume.WriteFileAtomicInRoot(
		root,
		"mods/dedicated_server_mods_setup.lua",
		[]byte("-- Managed offline by GamePanel Artifact Registry\n"),
		0o644,
	); err != nil {
		return err
	}
	for _, shard := range []string{"Master", "Caves"} {
		relPath := "DoNotStarveTogether/Cluster_1/" + shard + "/modoverrides.lua"
		existing, err := volume.ReadOptionalFileInRoot(root, relPath)
		if err != nil {
			return err
		}
		lua := renderDSTModOverrides(entries, string(existing))
		if err := volume.WriteFileAtomicInRoot(root, relPath, []byte(lua), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func renderDSTModOverrides(entries []dstModEntry, existing string) string {
	var b strings.Builder
	b.WriteString("return {\n")
	for _, entry := range entries {
		if entry.id == "" {
			continue
		}
		wkey := "workshop-" + entry.id
		if len(entry.config) > 0 {
			writeConfiguredDSTEntry(&b, wkey, entry)
			continue
		}
		if preserved := extractExistingDSTConfigBlock(existing, wkey); preserved != "" {
			fmt.Fprintf(&b, "  [%q] = {\n    enabled = true,\n    configuration_options = %s,\n  },\n", wkey, preserved)
			continue
		}
		fmt.Fprintf(&b, "  [%q] = { enabled = true },\n", wkey)
	}
	b.WriteString("}\n")
	return b.String()
}

func writeConfiguredDSTEntry(b *strings.Builder, wkey string, entry dstModEntry) {
	keys := make([]string, 0, len(entry.config))
	for k := range entry.config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(b, "  [%q] = {\n    enabled = true,\n    configuration_options = {\n", wkey)
	for _, k := range keys {
		fmt.Fprintf(b, "      [%s] = %s,\n", strconv.Quote(k), driver.FormatLuaScalar(k, entry.config[k], entry.schema))
	}
	b.WriteString("    },\n  },\n")
}
