import type { AlertItem, AuditEventItem, PlatformLogEntry } from "./types";

export interface AlertRuleItem {
  id: string;
  title: string;
  description: string;
  metric:
    | "memory_usage_ratio"
    | "storage_usage_ratio"
    | "cpu_usage_ratio"
    | "online_players_ratio"
    | "server_tps_low"
    | "container_oom_events"
    | "instance_node_phase"
    | "node_heartbeat_age"
    | "node_memory_usage_ratio"
    | "node_disk_usage_ratio"
    | "alertmanager_v2_alerts";
  threshold: number;
  forSeconds?: number;
  severity: "warning" | "critical";
  scope: string; // "all" | "platform" | tenantName
  enabled: boolean;
}

export function defaultForSecondsForMetric(metric: AlertRuleItem["metric"]): number {
  switch (metric) {
    case "cpu_usage_ratio":
      return 180;
    case "memory_usage_ratio":
    case "storage_usage_ratio":
    case "online_players_ratio":
    case "server_tps_low":
    case "node_memory_usage_ratio":
    case "node_disk_usage_ratio":
      return 120;
    default:
      return 0;
  }
}

export const ALERT_METRIC_OPTIONS: Array<{
  value: AlertRuleItem["metric"];
  label: string;
  unit: "%" | "s" | "TPS" | "event";
  defaultThreshold: number;
  defaultForSeconds: number;
  defaultScope: "all" | "platform";
  defaultTitle: string;
  defaultDesc: string;
}> = [
  {
    value: "memory_usage_ratio",
    label: "服务器内存使用率 (%)",
    unit: "%",
    defaultThreshold: 85,
    defaultForSeconds: 120,
    defaultScope: "all",
    defaultTitle: "服务器内存占用过高预警",
    defaultDesc: "当运行中游戏服务器内存使用率持续达到配额上限设定比例时触发告警。",
  },
  {
    value: "storage_usage_ratio",
    label: "服务器存储使用率 (%)",
    unit: "%",
    defaultThreshold: 85,
    defaultForSeconds: 120,
    defaultScope: "all",
    defaultTitle: "服务器存储占用过高告警",
    defaultDesc: "当服务器工作目录磁盘占用持续达到存储配额设定比例时提醒清理或扩容。",
  },
  {
    value: "cpu_usage_ratio",
    label: "服务器 CPU 使用率 (%)",
    unit: "%",
    defaultThreshold: 90,
    defaultForSeconds: 180,
    defaultScope: "all",
    defaultTitle: "服务器 CPU 持续满载预警",
    defaultDesc: "当运行中游戏服务器 CPU 占用率持续达到配额上限设定比例时触发预警。",
  },
  {
    value: "online_players_ratio",
    label: "游戏在线玩家满载率 (%)",
    unit: "%",
    defaultThreshold: 90,
    defaultForSeconds: 120,
    defaultScope: "all",
    defaultTitle: "游戏在线玩家接近满服提醒",
    defaultDesc: "当运行中游戏服务器在线玩家人数持续达到最大人数上限设定比例时触发提醒，便于及时扩容或分流。",
  },
  {
    value: "server_tps_low",
    label: "游戏服务端 TPS 掉帧预警 (TPS <=)",
    unit: "TPS",
    defaultThreshold: 15,
    defaultForSeconds: 120,
    defaultScope: "all",
    defaultTitle: "游戏服务端 TPS 低帧卡顿预警",
    defaultDesc: "当运行中游戏服务器实时服务端 TPS 持续跌至或低于设定阈值时触发卡顿掉帧预警。",
  },
  {
    value: "container_oom_events",
    label: "容器 OOM 异常退出",
    unit: "event",
    defaultThreshold: 1,
    defaultForSeconds: 0,
    defaultScope: "all",
    defaultTitle: "容器 OOMKilled 异常退出检测",
    defaultDesc: "实时监听容器 ExitCode 137 (OOMKilled) 或进程异常崩溃退出并记录恢复事件。",
  },
  {
    value: "instance_node_phase",
    label: "服务器所在节点失联",
    unit: "event",
    defaultThreshold: 1,
    defaultForSeconds: 0,
    defaultScope: "all",
    defaultTitle: "服务器所在节点失联检测",
    defaultDesc: "当服务器绑定的计算节点进入失联状态时，将该节点上的运行服务器标记为状态未知。",
  },
  {
    value: "node_heartbeat_age",
    label: "计算节点心跳超时 (秒)",
    unit: "s",
    defaultThreshold: 40,
    defaultForSeconds: 0,
    defaultScope: "platform",
    defaultTitle: "计算节点心跳失联巡检",
    defaultDesc: "周期性检测宿主机节点心跳，超过设定秒数未上报自动标记为失联。",
  },
  {
    value: "node_memory_usage_ratio",
    label: "宿主机物理内存使用率 (%)",
    unit: "%",
    defaultThreshold: 85,
    defaultForSeconds: 120,
    defaultScope: "platform",
    defaultTitle: "计算节点内存占用过高告警",
    defaultDesc: "当计算节点宿主机物理内存使用率持续达到设定阈值时触发平台级预警。",
  },
  {
    value: "node_disk_usage_ratio",
    label: "宿主机磁盘空间使用率 (%)",
    unit: "%",
    defaultThreshold: 85,
    defaultForSeconds: 120,
    defaultScope: "platform",
    defaultTitle: "计算节点磁盘占用过高告警",
    defaultDesc: "当计算节点宿主机工作分区磁盘占用持续达到设定阈值时触发平台级预警。",
  },
  {
    value: "alertmanager_v2_alerts",
    label: "Alertmanager 外部告警同步",
    unit: "event",
    defaultThreshold: 1,
    defaultForSeconds: 0,
    defaultScope: "all",
    defaultTitle: "Prometheus Alertmanager 外部联动",
    defaultDesc: "自动聚合并同步外部 Alertmanager (/api/v2/alerts) 中处于 active 状态的告警。",
  },
];

