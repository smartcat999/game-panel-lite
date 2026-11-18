package slack

import (
	"context"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func init() {
	driver.RegisterChannelDriver(&Driver{})
}

// Driver delivers alert summaries to Slack Incoming Webhooks and compatible Discord webhooks.
type Driver struct{}

// Type returns the canonical Slack channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeSlack }

// Priority places Slack/Discord URL auto-detection before the generic HTTP webhook driver.
func (*Driver) Priority() int { return 40 }

// MatchURL reports whether rawURL points to a Slack or Discord webhook endpoint.
func (*Driver) MatchURL(rawURL string) bool {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.Contains(u, "hooks.slack.com") || strings.Contains(u, "discord.com/api/webhooks")
}

// Send posts a plain-text alert body tailored for Discord ("content") or Slack ("text").
func (*Driver) Send(ctx context.Context, req driver.DeliveryRequest) error {
	payload := map[string]any{"text": req.PlainText}
	if strings.Contains(strings.ToLower(req.RawURL), "discord.com/api/webhooks") {
		payload = map[string]any{"content": req.PlainText}
	}
	return driver.PostJSONWebhook(ctx, req.RawURL, strings.TrimSpace(req.Channel.Secret), req.DeliveryID, req.TimestampSec, payload)
}
