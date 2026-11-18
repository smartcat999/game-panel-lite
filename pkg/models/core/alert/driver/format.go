package driver

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// SanitizeConsoleURL normalizes an external non-loopback HTTP(S) console origin used in deep links.
func SanitizeConsoleURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// CardHeaderStyleAndTitle derives a card color template and concise title from a batch of items.
func CardHeaderStyleAndTitle(channelName string, items []NotificationItem) (template, headerTitle string) {
	if len(items) == 1 {
		return singleCardHeaderStyle(channelName, items[0])
	}
	firing, resolved, hasCritical := countBatchStates(items)
	scopeLabel := batchScopeLabel(items)
	if firing == 0 {
		return "green", fmt.Sprintf("%s · %d 项告警已恢复", scopeLabel, resolved)
	}
	tpl := "orange"
	if hasCritical {
		tpl = "red"
	}
	if resolved > 0 {
		return tpl, fmt.Sprintf("%s · %d 项触发 / %d 项恢复", scopeLabel, firing, resolved)
	}
	return tpl, fmt.Sprintf("%s · %d 项告警触发", scopeLabel, firing)
}

func singleCardHeaderStyle(channelName string, it NotificationItem) (string, string) {
	switch it.State {
	case corev1.AlertStateTest:
		return "blue", testCardHeaderTitle(channelName, it)
	case corev1.AlertStateResolved:
		return "green", "告警已恢复 · " + TargetShortDesc(it)
	default:
		if it.Severity == corev1.AlertSeverityCritical {
			return "red", "严重告警 · " + it.RuleTitle
		}
		return "orange", "监控预警 · " + it.RuleTitle
	}
}

func testCardHeaderTitle(channelName string, it NotificationItem) string {
	if name := strings.TrimSpace(channelName); name != "" {
		return "通道连通性测试 · " + name
	}
	if it.Scope == corev1.AlertScopePlatform {
		return "通道连通性测试 · 平台全局"
	}
	if it.Tenant != "" {
		return "通道连通性测试 · 工作区 " + it.Tenant
	}
	return "通道连通性测试"
}

