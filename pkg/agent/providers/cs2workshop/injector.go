package cs2workshop

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	stateKey       = corev1.AddonInstallModeCS2Workshop + ":state"
	cfgRelPath     = "game/csgo/cfg/gp_workshop_maps.cfg"
	fallbackMapRef = "workshop"
)

type workshopEntry struct {
	id    string
	title string
}

type cs2WorkshopState struct {
	entries []workshopEntry
}

func init() {
	driver.RegisterAddonInjector(&Injector{})
}

// Injector records subscribed CS2 Steam Workshop maps (+host_workshop_map) into game/csgo/cfg/gp_workshop_maps.cfg.
type Injector struct{}

// Mode returns the canonical cs2_workshop addon installMode identifier.
func (*Injector) Mode() string {
	return corev1.AddonInstallModeCS2Workshop
}

// Inject registers a CS2 Workshop map ID for host_workshop_map startup / RCON indexing.
func (*Injector) Inject(ws *driver.WorkspaceContext, addon corev1.InstanceAddon, meta *driver.CachedArtifactMeta) error {
	wid := strings.TrimSpace(addon.Package)
	if wid == "" {
		wid = fallbackMapRef
	}
	title := strings.TrimSpace(addon.Title)
	if title == "" {
		title = wid
	}
	st := driver.GetOrInitState(ws, stateKey, func() *cs2WorkshopState { return &cs2WorkshopState{} })
	st.add(wid, title)
	ws.Next.Files = driver.AppendUniqueStr(ws.Next.Files, cfgRelPath)
	if len(meta.MapFolders) > 0 {
		for _, mf := range meta.MapFolders {
			ws.Next.MapFolders = driver.AppendUniqueStr(ws.Next.MapFolders, mf)
		}
	} else {
		ws.Next.MapFolders = driver.AppendUniqueStr(ws.Next.MapFolders, wid)
	}
	return nil
}

// Finalize writes game/csgo/cfg/gp_workshop_maps.cfg atomically with subscribed Workshop map references.
func (*Injector) Finalize(ws *driver.WorkspaceContext) error {
	st := driver.GetOrInitState(ws, stateKey, func() *cs2WorkshopState { return &cs2WorkshopState{} })
	return volume.WriteFileAtomicInRoot(ws.Root, cfgRelPath, buildWorkshopCFG(st.entries), 0o644)
}

// Cleanup removes game/csgo/cfg/gp_workshop_maps.cfg when all cs2_workshop addons are removed.
func (*Injector) Cleanup(root *os.Root, _ driver.AddonsManifest) {
	if root != nil {
		_ = root.Remove(cfgRelPath)
	}
}

func (s *cs2WorkshopState) add(id, title string) {
	idx := slices.IndexFunc(s.entries, func(e workshopEntry) bool { return e.id == id })
	if idx >= 0 {
		s.entries[idx].title = title
		return
	}
	s.entries = append(s.entries, workshopEntry{id: id, title: title})
}

func buildWorkshopCFG(entries []workshopEntry) []byte {
	var b strings.Builder
	b.WriteString("// Managed by GamePanel Cloud - CS2 Steam Workshop Maps\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "// host_workshop_map %s (%s)\n", e.id, strings.ReplaceAll(e.title, "\n", " "))
	}
	return []byte(b.String())
}