export const DEFAULT_ALERT_RULES: AlertRuleItem[] = [
  {
    id: "rule-mem-85",
    title: "服务器内存占用过高预警",
    description: "当运行中游戏服务器内存使用率持续达到或超过配额上限的 85% 时触发预警。",
    metric: "memory_usage_ratio",
    threshold: 85,
    forSeconds: 120,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-disk-85",
    title: "服务器存储占用过高告警",
    description: "当服务器工作目录磁盘占用持续达到或超过存储配额的 85% 时提醒清理或扩容。",
    metric: "storage_usage_ratio",
    threshold: 85,
    forSeconds: 120,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-cpu-90",
    title: "服务器 CPU 持续满载预警",
    description: "当运行中游戏服务器 CPU 占用率持续达到或超过配额上限的 90% 时触发预警。",
    metric: "cpu_usage_ratio",
    threshold: 90,
    forSeconds: 180,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-tps-low",
    title: "游戏服务端 TPS 低帧卡顿预警",
    description: "当运行中游戏服务器实时服务端 TPS 持续跌至或低于 15 TPS 时触发卡顿掉帧预警。",
    metric: "server_tps_low",
    threshold: 15,
    forSeconds: 120,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-players-90",
    title: "游戏在线玩家接近满服提醒",
    description: "当运行中游戏服务器在线玩家人数持续达到或超过最大人数上限的 90% 时触发提醒。",
    metric: "online_players_ratio",
    threshold: 90,
    forSeconds: 120,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-oom-killed",
    title: "容器 OOMKilled 异常退出检测",
    description: "实时监听容器 ExitCode 137 (OOMKilled) 或进程异常崩溃退出并记录恢复事件。",
    metric: "container_oom_events",
    threshold: 1,
    forSeconds: 0,
    severity: "critical",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-node-heartbeat",
    title: "计算节点心跳失联巡检",
    description: "周期性检测宿主机节点心跳，超过设定秒数未上报自动标记为失联。",
    metric: "node_heartbeat_age",
    threshold: 40,
    forSeconds: 0,
    severity: "critical",
    scope: "platform",
    enabled: true,
  },
  {
    id: "rule-node-mem-85",
    title: "计算节点内存占用过高告警",
    description: "当计算节点宿主机物理内存使用率持续达到或超过 85% 时触发平台级预警。",
    metric: "node_memory_usage_ratio",
    threshold: 85,
    forSeconds: 120,
    severity: "warning",
    scope: "platform",
    enabled: true,
  },
  {
    id: "rule-node-disk-85",
    title: "计算节点磁盘占用过高告警",
    description: "当计算节点宿主机工作分区磁盘占用持续达到或超过 85% 时触发平台级预警。",
    metric: "node_disk_usage_ratio",
    threshold: 85,
    forSeconds: 120,
    severity: "warning",
    scope: "platform",
    enabled: true,
  },
  {
    id: "rule-inst-unknown",
    title: "服务器所在节点失联检测",
    description: "当服务器绑定的计算节点进入失联状态时，将该节点上的运行服务器标记为状态未知。",
    metric: "instance_node_phase",
    threshold: 1,
    forSeconds: 0,
    severity: "critical",
    scope: "all",
    enabled: true,
  },
  {
    id: "rule-alertmanager-sync",
    title: "Prometheus Alertmanager 外部联动",
    description: "自动聚合并同步外部 Alertmanager (/api/v2/alerts) 中处于 active 状态的告警。",
    metric: "alertmanager_v2_alerts",
    threshold: 1,
    forSeconds: 0,
    severity: "warning",
    scope: "all",
    enabled: true,
  },
];

function formatDurationShort(sec?: number): string {
  const s = Number(sec) || 0;
  if (s <= 0) return "";
  if (s % 60 === 0) return `${s / 60}m`;
  return `${s}s`;
}

export function formatRuleConditionExpr(rule: AlertRuleItem): string {
  const th = rule.threshold || 85;
  const dur = formatDurationShort(
    rule.forSeconds ?? defaultForSecondsForMetric(rule.metric),
  );
  const suffix = dur ? ` · ${dur}` : "";
  switch (rule.metric) {
    case "memory_usage_ratio":
      return `memory_usage >= ${th}%${suffix}`;
    case "storage_usage_ratio":
      return `disk_usage >= ${th}%${suffix}`;
    case "cpu_usage_ratio":
      return `cpu_usage >= ${th}%${suffix}`;
    case "online_players_ratio":
      return `online_players >= ${th}%${suffix}`;
    case "server_tps_low":
      return `server_tps <= ${rule.threshold || 15} TPS${suffix}`;
    case "container_oom_events":
      return "reason == OOMKilled";
    case "instance_node_phase":
      return "node.phase == NotReady";
    case "node_heartbeat_age":
      return `heartbeat_age > ${rule.threshold || 40}s`;
    case "node_memory_usage_ratio":
      return `node_memory >= ${th}%${suffix}`;
    case "node_disk_usage_ratio":
      return `node_disk >= ${th}%${suffix}`;
    case "alertmanager_v2_alerts":
      return "status.state == active";
    default:
      return `${rule.metric} >= ${th}${suffix}`;
  }
}