// BuildStandardMarkdown renders Markdown alert summaries suitable for DingTalk and WeCom webhooks.
func BuildStandardMarkdown(channelName, baseURL, headerTitle string, items []NotificationItem, now time.Time) string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(headerTitle)
	b.WriteString("\n\n")
	if len(items) == 1 {
		b.WriteString(FormatSingleItemMarkdown(channelName, baseURL, items[0]))
		b.WriteString("\n")
	} else {
		limit := min(len(items), MaxRenderedItems)
		for i := range limit {
			b.WriteString("- ")
			b.WriteString(strings.ReplaceAll(FormatCompactItemMarkdown(baseURL, items[i]), "\n", " — "))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n> GamePanel Lite · ")
	b.WriteString(now.In(CSTZone).Format("2006-01-02 15:04:05 CST"))
	return b.String()
}

// FormatSingleItemMarkdown formats a single alert or test probe using safe bold/link Markdown.
func FormatSingleItemMarkdown(channelName, baseURL string, it NotificationItem) string {
	var lines []string
	if it.State == corev1.AlertStateTest {
		lines = append(lines, "**当前状态**：连通性正常")
		if chDesc := testChannelTargetDesc(channelName, baseURL, it); chDesc != "" {
			lines = append(lines, "**测试渠道**："+chDesc)
		}
		if it.Summary != "" {
			lines = append(lines, "**详情说明**："+it.Summary)
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("**当前状态**：%s", stateSeverityLabel(it.State, it.Severity)))
	if target := TargetFullDesc(baseURL, it); target != "" {
		lines = append(lines, fmt.Sprintf("**监控对象**：%s", target))
	}
	if it.MetricValue != "" {
		lines = append(lines, fmt.Sprintf("**实时指标**：**%s**", it.MetricValue))
	}
	if it.Summary != "" {
		lines = append(lines, fmt.Sprintf("**详情说明**：%s", it.Summary))
	}
	return strings.Join(lines, "\n")
}

// FormatCompactItemMarkdown formats one concise batch entry with an optional console deep link.
func FormatCompactItemMarkdown(baseURL string, it NotificationItem) string {
	targetText := TargetShortDesc(it)
	if link := ItemDeepLinkURL(baseURL, it); link != "" {
		targetText = fmt.Sprintf("[%s](%s)", targetText, link)
	}
	head := fmt.Sprintf("**[%s]** %s · %s", StateShortLabel(it.State), targetText, it.RuleTitle)
	if it.MetricValue != "" {
		return fmt.Sprintf("%s\n指标：**%s** ｜ %s", head, it.MetricValue, it.Summary)
	}
	return fmt.Sprintf("%s\n%s", head, it.Summary)
}

// BuildPlainText builds a concise plain-text alert summary for Slack, Discord, webhooks, and Shoutrrr.
func BuildPlainText(title, message string, items []NotificationItem) string {
	if len(items) > 1 {
		_, header := CardHeaderStyleAndTitle("", items)
		lines := make([]string, 0, min(len(items), MaxRenderedItems)+1)
		lines = append(lines, header)
		for i := range min(len(items), MaxRenderedItems) {
			it := items[i]
			lines = append(lines, fmt.Sprintf("- [%s] %s · %s (%s)", StateShortLabel(it.State), TargetShortDesc(it), it.RuleTitle, it.MetricValue))
		}
		return strings.Join(lines, "\n")
	}
	cleanTitle := CleanSingleBracketTitle(title)
	if len(items) == 1 && cleanTitle == "" {
		cleanTitle = FormatSingleItemTitle(items[0])
	}
	cleanMsg := strings.TrimSpace(message)
	if cleanMsg == "" && len(items) == 1 {
		cleanMsg = FormatSingleItemMessage(items[0])
	}
	if cleanMsg == "" {
		return cleanTitle
	}
	return cleanTitle + "\n" + cleanMsg
}

// CleanSingleBracketTitle collapses accidental double brackets (e.g. "[[...]]") into single brackets.
func CleanSingleBracketTitle(raw string) string {
	s := strings.TrimSpace(raw)
	for strings.HasPrefix(s, "[[") && strings.Contains(s, "]]") {
		s = "[" + strings.TrimPrefix(s, "[[")
		s = strings.Replace(s, "]]", "]", 1)
	}
	return s
}

// FormatSingleItemTitle formats a compact subject line for one notification item.
func FormatSingleItemTitle(it NotificationItem) string {
	if it.State == corev1.AlertStateTest {
		return it.RuleTitle
	}
	tag := StateShortLabel(it.State)
	if it.TargetKind == "nodes" {
		return fmt.Sprintf("[%s] 计算节点 %s · %s", tag, it.TargetName, it.RuleTitle)
	}
	if it.Tenant != "" {
		return fmt.Sprintf("[%s] 工作区 %s · 服务器 %s · %s", tag, it.Tenant, it.TargetName, it.RuleTitle)
	}
	return fmt.Sprintf("[%s] %s · %s", tag, it.TargetName, it.RuleTitle)
}

// FormatSingleItemMessage formats the concise detail text for one notification item.
func FormatSingleItemMessage(it NotificationItem) string {
	if it.MetricValue == "" || it.State == corev1.AlertStateTest {
		return it.Summary
	}
	return fmt.Sprintf("当前指标 %s — %s", it.MetricValue, it.Summary)
}

// StateShortLabel maps an alert state constant to a localized Chinese status tag.
func StateShortLabel(state string) string {
	switch state {
	case corev1.AlertStateResolved:
		return "已恢复"
	case corev1.AlertStateTest:
		return "通道测试"
	default:
		return "告警触发"
	}
}

// TargetShortDesc returns a short human-readable description of an item's target resource.
func TargetShortDesc(it NotificationItem) string {
	if it.TargetKind == "nodes" && it.TargetName != "" {
		return "节点 " + it.TargetName
	}
	if it.TargetKind == "instances" && it.TargetName != "" {
		return "服务器 " + it.TargetName
	}
	if it.TargetName != "" {
		return it.TargetName
	}
	return it.RuleTitle
}

// TargetFullDesc returns a Markdown-enriched target path including a deep link when baseURL is present.
func TargetFullDesc(baseURL string, it NotificationItem) string {
	link := ItemDeepLinkURL(baseURL, it)
	if it.TargetKind == "nodes" {
		if link != "" {
			return fmt.Sprintf("平台集群 / [计算节点 %s](%s)", it.TargetName, link)
		}
		return fmt.Sprintf("平台集群 / 计算节点 **%s**", it.TargetName)
	}
	if it.TargetKind == "instances" && it.Tenant != "" {
		if link != "" {
			return fmt.Sprintf("工作区 **%s** / [服务器 %s](%s)", it.Tenant, it.TargetName, link)
		}
		return fmt.Sprintf("工作区 **%s** / 服务器 **%s**", it.Tenant, it.TargetName)
	}
	if it.Tenant != "" {
		return fmt.Sprintf("工作区 **%s**", it.Tenant)
	}
	return it.TargetName
}

// ItemDeepLinkURL resolves the operator console URL corresponding to one alert item.
func ItemDeepLinkURL(baseURL string, it NotificationItem) string {
	if baseURL == "" {
		return ""
	}
	if it.State == corev1.AlertStateTest {
		return scopeFallbackURL(baseURL, it)
	}
	if it.TargetKind == "nodes" {
		return baseURL + "/console/admin/nodes"
	}
	if it.TargetKind == "instances" && it.TargetName != "" {
		if it.Tenant != "" {
			return fmt.Sprintf("%s/console/servers/%s?tenant=%s&tab=console",
				baseURL, url.PathEscape(it.TargetName), url.QueryEscape(it.Tenant))
		}
		return baseURL + "/console/servers/" + url.PathEscape(it.TargetName) + "?tab=console"
	}
	return scopeFallbackURL(baseURL, it)
}

func scopeFallbackURL(baseURL string, it NotificationItem) string {
	if it.Scope == corev1.AlertScopePlatform {
		return baseURL + "/console/admin/alerts"
	}
	if it.Tenant != "" {
		return baseURL + "/console/events?tenant=" + url.QueryEscape(it.Tenant)
	}
	return baseURL + "/console"
}

func testChannelTargetDesc(channelName, baseURL string, it NotificationItem) string {
	scope := TargetFullDesc(baseURL, it)
	name := strings.TrimSpace(channelName)
	if name != "" && scope != "" {
		return fmt.Sprintf("**%s**（%s）", name, scope)
	}
	if name != "" {
		return fmt.Sprintf("**%s**", name)
	}
	return scope
}

func countBatchStates(items []NotificationItem) (firing, resolved int, hasCritical bool) {
	for _, it := range items {
		if it.State == corev1.AlertStateResolved {
			resolved++
			continue
		}
		firing++
		if it.Severity == corev1.AlertSeverityCritical {
			hasCritical = true
		}
	}
	return firing, resolved, hasCritical
}

func batchScopeLabel(items []NotificationItem) string {
	for _, it := range items {
		if it.Scope == corev1.AlertScopePlatform {
			return "平台集群"
		}
		if it.Tenant != "" {
			return fmt.Sprintf("工作区 %s", it.Tenant)
		}
	}
	return "GamePanel"
}

func stateSeverityLabel(state, severity string) string {
	switch state {
	case corev1.AlertStateResolved:
		return "已恢复 (Resolved)"
	case corev1.AlertStateTest:
		return "通道连通性测试"
	default:
		if severity == corev1.AlertSeverityCritical {
			return "严重告警 (Critical · Firing)"
		}
		return "监控预警 (Warning · Firing)"
	}
}
