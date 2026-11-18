package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func init() {
	driver.RegisterChannelDriver(&Driver{})
}

// Driver delivers DingTalk custom robot Markdown webhooks with optional query HMAC-SHA256 signing.
type Driver struct{}

// Type returns the canonical DingTalk channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeDingTalk }

// Priority places DingTalk URL auto-detection before the generic HTTP webhook driver.
func (*Driver) Priority() int { return 20 }

// MatchURL reports whether rawURL points to a DingTalk robot webhook endpoint.
func (*Driver) MatchURL(rawURL string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(rawURL)), "oapi.dingtalk.com")
}

// Send formats and posts a Markdown alert payload to a DingTalk webhook URL.
func (*Driver) Send(ctx context.Context, req driver.DeliveryRequest) error {
	secret := strings.TrimSpace(req.Channel.Secret)
	targetURL := SignURL(req.RawURL, secret, req.Now)
	payload := BuildMarkdownPayload(req.Channel.Name, req.Channel.ConsoleURL, req.Items, req.Now)
	return driver.PostJSONWebhook(ctx, targetURL, secret, req.DeliveryID, req.TimestampSec, payload)
}

// BuildMarkdownPayload constructs a DingTalk Markdown robot message payload.
func BuildMarkdownPayload(channelName, consoleURL string, items []driver.NotificationItem, now time.Time) map[string]any {
	_, headerTitle := driver.CardHeaderStyleAndTitle(channelName, items)
	md := driver.BuildStandardMarkdown(channelName, driver.SanitizeConsoleURL(consoleURL), headerTitle, items, now)
	return map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": headerTitle, "text": md},
	}
}

// SignURL appends DingTalk millisecond timestamp and HMAC-SHA256 sign query parameters when secret is set.
func SignURL(targetURL, secret string, now time.Time) string {
	if secret == "" || strings.Contains(targetURL, "&sign=") {
		return targetURL
	}
	tsMilli := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(tsMilli + "\n" + secret))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	sep := "&"
	if !strings.Contains(targetURL, "?") {
		sep = "?"
	}
	return targetURL + sep + "timestamp=" + tsMilli + "&sign=" + sign
}