export function formatRuleScopeLabel(rule: AlertRuleItem): string {
  if (
    rule.metric === "node_heartbeat_age" ||
    rule.metric === "node_memory_usage_ratio" ||
    rule.metric === "node_disk_usage_ratio" ||
    rule.scope === "platform"
  ) {
    return "平台计算节点";
  }
  if (rule.metric === "alertmanager_v2_alerts") {
    return "全局与工作区";
  }
  if (!rule.scope || rule.scope === "all") {
    return "全部工作区实例";
  }
  return `工作区 ${rule.scope}`;
}

export function parsePlatformAlertRules(
  annotations?: Record<string, string>,
  specRules?: Array<Partial<AlertRuleItem>>,
): AlertRuleItem[] {
  const normalizeRules = (items: Array<Partial<AlertRuleItem>>): AlertRuleItem[] =>
    items.map((r, i) => {
      const metric = (r.metric || "memory_usage_ratio") as AlertRuleItem["metric"];
      const forSec = Number(r.forSeconds) > 0 ? Number(r.forSeconds) : defaultForSecondsForMetric(metric);
      return {
        id: r.id || `rule-${i + 1}`,
        title: r.title?.trim() || "自定义监控规则",
        description: r.description?.trim() || "",
        metric,
        threshold: Number(r.threshold) > 0 ? Number(r.threshold) : 85,
        forSeconds: forSec,
        severity: r.severity === "critical" ? "critical" : "warning",
        scope: r.scope || "all",
        enabled: r.enabled !== false,
      };
    });

  if (Array.isArray(specRules) && specRules.length > 0) {
    return normalizeRules(specRules);
  }
  if (annotations) {
    const raw = annotations["core.gamepanel.io/platform-alert-rules"]?.trim();
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as Partial<AlertRuleItem>[];
        if (Array.isArray(parsed) && parsed.length > 0) {
          return normalizeRules(parsed);
        }
      } catch {
        // Fall back to default rules
      }
    }
  }
  return DEFAULT_ALERT_RULES;
}

export function matchAlertsForRule(rule: AlertRuleItem, alerts: AlertItem[]): AlertItem[] {
  const idToken = `-${rule.id}-`;
  return alerts.filter((a) => {
    const name = a.metadata.name || "";
    if (name.includes(idToken)) return true;
    if (rule.metric === "alertmanager_v2_alerts" && name.startsWith("am-")) return true;
    const cond = (a.condition || "").trim();
    switch (rule.metric) {
      case "memory_usage_ratio":
        return cond.startsWith("memory_usage");
      case "storage_usage_ratio":
        return cond.startsWith("disk_usage");
      case "cpu_usage_ratio":
        return cond.startsWith("cpu_usage");
      case "online_players_ratio":
        return cond.startsWith("online_players");
      case "server_tps_low":
        return cond.startsWith("server_tps");
      case "container_oom_events":
        return cond === "reason == OOMKilled" || cond === "phase == Failed";
      case "instance_node_phase":
        return cond === "node.phase == NotReady";
      case "node_heartbeat_age":
        return cond.startsWith("heartbeat_age");
      case "node_memory_usage_ratio":
        return (
          cond.startsWith("node_memory") ||
          (a.resource === "nodes" && !cond && a.title.includes("内存"))
        );
      case "node_disk_usage_ratio":
        return (
          cond.startsWith("node_disk") ||
          (a.resource === "nodes" &&
            !cond &&
            (a.title.includes("磁盘") || a.title.includes("存储")))
        );
      default:
        return false;
    }
  });
}

export function inferMetricForAlert(alert: AlertItem): AlertRuleItem["metric"] {
  const name = alert.metadata.name || "";
  if (name.startsWith("am-")) return "alertmanager_v2_alerts";
  const cond = (alert.condition || "").trim();
  if (cond.startsWith("node_disk")) return "node_disk_usage_ratio";
  if (cond.startsWith("node_memory")) return "node_memory_usage_ratio";
  if (cond.startsWith("heartbeat_age")) return "node_heartbeat_age";
  if (cond.startsWith("memory_usage")) return "memory_usage_ratio";
  if (cond.startsWith("disk_usage")) return "storage_usage_ratio";
  if (cond.startsWith("cpu_usage")) return "cpu_usage_ratio";
  if (cond.startsWith("online_players")) return "online_players_ratio";
  if (cond.startsWith("server_tps")) return "server_tps_low";
  if (cond === "reason == OOMKilled" || cond === "phase == Failed") {
    return "container_oom_events";
  }
  if (cond === "node.phase == NotReady") return "instance_node_phase";

  const title = alert.title || "";
  if (alert.resource === "nodes") {
    if (title.includes("磁盘") || title.includes("存储")) return "node_disk_usage_ratio";
    if (title.includes("内存")) return "node_memory_usage_ratio";
    return "node_heartbeat_age";
  }
  if (title.includes("磁盘") || title.includes("存储")) return "storage_usage_ratio";
  if (title.includes("CPU")) return "cpu_usage_ratio";
  if (title.includes("TPS")) return "server_tps_low";
  if (title.includes("玩家")) return "online_players_ratio";
  if (title.includes("OOM")) return "container_oom_events";
  if (title.includes("失联")) return "instance_node_phase";
  return "memory_usage_ratio";
}

export function sanitizeObservabilityText(raw?: string): string {
  if (!raw) return "";
  return raw.replace(/\s*[—–]\s*/g, " · ").trim();
}

export interface AlertChannelItem {
  id: string;
  name: string;
  type: "feishu" | "dingtalk" | "wecom" | "slack" | "shoutrrr" | "webhook";
  url: string;
  secret?: string;
  consoleURL?: string;
  severity: "all" | "critical";
  enabled: boolean;
  state?: "healthy" | "retrying" | "failed";
  retryCount?: number;
  lastDeliveryAt?: string;
  lastError?: string;
  nextRetryAt?: string;
  pendingTitle?: string;
  pendingMessage?: string;
}

