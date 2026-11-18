package wecom

import (
	"context"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func init() {
	driver.RegisterChannelDriver(&Driver{})
}

// Driver delivers Enterprise WeChat (WeCom) group robot Markdown webhooks.
type Driver struct{}

// Type returns the canonical WeCom channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeWeCom }

// Priority places WeCom URL auto-detection before the generic HTTP webhook driver.
func (*Driver) Priority() int { return 30 }

// MatchURL reports whether rawURL points to a WeCom robot webhook endpoint.
func (*Driver) MatchURL(rawURL string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(rawURL)), "qyapi.weixin.qq.com")
}

// Send formats and posts a Markdown alert payload to a WeCom webhook URL.
func (*Driver) Send(ctx context.Context, req driver.DeliveryRequest) error {
	payload := BuildMarkdownPayload(req.Channel.Name, req.Channel.ConsoleURL, req.Items, req.Now)
	return driver.PostJSONWebhook(ctx, req.RawURL, strings.TrimSpace(req.Channel.Secret), req.DeliveryID, req.TimestampSec, payload)
}

// BuildMarkdownPayload constructs a WeCom Markdown robot message payload.
func BuildMarkdownPayload(channelName, consoleURL string, items []driver.NotificationItem, now time.Time) map[string]any {
	_, headerTitle := driver.CardHeaderStyleAndTitle(channelName, items)
	md := driver.BuildStandardMarkdown(channelName, driver.SanitizeConsoleURL(consoleURL), headerTitle, items, now)
	return map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": md},
	}
}
