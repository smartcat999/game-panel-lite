package shoutrrr

import (
	"context"
	"strings"

	shoutrrrsdk "github.com/containrrr/shoutrrr"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func init() {
	driver.RegisterChannelDriver(&Driver{})
}

// Driver delivers non-HTTP notification URLs (such as Telegram, SMTP, Gotify) via Shoutrrr.
type Driver struct{}

// Type returns the canonical Shoutrrr channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeShoutrrr }

// Priority assigns lowest priority so non-HTTP URLs fall back to Shoutrrr after specialized matchers.
func (*Driver) Priority() int { return 200 }

// MatchURL reports whether rawURL is a non-empty non-HTTP(S) notification URL.
func (*Driver) MatchURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	return trimmed != "" && !driver.IsHTTPURL(trimmed)
}

// Send dispatches req.PlainText through Shoutrrr to req.RawURL.
func (*Driver) Send(_ context.Context, req driver.DeliveryRequest) error {
	return shoutrrrsdk.Send(req.RawURL, req.PlainText)
}