export function resolveBrowserConsoleOrigin(): string | undefined {
  if (typeof window === "undefined") return undefined;
  const { protocol, hostname, origin } = window.location;
  if ((protocol !== "http:" && protocol !== "https:") || !origin) return undefined;
  const lower = (hostname || "").toLowerCase();
  if (!lower || lower === "localhost" || lower === "127.0.0.1" || lower === "::1" || lower === "[::1]") {
    return undefined;
  }
  return origin.replace(/\/+$/, "");
}

export interface NotificationDeliveryRecord {
  id: string;
  deliveryId: string;
  scope: "tenant" | "platform";
  tenant?: string;
  channelId: string;
  channelName: string;
  channelType: string;
  status: "delivered" | "retrying" | "failed";
  title: string;
  message: string;
  attempts: number;
  error?: string;
  nextRetryAt?: string;
  timestamp: string;
}

export const CHANNEL_TYPE_OPTIONS: Array<{
  value: AlertChannelItem["type"];
  label: string;
  shortLabel: string;
  placeholder: string;
}> = [
  {
    value: "feishu",
    label: "飞书群机器人",
    shortLabel: "飞书机器人",
    placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
  },
  {
    value: "dingtalk",
    label: "钉钉群机器人",
    shortLabel: "钉钉机器人",
    placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...",
  },
  {
    value: "wecom",
    label: "企业微信机器人",
    shortLabel: "企微机器人",
    placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
  },
  {
    value: "slack",
    label: "Slack / Discord Webhook",
    shortLabel: "Slack / Discord",
    placeholder: "https://hooks.slack.com/services/...",
  },
  {
    value: "shoutrrr",
    label: "Telegram / 邮件",
    shortLabel: "Telegram / 邮件",
    placeholder: "telegram://token@telegram?chats=-10012345678",
  },
  {
    value: "webhook",
    label: "自定义 HTTP Webhook",
    shortLabel: "自定义 Webhook",
    placeholder: "https://ops.example.com/api/v1/alerts/webhook",
  },
];

export function detectChannelType(rawUrl: string): AlertChannelItem["type"] {
  const u = (rawUrl || "").trim().toLowerCase();
  if (u.includes("open.feishu.cn") || u.includes("open.larksuite.com")) return "feishu";
  if (u.includes("oapi.dingtalk.com")) return "dingtalk";
  if (u.includes("qyapi.weixin.qq.com")) return "wecom";
  if (u.includes("hooks.slack.com") || u.includes("discord.com/api/webhooks")) return "slack";
  if (u.startsWith("http://") || u.startsWith("https://")) return "webhook";
  return "shoutrrr";
}

export function describeChannelType(type?: string, url?: string): string {
  const resolved = (type || detectChannelType(url || "")) as AlertChannelItem["type"];
  if (resolved === "shoutrrr" && url) {
    const scheme = url.split("://")[0]?.toLowerCase();
    if (scheme === "telegram") return "Telegram";
    if (scheme === "smtp") return "SMTP 邮件";
    if (scheme === "bark") return "Bark 推送";
  }
  return CHANNEL_TYPE_OPTIONS.find((o) => o.value === resolved)?.shortLabel || "Webhook";
}

export function maskChannelUrl(rawUrl: string): string {
  const u = (rawUrl || "").trim();
  if (u.length <= 54) return u;
  return `${u.slice(0, 44)}...${u.slice(-6)}`;
}

function normalizeAlertChannels(
  items: Array<Partial<AlertChannelItem>>,
  idPrefix: string,
  defaultName: string,
): AlertChannelItem[] {
  return items
    .filter((item) => item && typeof item.url === "string" && item.url.trim() !== "")
    .map((item, idx) => {
      const url = (item.url || "").trim();
      return {
        id: item.id || `${idPrefix}${idx + 1}`,
        name: item.name?.trim() || defaultName,
        type: item.type || detectChannelType(url),
        url,
        secret: item.secret || "",
        consoleURL: item.consoleURL?.trim() || undefined,
        severity: item.severity === "critical" ? "critical" : "all",
        enabled: item.enabled !== false,
        state: item.state || "healthy",
        retryCount: item.retryCount || 0,
        lastDeliveryAt: item.lastDeliveryAt || "",
        lastError: item.lastError || "",
        nextRetryAt: item.nextRetryAt || "",
        pendingTitle: item.pendingTitle || "",
        pendingMessage: item.pendingMessage || "",
      };
    });
}

