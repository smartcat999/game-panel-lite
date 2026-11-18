package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
)

func init() {
	driver.RegisterChannelDriver(&Driver{})
}

// Driver delivers Feishu/Lark interactive cards with optional custom-bot HMAC-SHA256 signing.
type Driver struct{}

// Type returns the canonical Feishu channel provider constant.
func (*Driver) Type() string { return corev1.ChannelTypeFeishu }

// Priority places Feishu URL auto-detection before the generic HTTP webhook driver.
func (*Driver) Priority() int { return 10 }

// MatchURL reports whether rawURL points to a Feishu or Lark bot webhook endpoint.
func (*Driver) MatchURL(rawURL string) bool {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.Contains(u, "open.feishu.cn") || strings.Contains(u, "open.larksuite.com")
}

// Send formats and posts a Feishu interactive alert card to req.RawURL.
func (*Driver) Send(ctx context.Context, req driver.DeliveryRequest) error {
	secret := strings.TrimSpace(req.Channel.Secret)
	payload := BuildCardPayload(req.Channel.Name, req.Channel.ConsoleURL, req.Items, req.Now, req.TimestampSec, secret)
	return driver.PostJSONWebhook(ctx, req.RawURL, secret, req.DeliveryID, req.TimestampSec, payload)
}

// BuildCardPayload constructs a Feishu interactive message card payload without backticks or double brackets.
func BuildCardPayload(
	channelName, consoleURL string,
	items []driver.NotificationItem,
	now time.Time,
	tsSec, secret string,
) map[string]any {
	template, headerTitle := driver.CardHeaderStyleAndTitle(channelName, items)
	elements := buildCardElements(channelName, driver.SanitizeConsoleURL(consoleURL), items, now)
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"template": template,
				"title":    map[string]any{"tag": "plain_text", "content": headerTitle},
			},
			"elements": elements,
		},
	}
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(tsSec+"\n"+secret))
		payload["timestamp"] = tsSec
		payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	return payload
}

func buildCardElements(channelName, baseURL string, items []driver.NotificationItem, now time.Time) []map[string]any {
	elements := renderBodyElements(channelName, baseURL, items)
	if actions := buildActionButtons(baseURL, items); len(actions) > 0 {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}
	return append(elements,
		map[string]any{"tag": "hr"},
		map[string]any{
			"tag": "note",
			"elements": []map[string]any{{
				"tag":     "plain_text",
				"content": "GamePanel Lite 监控中心 · " + now.In(driver.CSTZone).Format("2006-01-02 15:04:05 CST"),
			}},
		},
	)
}

func renderBodyElements(channelName, baseURL string, items []driver.NotificationItem) []map[string]any {
	if len(items) == 1 {
		return []map[string]any{{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": driver.FormatSingleItemMarkdown(channelName, baseURL, items[0])},
		}}
	}
	var elements []map[string]any
	limit := min(len(items), driver.MaxRenderedItems)
	for i := range limit {
		if i > 0 {
			elements = append(elements, map[string]any{"tag": "hr"})
		}
		elements = append(elements, map[string]any{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": driver.FormatCompactItemMarkdown(baseURL, items[i])},
		})
	}
	if rem := len(items) - limit; rem > 0 {
		elements = append(elements, map[string]any{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": fmt.Sprintf("… 另有 **%d** 项告警事件，请前往控制台查看完整记录。", rem)},
		})
	}
	return elements
}

func buildActionButtons(baseURL string, items []driver.NotificationItem) []map[string]any {
	if baseURL == "" || len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return singleItemActionButtons(baseURL, items[0])
	}
	for _, it := range items {
		if it.Scope == corev1.AlertScopePlatform {
			return []map[string]any{
				linkButton("打开平台监控中心", baseURL+"/console/admin/alerts", "primary"),
				linkButton("计算节点列表", baseURL+"/console/admin/nodes", "default"),
			}
		}
		if it.Tenant != "" {
			tq := url.QueryEscape(it.Tenant)
			return []map[string]any{
				linkButton("查看告警记录", baseURL+"/console/events?tenant="+tq, "primary"),
				linkButton("工作区服务器列表", baseURL+"/console?tenant="+tq, "default"),
			}
		}
	}
	return []map[string]any{linkButton("打开监控控制台", baseURL+"/console", "default")}
}

func singleItemActionButtons(baseURL string, it driver.NotificationItem) []map[string]any {
	primaryStyle := "primary"
	if it.State == corev1.AlertStateResolved || it.State == corev1.AlertStateTest {
		primaryStyle = "default"
	}
	targetLink := driver.ItemDeepLinkURL(baseURL, it)
	if it.State == corev1.AlertStateTest {
		return []map[string]any{linkButton("查看通知渠道设置", targetLink, "default")}
	}
	if it.TargetKind == "instances" && it.TargetName != "" {
		btns := []map[string]any{linkButton("查看服务器控制台", targetLink, primaryStyle)}
		if it.Tenant != "" {
			btns = append(btns, linkButton("告警记录", baseURL+"/console/events?tenant="+url.QueryEscape(it.Tenant), "default"))
		}
		return btns
	}
	if it.TargetKind == "nodes" {
		return []map[string]any{
			linkButton("查看计算节点", targetLink, primaryStyle),
			linkButton("平台监控中心", baseURL+"/console/admin/alerts", "default"),
		}
	}
	return []map[string]any{linkButton("前往控制台查看", targetLink, primaryStyle)}
}

func linkButton(label, targetURL, btnType string) map[string]any {
	return map[string]any{
		"tag":  "button",
		"text": map[string]any{"tag": "plain_text", "content": label},
		"type": btnType,
		"url":  targetURL,
	}
}
