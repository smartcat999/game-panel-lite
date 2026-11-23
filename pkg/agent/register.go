package agent

import (
	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"

	// Self-register built-in addon injectors for each corev1.AddonInstallMode.
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/bepinex"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/cs2plugin"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/cs2workshop"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/dst"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/flatdir"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/oxide"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/sml"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/tmod"
	_ "github.com/smartcat999/game-panel-cloud/pkg/agent/providers/workshoptree"
)

type (
	// AddonInjector injects and finalizes addons of a single installMode inside an instance workspace.
	AddonInjector = driver.AddonInjector
	// AddonsManifest tracks materialized addon files, directories, and active modes in an instance workspace.
	AddonsManifest = driver.AddonsManifest
	// BaseInjector provides default no-op Finalize and Cleanup hooks for simple injectors.
	BaseInjector = driver.BaseInjector
	// WorkspaceContext carries mutable materialization state across addon injections.
	WorkspaceContext = driver.WorkspaceContext
)

// RegisterAddonInjector registers an AddonInjector implementation keyed by its Mode().
func RegisterAddonInjector(inj AddonInjector) {
	driver.RegisterAddonInjector(inj)
}

// LookupAddonInjector returns the registered AddonInjector for the given addon installMode.
func LookupAddonInjector(mode string) (AddonInjector, bool) {
	return driver.LookupAddonInjector(mode)
}

// RegisteredAddonModes returns the sorted list of registered addon installMode identifiers.
func RegisteredAddonModes() []string {
	return driver.RegisteredAddonModes()
}