export function buildNotificationRecords(
  events: AuditEventItem[],
  channels: AlertChannelItem[],
  defaultScope: "tenant" | "platform" = "tenant",
  tenantName?: string,
): NotificationDeliveryRecord[] {
  const channelMap = new Map<string, AlertChannelItem>();
  for (const ch of channels) {
    channelMap.set(ch.id, ch);
  }
  const records: NotificationDeliveryRecord[] = [];
  const nowIso = new Date().toISOString();

  for (const ch of channels) {
    const hasChannelEvent = events.some(
      (ev) =>
        ev.resource === "alerts" &&
        ev.subresource === "webhook" &&
        (ev.resourceName === ch.id || ev.attributes?.channelID === ch.id),
    );
    if (ch.state === "retrying" && !hasChannelEvent) {
      records.push({
        id: `retrying-${ch.id}`,
        deliveryId: `retry-${ch.id}-${(ch.retryCount || 1) + 1}`,
        scope: defaultScope,
        tenant: tenantName,
        channelId: ch.id,
        channelName: ch.name,
        channelType: ch.type,
        status: "retrying",
        title: ch.pendingTitle || "告警通知投递重试中",
        message: ch.pendingMessage || "系统正在按指数退避策略自动重试投递该通知。",
        attempts: ch.retryCount || 1,
        error: ch.lastError || undefined,
        nextRetryAt: ch.nextRetryAt || undefined,
        timestamp: ch.lastDeliveryAt || nowIso,
      });
    } else if (ch.state === "failed" && ch.lastError && !hasChannelEvent) {
      records.push({
        id: `failed-${ch.id}`,
        deliveryId: `dlq-${ch.id}`,
        scope: defaultScope,
        tenant: tenantName,
        channelId: ch.id,
        channelName: ch.name,
        channelType: ch.type,
        status: "failed",
        title: ch.pendingTitle || "告警通知投递失败",
        message: ch.pendingMessage || "",
        attempts: ch.retryCount || 3,
        error: ch.lastError,
        timestamp: ch.lastDeliveryAt || nowIso,
      });
    }
  }

  for (const ev of events) {
    if (ev.resource !== "alerts" || ev.subresource !== "webhook") {
      continue;
    }
    const attrs = ev.attributes || {};
    if (attrs.silenced === "true") {
      continue;
    }
    const chId = attrs.channelID || ev.resourceName || "default";
    const knownCh = channelMap.get(chId);
    const isRetrying =
      ev.action === "WebhookPending" ||
      ev.action === "WebhookSending" ||
      ev.action === "WebhookRetrying";
    const isSuccess =
      !isRetrying &&
      (ev.action === "WebhookDelivered" || (ev.statusCode >= 200 && ev.statusCode < 400));
    const status: "delivered" | "retrying" | "failed" = isRetrying
      ? "retrying"
      : isSuccess
        ? "delivered"
        : "failed";
    const scope = (attrs.scope === "platform" ? "platform" : defaultScope) as "tenant" | "platform";
    records.push({
      id: ev.metadata.name,
      deliveryId: ev.requestID || ev.metadata.name,
      scope,
      tenant: ev.metadata.tenant || tenantName,
      channelId: chId,
      channelName: attrs.channelName || ev.targetName || knownCh?.name || chId,
      channelType: attrs.channelType || knownCh?.type || "webhook",
      status,
      title:
        attrs.title ||
        (isRetrying
          ? "告警通知投递重试中"
          : isSuccess
            ? "告警通知投递成功"
            : "告警通知投递失败 (3 次退避重试耗尽)"),
      message: attrs.message || "",
      attempts: Math.max(1, Number(attrs.attempts) || (isSuccess || isRetrying ? 1 : 3)),
      error: attrs.error || undefined,
      nextRetryAt: attrs.nextRetryAt || undefined,
      timestamp: ev.timestamp || ev.metadata.creationTimestamp || nowIso,
    });
  }

  return records;
}

export function parsePlatformAlertChannels(
  annotations?: Record<string, string>,
  specChannels?: Array<Partial<AlertChannelItem>>,
): AlertChannelItem[] {
  if (Array.isArray(specChannels) && specChannels.length > 0) {
    return normalizeAlertChannels(specChannels, "pch-", "平台默认通知渠道");
  }
  if (!annotations) return [];
  const raw = annotations["core.gamepanel.io/platform-alert-channels"]?.trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as Partial<AlertChannelItem>[];
    if (Array.isArray(parsed)) {
      return normalizeAlertChannels(parsed, "pch-", "平台默认通知渠道");
    }
  } catch {
    // Ignore malformed JSON
  }
  return [];
}

export function parseTenantAlertChannels(
  annotations?: Record<string, string>,
  specChannels?: Array<Partial<AlertChannelItem>>,
): AlertChannelItem[] {
  if (Array.isArray(specChannels) && specChannels.length > 0) {
    return normalizeAlertChannels(specChannels, "ch-", "默认通知渠道");
  }
  if (!annotations) return [];
  const raw = annotations["core.gamepanel.io/alert-channels"]?.trim();
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as Partial<AlertChannelItem>[];
      if (Array.isArray(parsed)) {
        return normalizeAlertChannels(parsed, "ch-", "默认通知渠道");
      }
    } catch {
      // Fall back to legacy single-webhook annotation
    }
  }
  const legacy = annotations["core.gamepanel.io/alert-webhook"]?.trim();
  if (legacy) {
    return [
      {
        id: "default",
        name: "默认通知渠道",
        type: detectChannelType(legacy),
        url: legacy,
        severity: "all",
        enabled: true,
        state: "healthy",
      },
    ];
  }
  return [];
}

export interface AlertSilenceItem {
  id: string;
  scope?: string;
  targetKind: "instances" | "nodes" | "*";
  targetName: string;
  ruleID?: string;
  reason?: string;
  createdBy?: string;
  startsAt?: string;
  expiresAt: string;
}

export const SILENCE_DURATION_OPTIONS: Array<{
  hours: 1 | 4 | 24;
  label: string;
}> = [
  { hours: 1, label: "1 小时" },
  { hours: 4, label: "4 小时" },
  { hours: 24, label: "24 小时" },
];

