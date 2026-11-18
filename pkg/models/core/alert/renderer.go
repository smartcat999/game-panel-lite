package alert

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func deliverBatchSync(ctx context.Context, ch corev1.AlertChannel, deliveryID, title, message string, items []notificationItem) error {
	u := strings.TrimSpace(ch.URL)
	if u == "" {
		return fmt.Errorf("empty webhook URL")
	}
	if deliveryID == "" {
		deliveryID = newDeliveryID()
	}
	if len(items) == 0 {
		items = fallbackItemsFromTitleMessage(title, message)
	}
	now := time.Now().UTC()
	drv, ok := driver.ResolveDriver(ch, u)
	if !ok {
		return fmt.Errorf("unsupported notification channel type %q", ch.Type)
	}
	return drv.Send(ctx, driver.DeliveryRequest{
		Channel:      ch,
		RawURL:       u,
		DeliveryID:   deliveryID,
		Title:        title,
		Message:      message,
		PlainText:    driver.BuildPlainText(title, message, items),
		Items:        items,
		Now:          now,
		TimestampSec: strconv.FormatInt(now.Unix(), 10),
	})
}

func fallbackItemsFromTitleMessage(title, message string) []notificationItem {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "GamePanel Lite 告警通知"
	}
	return []notificationItem{{
		RuleTitle:  cleanTitle,
		Severity:   corev1.AlertSeverityWarning,
		State:      corev1.AlertStateFiring,
		Summary:    strings.TrimSpace(message),
		OccurredAt: time.Now().UTC(),
	}}
}
