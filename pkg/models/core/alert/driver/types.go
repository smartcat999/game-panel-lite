package driver

import (
	"context"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// MaxRenderedItems caps the number of alert entries rendered directly inside one batch message.
const MaxRenderedItems = 8

// CSTZone is the Asia/Shanghai (+08:00) fixed timezone used in operator-facing notification timestamps.
var CSTZone = time.FixedZone("CST", 8*3600)

// NotificationItem is one normalized firing, resolved, or test alert event routed to channels.
type NotificationItem struct {
	AlertKey    string    `json:"alertKey"`
	Scope       string    `json:"scope"`
	Tenant      string    `json:"tenant,omitempty"`
	TargetKind  string    `json:"targetKind"`
	TargetName  string    `json:"targetName"`
	RuleID      string    `json:"ruleID,omitempty"`
	RuleTitle   string    `json:"ruleTitle"`
	Severity    string    `json:"severity"`
	State       string    `json:"state"`
	MetricValue string    `json:"metricValue,omitempty"`
	Summary     string    `json:"summary"`
	OccurredAt  time.Time `json:"occurredAt"`
}

// DeliveryRequest carries normalized alert batch content and webhook metadata for a ChannelDriver.
type DeliveryRequest struct {
	Channel      corev1.AlertChannel
	RawURL       string
	DeliveryID   string
	Title        string
	Message      string
	PlainText    string
	Items        []NotificationItem
	Now          time.Time
	TimestampSec string
}

// ChannelDriver delivers normalized alert batches to a specific notification channel backend.
type ChannelDriver interface {
	Type() string
	Priority() int
	MatchURL(rawURL string) bool
	Send(ctx context.Context, req DeliveryRequest) error
}