function normalizeAlertSilences(
  items: Array<Partial<AlertSilenceItem>>,
  idPrefix: string,
  defaultScope: string,
): AlertSilenceItem[] {
  const nowMs = Date.now();
  const out: AlertSilenceItem[] = [];
  items.forEach((item, idx) => {
    if (!item || !item.expiresAt) return;
    const expMs = new Date(item.expiresAt).getTime();
    if (!Number.isFinite(expMs) || expMs <= nowMs) return;
    const kind =
      item.targetKind === "instances" || item.targetKind === "nodes" ? item.targetKind : "*";
    const targetName = (item.targetName || "").trim() || "*";
    const ruleID = item.ruleID?.trim() === "*" ? "" : item.ruleID?.trim() || "";
    out.push({
      id: item.id?.trim() || `${idPrefix}${idx + 1}`,
      scope: item.scope?.trim() || defaultScope,
      targetKind: kind,
      targetName,
      ruleID: ruleID || undefined,
      reason: item.reason?.trim() || undefined,
      createdBy: item.createdBy?.trim() || undefined,
      startsAt: item.startsAt || undefined,
      expiresAt: new Date(expMs).toISOString(),
    });
  });
  return out;
}

export function parseTenantAlertSilences(
  annotations?: Record<string, string>,
  specSilences?: Array<Partial<AlertSilenceItem>>,
  tenantName = "",
): AlertSilenceItem[] {
  if (Array.isArray(specSilences)) {
    return normalizeAlertSilences(specSilences, "sil-", tenantName);
  }
  const raw = annotations?.["core.gamepanel.io/alert-silences"]?.trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as Array<Partial<AlertSilenceItem>>;
    return Array.isArray(parsed) ? normalizeAlertSilences(parsed, "sil-", tenantName) : [];
  } catch {
    return [];
  }
}

export function parsePlatformAlertSilences(
  annotations?: Record<string, string>,
  specSilences?: Array<Partial<AlertSilenceItem>>,
): AlertSilenceItem[] {
  if (Array.isArray(specSilences)) {
    return normalizeAlertSilences(specSilences, "psil-", "all");
  }
  const raw = annotations?.["core.gamepanel.io/platform-alert-silences"]?.trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as Array<Partial<AlertSilenceItem>>;
    return Array.isArray(parsed) ? normalizeAlertSilences(parsed, "psil-", "all") : [];
  } catch {
    return [];
  }
}

export function formatSilenceTargetLabel(s: AlertSilenceItem, isPlatform = false): string {
  if (s.targetKind === "nodes") {
    return s.targetName === "*" ? "全部计算节点" : `计算节点 ${s.targetName}`;
  }
  if (s.targetKind === "instances") {
    const prefix =
      isPlatform && s.scope && s.scope !== "all" && s.scope !== "platform"
        ? `工作区 ${s.scope} · `
        : "";
    return s.targetName === "*" ? `${prefix}全部实例` : `${prefix}实例 ${s.targetName}`;
  }
  if (isPlatform) {
    if (s.scope && s.scope !== "all" && s.scope !== "platform") {
      return `工作区 ${s.scope} 全部告警`;
    }
    return "集群全局维护静默";
  }
  return "当前工作区全部实例";
}

export function formatSilenceRemaining(expiresAt?: string): string {
  if (!expiresAt) return "免打扰中";
  const diffMs = new Date(expiresAt).getTime() - Date.now();
  if (!Number.isFinite(diffMs) || diffMs <= 0) return "即将恢复";
  const totalMins = Math.max(1, Math.ceil(diffMs / 60000));
  if (totalMins < 60) return `剩余 ${totalMins} 分钟`;
  const hrs = Math.floor(totalMins / 60);
  const mins = totalMins % 60;
  return mins > 0 ? `剩余 ${hrs} 小时 ${mins} 分钟` : `剩余 ${hrs} 小时`;
}

export function formatChannelDeliveryStatus(ch: AlertChannelItem): {
  badgeLabel: string;
  badgeTone: "success" | "warning" | "danger" | "neutral";
  dotColor: string;
  deliveryLabel: string;
  deliveryTextColor: string;
} {
  if (ch.state === "retrying") {
    const attempt = ch.retryCount || 1;
    let countdown = "即将重试";
    if (ch.nextRetryAt) {
      const remSec = Math.max(1, Math.round((new Date(ch.nextRetryAt).getTime() - Date.now()) / 1000));
      countdown = remSec < 60 ? `${remSec} 秒后重试` : `${Math.round(remSec / 60)} 分钟后重试`;
    }
    return {
      badgeLabel: "重试中",
      badgeTone: "warning",
      dotColor: "bg-[#B54708]",
      deliveryLabel: `第 ${attempt}/3 次 · ${countdown}`,
      deliveryTextColor: "text-[#B54708]",
    };
  }
  if (ch.state === "failed") {
    return {
      badgeLabel: "投递异常",
      badgeTone: "danger",
      dotColor: "bg-[#B42318]",
      deliveryLabel: `${ch.retryCount || 3} 次重试失败`,
      deliveryTextColor: "text-[#B42318]",
    };
  }
  let deliveryLabel = "就绪 · 等待触发";
  let deliveryTextColor = "text-[#14161F]";
  if (ch.lastDeliveryAt) {
    const diffMins = Math.max(0, Math.round((Date.now() - new Date(ch.lastDeliveryAt).getTime()) / 60000));
    deliveryTextColor = "text-[#1F7A45]";
    if (diffMins < 1) {
      deliveryLabel = "刚刚已送达";
    } else if (diffMins < 60) {
      deliveryLabel = `${diffMins} 分钟前已送达`;
    } else if (diffMins < 1440) {
      deliveryLabel = `${Math.round(diffMins / 60)} 小时前已送达`;
    } else {
      deliveryLabel = `${Math.round(diffMins / 1440)} 天前已送达`;
    }
  }
  return {
    badgeLabel: "正常",
    badgeTone: "success",
    dotColor: "bg-[#1F9D55]",
    deliveryLabel,
    deliveryTextColor,
  };
}

