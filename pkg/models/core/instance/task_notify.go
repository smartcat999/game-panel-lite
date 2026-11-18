package instance

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
)

// HumanizeTaskError converts internal repository, S3, quota, and node-agent errors into clear Chinese explanations.
func HumanizeTaskError(err error) string {
	if err == nil {
		return "后台任务执行未完成，请前往实例页面检查详情。"
	}
	if errors.Is(err, corev1.AdmissionDenied) {
		return "工作区备份数量上限或备份存储配额已满，请清理旧备份快照或联系管理员扩容配额。"
	}
	if errors.Is(err, s3.ErrNotFound) {
		return "对象存储中未找到目标备份归档文件，可能已被清理或上传未完成。"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "任务执行响应超时，请检查节点网络、磁盘负载或稍后重试。"
	}
	raw := strings.TrimSpace(err.Error())
	return translateTaskErrorText(raw)
}

func translateTaskErrorText(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "backup storage quota is zero"):
		return "工作区当前备份存储配额为 0，无法创建备份快照，请扩容工作区备份配额。"
	case strings.Contains(lower, "tenant is not admitting backups"):
		return "当前工作区处于暂停或受限状态，暂不允许执行新的备份任务。"
	case strings.Contains(lower, "object storage") && strings.Contains(lower, "not configured"):
		return "平台对象存储（S3）尚未配置，无法执行归档备份或恢复。"
	case strings.Contains(lower, "another backup archive is already in progress"):
		return "当前实例已有正在执行的备份归档任务，本次触发已跳过。"
	case strings.Contains(lower, "restore is in progress"):
		return "当前实例正在执行存档恢复，暂无法同时执行备份任务。"
	case strings.Contains(lower, "exceeds maximum allowed backup size") || strings.Contains(lower, "exceeded max"):
		return "存档归档体积超出允许的单次备份容量上限，请清理冗余文件或扩容配额。"
	case strings.Contains(lower, "files require an active bound instance"):
		return "实例尚未完成节点绑定或当前未就绪，暂无法访问数据目录。"
	case strings.Contains(lower, "node-agent"):
		return "计算节点文件/终端服务响应异常，请检查实例与节点运行状态后重试。"
	default:
		return "任务执行异常：" + raw
	}
}

// FormatBackupDisplayName decodes hex-encoded custom labels (`--<hex>`) and formats archive timestamps cleanly.
func FormatBackupDisplayName(instanceName, rawPath, fallbackUID string) string {
	stem := strings.TrimSpace(rawPath)
	if idx := strings.LastIndex(stem, "/"); idx >= 0 {
		stem = stem[idx+1:]
	}
	if strings.HasPrefix(stem, "backup-") && len(stem) > 72 && stem[71] == '-' {
		stem = stem[72:]
	}
	stem = stripArchiveExtension(stem)
	if instanceName != "" && strings.HasPrefix(stem, instanceName+"-") {
		stem = strings.TrimPrefix(stem, instanceName+"-")
	}
	if idx := strings.LastIndex(stem, "--"); idx >= 0 && idx+2 < len(stem) {
		if decoded, err := hex.DecodeString(stem[idx+2:]); err == nil && len(decoded) > 0 && utf8.Valid(decoded) {
			if s := strings.TrimSpace(string(decoded)); s != "" {
				return s
			}
		}
	}
	if formatted, ok := formatCompactArchiveTimestamp(stem); ok {
		return formatted
	}
	if stem != "" {
		return stem
	}
	return strings.TrimSpace(fallbackUID)
}

func formatCompactArchiveTimestamp(stem string) (string, bool) {
	prefix := "存档备份 "
	ts := stem
	if strings.HasPrefix(ts, "auto-") {
		prefix = "自动备份 "
		ts = strings.TrimPrefix(ts, "auto-")
	}
	if len(ts) == 15 && ts[8] == '-' {
		if t, err := time.Parse("20060102-150405", ts); err == nil {
			return prefix + t.Format("2006-01-02 15:04"), true
		}
	}
	return "", false
}

func stripArchiveExtension(name string) string {
	for _, ext := range []string{".tar.gz", ".tgz", ".zip"} {
		if len(name) >= len(ext) && strings.EqualFold(name[len(name)-len(ext):], ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}
