package app

import (
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"

	// Self-register built-in backup archive addon extractors per corev1.AddonInstallMode.
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/bepinex"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/cs2plugin"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/dst"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/flatdir"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/oxide"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/sml"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/tmod"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/app/providers/workshoptree"
)

type (
	// AddonExtractor discovers and normalizes backup addons for a single AddonInstallMode.
	AddonExtractor = driver.AddonExtractor
	// BaseExtractor provides default fallback implementations for AddonExtractors.
	BaseExtractor = driver.BaseExtractor
	// ExtractContext holds normalized archive entries and optional subtree scoping for addon extraction.
	ExtractContext = driver.ExtractContext
)

// RegisterAddonExtractor registers ext under its Mode() identifier.
func RegisterAddonExtractor(ext AddonExtractor) {
	driver.RegisterAddonExtractor(ext)
}

// LookupAddonExtractor returns the AddonExtractor registered for mode.
func LookupAddonExtractor(mode string) (AddonExtractor, bool) {
	return driver.LookupAddonExtractor(mode)
}

// RegisteredAddonModes returns the sorted list of registered addon extractor modes.
func RegisteredAddonModes() []string {
	return driver.RegisteredAddonModes()
}