export function formatLogMessage(entry: PlatformLogEntry): string {
  const raw = entry.message || "";
  const f = entry.fields || {};
  if (raw === "audit.event" && f.method && f.requestURI) {
    const userPart = f.user ? `user=${f.user}` : "";
    const statusPart = f.status ? `status=${f.status}` : "";
    const durPart = f.duration ? `duration=${f.duration}` : "";
    const meta = [userPart, statusPart, durPart].filter(Boolean).join(", ");
    return `${f.method} ${f.requestURI}${meta ? ` (${meta})` : ""}`;
  }
  const cleaned = raw
    .replace(/^\[?\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\]?\s*(?:\[(?:LOG|INFO|WARN|ERROR|DEBUG)\]\s*)?/i, "")
    .trim();
  const extraPairs = Object.entries(f)
    .filter(([k]) => ["addr", "topic", "driver"].includes(k))
    .map(([k, v]) => `${k}=${v}`);
  if (extraPairs.length > 0) {
    return `${cleaned || raw} (${extraPairs.join(", ")})`;
  }
  return cleaned || raw;
}

export function prettyAuditPayload(raw?: string): { text: string; isJson: boolean } {
  const trimmed = (raw || "").trim();
  if (!trimmed) return { text: "", isJson: false };
  try {
    const parsed = JSON.parse(trimmed);
    return { text: JSON.stringify(parsed, null, 2), isJson: true };
  } catch {
    return { text: trimmed, isJson: false };
  }
}

