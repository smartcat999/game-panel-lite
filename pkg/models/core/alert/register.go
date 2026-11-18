package alert

import (
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"

	// Self-register built-in notification channel delivery drivers.
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/dingtalk"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/feishu"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/shoutrrr"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/slack"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/webhook"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/wecom"
)

type (
	// ChannelDriver delivers normalized alert batches to a specific notification channel backend.
	ChannelDriver = driver.ChannelDriver
	// DeliveryRequest carries normalized alert batch content and webhook metadata for a ChannelDriver.
	DeliveryRequest = driver.DeliveryRequest
)

// RegisterChannelDriver registers a ChannelDriver keyed by its canonical Type().
func RegisterChannelDriver(drv ChannelDriver) {
	driver.RegisterChannelDriver(drv)
}

// LookupChannelDriver returns the registered ChannelDriver for the given channel type.
func LookupChannelDriver(typeName string) (ChannelDriver, bool) {
	return driver.LookupChannelDriver(typeName)
}

// DetectChannelType infers the provider type from rawURL using registered drivers.
func DetectChannelType(rawURL string) string {
	return driver.DetectChannelType(rawURL)
}
