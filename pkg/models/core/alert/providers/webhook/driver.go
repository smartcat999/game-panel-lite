package webhook

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

// Driver delivers generic JSON payloads to arbitrary HTTP(S) webhook endpoints.
type Driver struct{}

// Type returns the canonical generic HTTP webhook channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeWebhook }

// Priority makes generic HTTP webhook the fallback among http:// and https:// URLs.
func (*Driver) Priority() int { return 100 }

// MatchURL reports whether rawURL is an HTTP or HTTPS URL.
func (*Driver) MatchURL(rawURL string) bool {
	return driver.IsHTTPURL(rawURL)
}

// Send posts a structured JSON alert envelope to req.RawURL.
func (*Driver) Send(ctx context.Context, req driver.DeliveryRequest) error {
	payload := map[string]any{
		"msg_type":   "text",
		"msgtype":    "text",
		"text":       req.PlainText,
		"content":    map[string]string{"text": req.PlainText},
		"title":      driver.CleanSingleBracketTitle(req.Title),
		"message":    req.Message,
		"items":      req.Items,
		"deliveryId": req.DeliveryID,
		"timestamp":  req.Now.Format(time.RFC3339),
	}
	return driver.PostJSONWebhook(ctx, req.RawURL, strings.TrimSpace(req.Channel.Secret), req.DeliveryID, req.TimestampSec, payload)
}