export function formatDateTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function formatLogTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  const ms = String(d.getMilliseconds()).padStart(3, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${ms}`;
}

export function formatAlertDuration(startsAt?: string, state?: string): string {
  if (!startsAt) return "刚刚";
  const diffMs = Math.max(0, Date.now() - new Date(startsAt).getTime());
  const mins = Math.max(1, Math.round(diffMs / 60000));
  if (state === "resolved") {
    if (mins < 60) return `已恢复 (${mins} 分钟前)`;
    const hrs = Math.round(mins / 60);
    if (hrs < 24) return `已恢复 (${hrs} 小时前)`;
    return `已恢复 (${Math.round(hrs / 24)} 天前)`;
  }
  if (mins < 60) return `已持续 ${mins} 分钟`;
  const hrs = Math.floor(mins / 60);
  const rem = mins % 60;
  if (hrs < 24) return rem > 0 ? `已持续 ${hrs} 小时 ${rem} 分` : `已持续 ${hrs} 小时`;
  return `已持续 ${Math.round(hrs / 24)} 天`;
}

const ROLE_DISPLAY_MAP: Record<string, string> = {
  "instance-operator": "运维",
  "tenant-viewer": "只读",
  "tenant-admin": "管理员",
  "platform-admin": "平台管理员",
};

export function formatAuditRole(role?: string): string {
  if (!role) return "";
  return ROLE_DISPLAY_MAP[role] || role;
}

function isLegacyInviteTokenOrEventId(val?: string, eventId?: string): boolean {
  if (!val) return true;
  if (eventId && val === eventId) return true;
  if (val.includes(".") || val.startsWith("eyJ") || val.length > 48) return true;
  return /^[0-9a-f]{16}$/i.test(val);
}

export function formatAuditActor(actor?: string): string {
  const a = (actor || "").trim();
  if (!a || a === "system:anonymous") return "未登录访客";
  return a;
}

export function redactAuditURI(uri: string): string {
  return uri.replace(/(\/invites\/)[^/?#]+/g, "$1[REDACTED]");
}

export function describeAuditAction(ev: AuditEventItem): string {
  const { resource, subresource, verb, action } = ev;
  if (resource === "alerts" && subresource === "webhook") {
    if (action === "WebhookDelivered") return "告警通知投递成功";
    if (action === "WebhookDeliveryFailed") return "告警通知投递失败";
    if (action === "WebhookPending" || action === "WebhookSending" || action === "WebhookRetrying") {
      return "告警通知投递重试中";
    }
    return "告警通知投递事件";
  }
  if (resource === "invites") {
    const uri = ev.attributes?.requestURI || "";
    const isAccept =
      action === "accept" ||
      Boolean(ev.resourceName && (ev.resourceName.includes(".") || ev.resourceName.startsWith("eyJ"))) ||
      /\/invites\/[^/]+/.test(uri);
    return isAccept ? "接受协作邀请加入工作区" : "生成工作区邀请链接";
  }
  if (resource === "instances") {
    if (subresource === "state") {
      if (action === "Running") return "启动游戏服务器实例";
      if (action === "Stopped") return "停止游戏服务器实例";
      return "切换服务器运行状态";
    }
    if (subresource === "files") {
      if (action === "backup") return "创建实例全量备份";
      if (action === "restore") return "恢复实例备份快照";
      if (action === "delete_backup") return "删除实例备份快照";
      if (action === "write") return "修改实例配置文件";
      if (action === "upload") return "上传实例工作文件";
      if (action === "download") return "下载实例工作文件";
      if (action === "mkdir") return "新建实例工作目录";
      if (action === "delete" || action === "rm") return "删除实例工作文件";
      return "管理实例文件";
    }
    if (subresource === "exec") return "执行实例控制台命令";
    if (verb === "create") return "新建游戏服务器实例";
    if (verb === "patch" || verb === "update") return "更新实例资源配置";
    if (verb === "delete") return "销毁游戏服务器实例";
  }
  if (resource === "tenants") {
    if (verb === "create") return "新建工作区";
    if (verb === "patch" || verb === "update") return "调整工作区配额与设置";
    if (verb === "delete") return "删除工作区";
  }
  if (resource === "nodes") {
    if (verb === "create") return "注册计算节点";
    if (verb === "patch" || verb === "update") return "变更计算节点调度状态";
    if (verb === "delete") return "移除计算节点";
  }
  if (resource === "runtimeclasses") {
    if (verb === "create") return "新建运行环境规格";
    if (verb === "patch" || verb === "update") return "更新运行环境规格";
    if (verb === "delete") return "移除运行环境规格";
  }
  if (resource === "apps") {
    if (verb === "create") return "新建游戏应用模板";
    if (verb === "patch" || verb === "update") return "更新游戏应用模板";
    if (verb === "delete") return "移除游戏应用模板";
  }
  if (resource === "users") {
    if (verb === "create") return "创建平台用户";
    if (verb === "patch" || verb === "update") return "更新账号状态或密码";
    if (verb === "delete") return "删除平台用户";
  }
  if (resource === "tenantroles" || resource === "globalroles") {
    if (verb === "create") return "新建权限角色";
    if (verb === "patch" || verb === "update") return "更新权限角色";
    if (verb === "delete") return "删除权限角色";
  }
  if (resource === "tenantrolebindings" || resource === "globalrolebindings") {
    if (verb === "create") return "授予成员角色权限";
    if (verb === "delete") return "撤销成员角色权限";
  }
  const verbLabel: Record<string, string> = {
    create: "创建",
    update: "更新",
    patch: "变更",
    delete: "删除",
  };
  return `${verbLabel[verb] || verb} ${resource}`;
}

export function describeAuditTargetBase(ev: AuditEventItem): string {
  const tenant = ev.metadata.tenant;
  if (ev.resource === "invites") {
    const candidate = ev.targetName || ev.resourceName || "";
    if (isLegacyInviteTokenOrEventId(candidate, ev.metadata.name)) {
      return tenant || "—";
    }
    return tenant && tenant !== candidate ? `${tenant}/${candidate}` : candidate;
  }
  const name = ev.targetName || ev.resourceName || ev.metadata.name || "—";
  return tenant && tenant !== name ? `${tenant}/${name}` : name;
}

export function formatAuditPathShort(rawPath?: string, tenant?: string, instanceName?: string): string {
  if (!rawPath) return "";
  let p = rawPath.trim();
  if (tenant && instanceName) {
    const prefix = `tenants/${tenant}/instances/${instanceName}/`;
    if (p.startsWith(prefix)) {
      p = p.slice(prefix.length);
    }
  }
  // Strip hex-encoded backup note suffix (`--<hex>.tar.gz` -> `.tar.gz`) for concise table display
  p = p.replace(/--[0-9a-fA-F]{8,}(\.tar\.gz)$/, "$1");
  return p;
}

export function summarizeUserAgent(ua?: string): string {
  if (!ua) return "GamePanel Console Client";
  const s = ua.trim();
  if (!s.startsWith("Mozilla/")) return s;
  let browser = "Browser";
  if (s.includes("Edg/")) browser = "Edge";
  else if (s.includes("Chrome/")) browser = "Chrome";
  else if (s.includes("Firefox/")) browser = "Firefox";
  else if (s.includes("Safari/") && !s.includes("Chrome/")) browser = "Safari";

  let os = "";
  if (s.includes("Mac OS X") || s.includes("Macintosh")) os = "macOS";
  else if (s.includes("Windows NT")) os = "Windows";
  else if (s.includes("Linux")) os = "Linux";
  else if (s.includes("iPhone") || s.includes("iPad")) os = "iOS";
  else if (s.includes("Android")) os = "Android";

  return os ? `${browser} · ${os}` : browser;
}

export function describeAuditTarget(ev: AuditEventItem): string {
  const base = describeAuditTargetBase(ev);
  const rawRole = ev.attributes?.role;
  if (
    rawRole &&
    (ev.resource === "invites" || ev.resource === "tenantrolebindings" || ev.resource === "globalrolebindings")
  ) {
    return `${base} (${ROLE_DISPLAY_MAP[rawRole] || rawRole})`;
  }
  const name = ev.targetName || ev.resourceName || ev.metadata.name || "";
  const shortPath = formatAuditPathShort(ev.attributes?.path, ev.metadata.tenant, name);
  return shortPath ? `${base} (${shortPath})` : base;
}

export function describeAuditMethodPath(ev: AuditEventItem): string {
  const method =
    ev.attributes?.method ||
    (ev.verb === "create"
      ? "POST"
      : ev.verb === "patch"
        ? "PATCH"
        : ev.verb === "update"
          ? "PUT"
          : ev.verb === "delete"
            ? "DELETE"
            : ev.verb.toUpperCase());
  if (ev.attributes?.requestURI) {
    return `${method} ${redactAuditURI(ev.attributes.requestURI)}`;
  }
  const group = ev.apiGroup || "core.gamepanel.io";
  const tenantPart = ev.metadata.tenant ? `/tenants/${ev.metadata.tenant}` : "";
  const safeName =
    ev.resource === "invites" && isLegacyInviteTokenOrEventId(ev.resourceName, ev.metadata.name)
      ? "[REDACTED]"
      : ev.resourceName;
  const namePart = safeName ? `/${safeName}` : "";
  const subPart = ev.subresource ? `/${ev.subresource}` : "";
  return `${method} /apis/${group}/v1${tenantPart}/${ev.resource}${namePart}${subPart}`;
}

export function downloadCSV(filename: string, headers: string[], rows: string[][]) {
  const escape = (val: string) => `"${String(val ?? "").replace(/"/g, '""')}"`;
  const lines = [headers.map(escape).join(","), ...rows.map((r) => r.map(escape).join(","))];
  const blob = new Blob([`\uFEFF${lines.join("\n")}`], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
