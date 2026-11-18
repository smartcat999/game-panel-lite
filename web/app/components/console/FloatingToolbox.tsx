"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type UIEvent } from "react";
import { usePathname } from "next/navigation";
import {
  ArrowLeft,
  ChevronRight,
  Download,
  RefreshCw,
  RotateCw,
  Search,
  Terminal,
  Wrench,
  X,
} from "lucide-react";
import { useConsole } from "../../console/ConsoleProvider";
import {
  describeAuditAction,
  describeAuditMethodPath,
  describeAuditTarget,
  downloadCSV,
  formatAuditActor,
  formatDateTime,
  formatLogMessage,
  formatLogTime,
  prettyAuditPayload,
  summarizeUserAgent,
} from "../../console/lib/observability";
import type { AuditEventItem, PlatformLogEntry } from "../../console/lib/types";
import {
  InteractiveTerminal,
  type InteractiveTerminalHandle,
  type TerminalConnectionStatus,
} from "./servers/InteractiveTerminal";
import { TimelineHistogram, filterByHistogramBucket } from "./TimelineHistogram";
import {
  Badge,
  Button,
  EmptyState,
  IconButton,
  Input,
  Select,
  Skeleton,
  focusRing,
} from "./ui";

export type ToolboxTool = "logs" | "audits" | "terminal";

export interface OpenToolboxDetail {
  tool?: ToolboxTool;
  server?: string;
  node?: string;
  q?: string;
}

export interface ToolboxDirectoryItem {
  key: string;
  icon: ReactNode;
  title: string;
  description?: string;
  active?: boolean;
  onSelect: () => void;
}

export interface ToolboxDirectoryGroup {
  title: string;
  badge?: ReactNode;
  items: ToolboxDirectoryItem[];
}

export function ToolboxDirectoryCard({
  subtitle,
  groups,
}: {
  subtitle?: string;
  groups: ToolboxDirectoryGroup[];
}) {
  return (
    <aside
      role="dialog"
      aria-label="工具箱目录"
      className="fixed bottom-20 right-6 z-50 flex w-[min(360px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white shadow-[0_20px_44px_-18px_rgba(36,46,66,0.25)]"
    >
      {/* KubeSphere Dark Banner Header */}
      <div className="border-b border-[#EDF0F7] bg-[#F5F7FA] p-3">
        <div className="flex items-center gap-3 rounded-[10px] bg-[#242E42] px-3.5 py-3 text-white">
          <span className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-[9px] bg-white/12 text-[#C7D4FF]">
            <Wrench className="h-4 w-4" aria-hidden />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[14px] font-bold leading-tight text-white">工具箱</div>
            {subtitle && <div className="mt-0.5 truncate text-[11px] text-[#9AA5B8]">{subtitle}</div>}
          </div>
        </div>
      </div>

      {/* Grouped Utility Items */}
      <div className="max-h-[min(380px,calc(100dvh-14rem))] space-y-3 overflow-y-auto bg-white p-3">
        {groups.map((group) => (
          <div key={group.title}>
            <div className="flex items-center justify-between px-1.5 pb-1.5 text-[11px] font-semibold text-[#8C93A8]">
              <span>{group.title}</span>
              {group.badge}
            </div>
            <div className="space-y-1">
              {group.items.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onClick={item.onSelect}
                  className={cx(
                    "flex w-full items-center gap-2.5 rounded-[11px] border px-2.5 py-2 text-left transition-all",
                    item.active
                      ? "border-[#CBD3FF] bg-[#EEF1FF]"
                      : "border-transparent bg-transparent hover:border-[#EDF0F7] hover:bg-[#F6F8FC]",
                    focusRing,
                  )}
                >
                  <span
                    className={cx(
                      "flex h-8 w-8 shrink-0 items-center justify-center rounded-[9px]",
                      item.active
                        ? "bg-white text-[#2B3AE8] shadow-[0_2px_6px_rgba(43,58,232,0.14)]"
                        : "bg-[#F1F4FA] text-[#46506B]",
                    )}
                  >
                    {item.icon}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span
                      className={cx(
                        "block truncate text-[13px] font-semibold leading-snug",
                        item.active ? "text-[#2B3AE8]" : "text-[#14161F]",
                      )}
                    >
                      {item.title}
                    </span>
                  </span>
                  <ChevronRight
                    className={cx(
                      "h-3.5 w-3.5 shrink-0",
                      item.active ? "text-[#2B3AE8]" : "text-[#8C93A8]",
                    )}
                    aria-hidden
                  />
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
}

export function openFloatingToolbox(detail: OpenToolboxDetail = {}): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<OpenToolboxDetail>("gp:open-toolbox", { detail }));
}

type LogWindowOption = "15m" | "1h" | "6h" | "24h" | "7d";
type LogLevelOption = "ALL" | "INFO" | "WARN" | "ERROR";
type AuditWindowOption = "all" | "1h" | "24h" | "7d" | "30d";
type AuditResourceOption = "all" | "instances" | "backups" | "nodes" | "tenants" | "users" | "members";
type AuditStatusOption = "all" | "ok" | "error";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");
const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*[a-zA-Z]|\[\d+(?:;\d+)*m/g, "");

const WINDOW_DURATION_MS: Record<string, number> = {
  "15m": 15 * 60 * 1000,
  "1h": 60 * 60 * 1000,
  "6h": 6 * 60 * 60 * 1000,
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
  "30d": 30 * 24 * 60 * 60 * 1000,
};

function isWithinWindow(timestamp: string | undefined, win: string): boolean {
  const ms = WINDOW_DURATION_MS[win];
  if (!ms || !timestamp) return true;
  const t = Date.parse(timestamp);
  if (Number.isNaN(t)) return true;
  return t >= Date.now() - ms;
}

function logLevelTone(level?: string): "neutral" | "warning" | "danger" {
  const upper = (level || "INFO").toUpperCase();
  if (upper.startsWith("ERR") || upper.startsWith("FAT") || upper.startsWith("PAN")) return "danger";
  if (upper.startsWith("WARN")) return "warning";
  return "neutral";
}

function isErrorLog(level?: string): boolean {
  const upper = (level || "").toUpperCase();
  return upper.startsWith("ERR") || upper.startsWith("FAT") || upper.startsWith("PAN") || upper.startsWith("WARN");
}

function matchesAuditResource(ev: AuditEventItem, resource: AuditResourceOption): boolean {
  if (resource === "all") return true;
  if (resource === "backups") return ev.resource === "instances" && ev.subresource === "files";
  if (resource === "members") {
    return ev.resource === "invites" || ev.resource === "tenantroles" || ev.resource === "tenantrolebindings";
  }
  return ev.resource === resource;
}

function resolveApiAuditResource(resource: AuditResourceOption): string | null {
  if (resource === "instances" || resource === "backups") return "instances";
  if (resource === "nodes" || resource === "tenants" || resource === "users") return resource;
  return null;
}

function readApiError(status: number, text: string): string {
  try {
    const parsed = JSON.parse(text) as { message?: string; error?: string };
    return parsed.message || parsed.error || `请求失败 (${status})`;
  } catch {
    return text.trim() || `请求失败 (${status})`;
  }
}

const LOG_PAGE_SIZE = 100;

export function ToolboxLogsView({
  isPlatformMode,
  initialQuery = "",
  initialInstance = "",
  onTraceAudit,
}: {
  isPlatformMode: boolean;
  initialQuery?: string;
  initialInstance?: string;
  onTraceAudit?: (requestId: string) => void;
}) {
  const { apiFetch, instances, tenant, tenants } = useConsole();

  const [logTenant, setLogTenant] = useState("all");
  const [logInstance, setLogInstance] = useState(initialInstance || "all");
  const [platLogInstance, setPlatLogInstance] = useState(initialInstance || "");
  const [logLevel, setLogLevel] = useState<LogLevelOption>("ALL");
  const [logWindow, setLogWindow] = useState<LogWindowOption>("1h");
  const [logQ, setLogQ] = useState(initialQuery);
  const [debouncedLogQ, setDebouncedLogQ] = useState(initialQuery.trim());
  const [debouncedPlatInst, setDebouncedPlatInst] = useState(initialInstance.trim());
  const [logs, setLogs] = useState<PlatformLogEntry[]>([]);
  const [logTotalCount, setLogTotalCount] = useState(0);
  const [logNextOffset, setLogNextOffset] = useState(0);
  const [logHasMore, setLogHasMore] = useState(false);
  const [logLoading, setLogLoading] = useState(false);
  const [logLoadingMore, setLogLoadingMore] = useState(false);
  const logLoadingMoreRef = useRef(false);
  const [logError, setLogError] = useState("");
  const [logBucket, setLogBucket] = useState<number | null>(null);
  const [onlyLogErrors, setOnlyLogErrors] = useState(false);

  const effectiveWorkspaceInstance = useMemo(() => {
    if (logInstance === "all") return "all";
    if (instances.length > 0 && !instances.some((i) => i.metadata.name === logInstance)) {
      return "all";
    }
    return logInstance;
  }, [instances, logInstance]);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedLogQ(logQ.trim()), 250);
    return () => clearTimeout(t);
  }, [logQ]);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedPlatInst(platLogInstance.trim()), 250);
    return () => clearTimeout(t);
  }, [platLogInstance]);

  const buildLogQueryPath = useCallback(
    (offset = 0) => {
      const qs = new URLSearchParams({
        limit: String(LOG_PAGE_SIZE),
        window: logWindow,
      });
      if (offset > 0) qs.set("offset", String(offset));
      if (logLevel !== "ALL") qs.set("level", logLevel);
      if (debouncedLogQ) qs.set("q", debouncedLogQ);

      if (isPlatformMode) {
        qs.set("scope", "all");
        if (logTenant !== "all") qs.set("tenant", logTenant);
        if (debouncedPlatInst) qs.set("instance", debouncedPlatInst);
        return `/apis/core.gamepanel.io/v1/logs?${qs.toString()}`;
      }
      if (effectiveWorkspaceInstance !== "all") {
        qs.set("instance", effectiveWorkspaceInstance);
      }
      return `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/logs?${qs.toString()}`;
    },
    [
      logWindow,
      logLevel,
      debouncedLogQ,
      isPlatformMode,
      logTenant,
      debouncedPlatInst,
      effectiveWorkspaceInstance,
      tenant,
    ],
  );

  const loadLogs = useCallback(async () => {
    if (!isPlatformMode && !tenant) {
      setLogs([]);
      setLogTotalCount(0);
      setLogNextOffset(0);
      setLogHasMore(false);
      return;
    }
    logLoadingMoreRef.current = false;
    setLogLoadingMore(false);
    setLogLoading(true);
    setLogError("");
    try {
      const res = await apiFetch(buildLogQueryPath(0));
      if (!res.ok) {
        setLogError(readApiError(res.status, await res.text()));
        return;
      }
      const data = (await res.json()) as { items?: PlatformLogEntry[]; metadata?: { totalCount?: number } };
      const items = data.items || [];
      const total = Math.max(data.metadata?.totalCount || 0, items.length);
      setLogs(items);
      setLogTotalCount(total);
      setLogNextOffset(items.length);
      setLogHasMore(items.length > 0 && items.length < total);
    } catch (err) {
      setLogError(err instanceof Error ? err.message : "日志加载失败");
    } finally {
      setLogLoading(false);
    }
  }, [apiFetch, buildLogQueryPath, isPlatformMode, tenant]);

  const loadMoreLogs = useCallback(async () => {
    if ((!isPlatformMode && !tenant) || logLoading || logLoadingMoreRef.current || !logHasMore) {
      return;
    }
    logLoadingMoreRef.current = true;
    setLogLoadingMore(true);
    try {
      const res = await apiFetch(buildLogQueryPath(logNextOffset));
      if (!res.ok) {
        setLogError(readApiError(res.status, await res.text()));
        return;
      }
      const data = (await res.json()) as { items?: PlatformLogEntry[]; metadata?: { totalCount?: number } };
      const nextItems = data.items || [];
      const nextOffset = logNextOffset + nextItems.length;
      const nextTotal = Math.max(logTotalCount, data.metadata?.totalCount || 0, nextOffset);
      setLogs((prev) => {
        const keyOf = (l: PlatformLogEntry) =>
          `${l.timestamp || ""}|${l.service || ""}|${l.level || ""}|${l.message || ""}|${l.fields?.requestId || ""}`;
        const seen = new Set(prev.map(keyOf));
        return [...prev, ...nextItems.filter((l) => !seen.has(keyOf(l)))];
      });
      setLogTotalCount(nextTotal);
      setLogNextOffset(nextOffset);
      setLogHasMore(nextItems.length > 0 && nextOffset < nextTotal);
    } catch (err) {
      setLogError(err instanceof Error ? err.message : "加载更多日志失败");
    } finally {
      logLoadingMoreRef.current = false;
      setLogLoadingMore(false);
    }
  }, [
    apiFetch,
    buildLogQueryPath,
    isPlatformMode,
    tenant,
    logLoading,
    logHasMore,
    logNextOffset,
    logTotalCount,
  ]);

  const handleLogScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      const el = event.currentTarget;
      if (el.scrollHeight - el.scrollTop - el.clientHeight <= 160) {
        void loadMoreLogs();
      }
    },
    [loadMoreLogs],
  );

  useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  const filteredLogs = logs;

  const logHistogramItems = useMemo(
    () =>
      filteredLogs.map((l) => ({
        timestamp: l.timestamp || l.metadata.creationTimestamp,
        isError: isErrorLog(l.level),
      })),
    [filteredLogs],
  );

  const visibleLogs = useMemo(() => {
    const inBucket = filterByHistogramBucket(
      filteredLogs,
      (l) => l.timestamp || l.metadata.creationTimestamp,
      logWindow,
      logBucket,
    );
    return onlyLogErrors ? inBucket.filter((l) => isErrorLog(l.level)) : inBucket;
  }, [filteredLogs, logWindow, logBucket, onlyLogErrors]);

  const handleExportLogs = () => {
    if (visibleLogs.length === 0) return;
    downloadCSV(
      `${isPlatformMode ? "platform" : tenant}-logs.csv`,
      ["时间", "级别", "工作区", "服务", "服务器", "容器 ID", "节点", "日志内容", "请求 ID"],
      visibleLogs.map((l) => [
        l.timestamp || "",
        (l.level || "INFO").toUpperCase(),
        l.fields?.tenant || "平台系统",
        l.service || "",
        l.fields?.instance || l.fields?.container || "",
        l.fields?.containerId || "",
        l.fields?.node || "",
        stripAnsi(formatLogMessage(l)),
        l.fields?.requestId || "",
      ]),
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[#F7F8FC]/40">
      {/* Compact Unified Filter Bar */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[#EEF0F7] bg-white px-4 py-2.5">
        {isPlatformMode ? (
          <>
            <Select
              aria-label="按工作区或系统筛选日志"
              value={logTenant}
              onChange={(e) => {
                setLogTenant(e.target.value);
                setLogBucket(null);
              }}
              className="h-8.5 w-36 text-xs"
            >
              <option value="all">全部范围</option>
              <option value="_platform">平台系统服务</option>
              {tenants.map((t) => (
                <option key={t.metadata.name} value={t.metadata.name}>
                  {t.spec?.displayName || t.metadata.name}
                </option>
              ))}
            </Select>
            <Input
              aria-label="服务器或服务名称筛选"
              value={platLogInstance}
              onChange={(e) => {
                setPlatLogInstance(e.target.value);
                setLogBucket(null);
              }}
              placeholder="服务器/服务名…"
              className="h-8.5 w-40 text-xs"
            />
          </>
        ) : (
          <Select
            aria-label="按服务器筛选日志"
            value={effectiveWorkspaceInstance}
            onChange={(e) => {
              setLogInstance(e.target.value);
              setLogBucket(null);
            }}
            className="h-8.5 w-44 text-xs"
          >
            <option value="all">全部服务器</option>
            {instances.map((inst) => (
              <option key={inst.metadata.name} value={inst.metadata.name}>
                {inst.metadata.name}
              </option>
            ))}
          </Select>
        )}

        <Select
          aria-label="日志级别"
          value={logLevel}
          onChange={(e) => {
            setLogLevel(e.target.value as LogLevelOption);
            setLogBucket(null);
            setOnlyLogErrors(false);
          }}
          className="h-8.5 w-32 text-xs"
        >
          <option value="ALL">全部级别</option>
          <option value="INFO">INFO</option>
          <option value="WARN">WARN</option>
          <option value="ERROR">ERROR</option>
        </Select>

        <Select
          aria-label="时间范围"
          value={logWindow}
          onChange={(e) => {
            setLogWindow(e.target.value as LogWindowOption);
            setLogBucket(null);
          }}
          className="h-8.5 w-32 text-xs"
        >
          <option value="15m">近 15 分钟</option>
          <option value="1h">近 1 小时</option>
          <option value="6h">近 6 小时</option>
          <option value="24h">近 24 小时</option>
          <option value="7d">近 7 天</option>
        </Select>

        <div className="relative min-w-[150px] flex-1">
          <Search
            className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]"
            aria-hidden
          />
          <Input
            aria-label="搜索日志关键词"
            value={logQ}
            onChange={(e) => {
              setLogQ(e.target.value);
              setLogBucket(null);
            }}
            placeholder="搜索日志关键词…"
            className="h-8.5 pl-8 text-xs"
          />
        </div>

        <IconButton label="刷新日志" onClick={() => void loadLogs()}>
          <RefreshCw className={cx("h-3.5 w-3.5", logLoading && "animate-spin")} />
        </IconButton>
      </div>

      {/* Aggregated Timeline + Unified Container Logs List */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3" onScroll={handleLogScroll}>
        {logError ? (
          <div className="rounded-[14px] border border-[#F5C2C7] bg-[#FFF5F5] px-4 py-3 text-xs text-[#C62828]">
            {logError}
          </div>
        ) : logLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-24 rounded-[14px]" />
            <Skeleton className="h-48 rounded-[14px]" />
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="rounded-[16px] border border-[#E3E6F2] bg-white">
            <EmptyState
              title="未检索到匹配的日志"
              body="请尝试放宽时间范围、清除关键词或确认对应服务器正在运行。"
            />
          </div>
        ) : (
          <div className="overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white">
            <TimelineHistogram
              title="日志时间分布"
              windowKey={logWindow}
              items={logHistogramItems}
              selectedBucket={logBucket}
              onSelectBucket={setLogBucket}
              onlyErrors={onlyLogErrors}
              onToggleOnlyErrors={() => setOnlyLogErrors((v) => !v)}
              className="rounded-none border-b border-[#EEF0F7]"
            />

            {visibleLogs.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-2 px-4 py-8 text-center">
                <p className="text-xs text-[#5B6075]">当前筛选条件下暂无日志条目</p>
                <div className="flex items-center gap-3">
                  {logBucket !== null && (
                    <button
                      type="button"
                      onClick={() => setLogBucket(null)}
                      className="text-xs font-medium text-[#2B3AE8] hover:underline"
                    >
                      清除时段筛选
                    </button>
                  )}
                  {onlyLogErrors && (
                    <button
                      type="button"
                      onClick={() => setOnlyLogErrors(false)}
                      className="text-xs font-medium text-[#2B3AE8] hover:underline"
                    >
                      显示全部日志
                    </button>
                  )}
                </div>
              </div>
            ) : (
              <div>
                <div
                  className={cx(
                    "sticky top-0 z-10 hidden border-b border-[#E6E8F0] bg-[#FAFBFD] px-4 py-2 text-[11px] font-semibold text-[#697089] sm:grid sm:gap-2.5",
                    isPlatformMode
                      ? "sm:grid-cols-[88px_54px_112px_96px_minmax(0,1fr)] lg:grid-cols-[88px_54px_92px_78px_108px_98px_92px_minmax(0,1fr)]"
                      : "sm:grid-cols-[88px_54px_116px_98px_minmax(0,1fr)] lg:grid-cols-[88px_54px_78px_116px_98px_92px_minmax(0,1fr)]",
                  )}
                >
                  <span>时间</span>
                  <span>级别</span>
                  {isPlatformMode && <span className="hidden lg:block">工作区</span>}
                  <span className="hidden lg:block">服务</span>
                  <span>服务器</span>
                  <span>容器 ID</span>
                  <span className="hidden lg:block">节点</span>
                  <span>日志内容</span>
                </div>
                <div className="divide-y divide-[#EEF0F7]">
                  {visibleLogs.map((l, idx) => {
                    const id = l.metadata?.name || `${l.timestamp}-${idx}`;
                    const fields = l.fields || {};
                    const levelText = (l.level || "INFO").toUpperCase();
                    const ts = l.timestamp || l.metadata?.creationTimestamp;
                    const workspaceLabel = fields.tenant || "平台系统";
                    const serviceLabel = l.service || "—";
                    const serverLabel = fields.instance || fields.container || "—";
                    const containerIdLabel = fields.containerId || "—";
                    const nodeLabel = fields.node || "—";
                    const msg = stripAnsi(formatLogMessage(l));
                    return (
                      <div
                        key={id}
                        className={cx(
                          "grid grid-cols-[88px_54px_minmax(0,1fr)] items-start gap-2 px-4 py-2 text-xs transition-colors hover:bg-[#F7F8FC] sm:gap-2.5",
                          isPlatformMode
                            ? "sm:grid-cols-[88px_54px_112px_96px_minmax(0,1fr)] lg:grid-cols-[88px_54px_92px_78px_108px_98px_92px_minmax(0,1fr)]"
                            : "sm:grid-cols-[88px_54px_116px_98px_minmax(0,1fr)] lg:grid-cols-[88px_54px_78px_116px_98px_92px_minmax(0,1fr)]",
                        )}
                      >
                        <span
                          className="font-mono text-[11px] leading-5 tabular-nums text-[#697089]"
                          title={formatDateTime(ts)}
                        >
                          {formatLogTime(ts)}
                        </span>
                        <span className="pt-0.5">
                          <Badge tone={logLevelTone(levelText)}>{levelText}</Badge>
                        </span>
                        {isPlatformMode && (
                          <span
                            className={cx(
                              "hidden truncate pt-0.5 font-mono text-[11px] lg:block",
                              fields.tenant ? "text-[#43495E]" : "text-[#8A90A6]",
                            )}
                            title={workspaceLabel}
                          >
                            {workspaceLabel}
                          </span>
                        )}
                        <span
                          className="hidden truncate pt-0.5 font-mono text-[11px] text-[#5B6075] lg:block"
                          title={serviceLabel}
                        >
                          {serviceLabel}
                        </span>
                        <span
                          className={cx(
                            "hidden truncate pt-0.5 font-mono text-[11px] font-medium sm:block",
                            fields.instance ? "text-[#2B3AE8]" : "text-[#43495E]",
                          )}
                          title={serverLabel}
                        >
                          {serverLabel}
                        </span>
                        <span
                          className="hidden truncate pt-0.5 font-mono text-[11px] text-[#5B6075] sm:block"
                          title={fields.containerId || fields.container || ""}
                        >
                          {containerIdLabel}
                        </span>
                        <span
                          className="hidden truncate pt-0.5 font-mono text-[11px] text-[#5B6075] lg:block"
                          title={nodeLabel}
                        >
                          {nodeLabel}
                        </span>
                        <div className="min-w-0 font-mono text-[11.5px] leading-relaxed text-[#14161F]">
                          <span className="mr-2 inline-block rounded bg-[#EEF1FF] px-1.5 py-0.5 text-[10px] font-medium text-[#2B3AE8] sm:hidden">
                            {serverLabel}
                          </span>
                          <span className="break-words">{msg}</span>
                          {isPlatformMode && fields.requestId && onTraceAudit && (
                            <button
                              type="button"
                              onClick={() => onTraceAudit(fields.requestId)}
                              className="ml-2 inline-flex font-sans text-[11px] font-medium text-[#2B3AE8] hover:underline"
                            >
                              关联审计
                            </button>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
                {logLoadingMore && (
                  <div className="flex items-center justify-center gap-1.5 border-t border-[#EEF0F7] py-3 text-xs text-[#5B6075]">
                    <RefreshCw className="h-3.5 w-3.5 animate-spin text-[#2B3AE8]" />
                    <span>正在加载更多日志…</span>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Concise Status Footer */}
      <div className="flex shrink-0 items-center justify-between gap-2 border-t border-[#EEF0F7] bg-white px-4 py-2 text-xs text-[#5B6075]">
        <span className="tabular-nums">
          {logBucket !== null || onlyLogErrors
            ? `筛选 ${visibleLogs.length} / 已加载 ${filteredLogs.length} 条${logTotalCount > filteredLogs.length ? `（总计 ${logTotalCount.toLocaleString()} 条）` : ""}`
            : logTotalCount > visibleLogs.length
              ? `已加载 ${visibleLogs.length} / 共 ${logTotalCount.toLocaleString()} 条日志`
              : `共 ${visibleLogs.length} 条日志`}
          {onlyLogErrors ? " · 仅看异常" : ""}
        </span>
        <button
          type="button"
          disabled={visibleLogs.length === 0}
          onClick={handleExportLogs}
          className={cx(
            "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium transition-colors",
            visibleLogs.length === 0
              ? "cursor-not-allowed text-[#A0A6BA]"
              : "text-[#2B3AE8] hover:bg-[#EEF1FF]",
            focusRing,
          )}
        >
          <Download className="h-3.5 w-3.5" />
          <span>导出 CSV</span>
        </button>
      </div>
    </div>
  );
}

function auditEventKey(ev: AuditEventItem, idx: number): string {
  return ev.metadata?.name || `${ev.requestID || idx}-${idx}`;
}

function resolveAuditMethod(ev: AuditEventItem): string {
  return (
    ev.attributes?.method ||
    (ev.verb === "create"
      ? "POST"
      : ev.verb === "patch"
        ? "PATCH"
        : ev.verb === "update"
          ? "PUT"
          : ev.verb === "delete"
            ? "DELETE"
            : ev.verb.toUpperCase())
  ).toUpperCase();
}

export function ToolboxAuditsView({
  isPlatformMode,
  initialQuery = "",
  onTraceLogs,
}: {
  isPlatformMode: boolean;
  initialQuery?: string;
  onTraceLogs?: (requestId: string) => void;
}) {
  const { apiFetch, tenant, tenants } = useConsole();

  const [auditTenant, setAuditTenant] = useState("all");
  const [auditWindow, setAuditWindow] = useState<AuditWindowOption>("24h");
  const [auditResource, setAuditResource] = useState<AuditResourceOption>("all");
  const [auditStatus, setAuditStatus] = useState<AuditStatusOption>("all");
  const [auditQ, setAuditQ] = useState(initialQuery);
  const [audits, setAudits] = useState<AuditEventItem[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditError, setAuditError] = useState("");
  const [auditBucket, setAuditBucket] = useState<number | null>(null);
  const [onlyAuditErrors, setOnlyAuditErrors] = useState(false);
  const [selectedAuditId, setSelectedAuditId] = useState<string | null>(null);

  const loadAudits = useCallback(async () => {
    if (!isPlatformMode && !tenant) {
      setAudits([]);
      return;
    }
    setAuditLoading(true);
    setAuditError("");
    try {
      const qs = new URLSearchParams({ limit: "200" });
      if (auditWindow !== "all") qs.set("window", auditWindow);
      const apiRes = resolveApiAuditResource(auditResource);
      if (apiRes) qs.set("resource", apiRes);

      let path: string;
      if (isPlatformMode) {
        qs.set("tenant", auditTenant);
        path = `/apis/core.gamepanel.io/v1/events?${qs.toString()}`;
      } else {
        path = `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/events?${qs.toString()}`;
      }

      const res = await apiFetch(path);
      if (!res.ok) {
        setAuditError(readApiError(res.status, await res.text()));
        return;
      }
      const data = (await res.json()) as { items?: AuditEventItem[] };
      setAudits(data.items || []);
    } catch (err) {
      setAuditError(err instanceof Error ? err.message : "审计事件加载失败");
    } finally {
      setAuditLoading(false);
    }
  }, [apiFetch, isPlatformMode, tenant, auditWindow, auditResource, auditTenant]);

  useEffect(() => {
    void loadAudits();
  }, [loadAudits]);

  const filteredAudits = useMemo(() => {
    const q = auditQ.trim().toLowerCase();
    return audits.filter((ev) => {
      if (
        ev.resource === "metering" ||
        (ev.resource === "alerts" && (ev.subresource === "webhook" || ev.subresource === "state"))
      ) {
        return false;
      }
      if (auditStatus === "error" && ev.statusCode < 400) return false;
      if (auditStatus === "ok" && ev.statusCode >= 400) return false;
      if (!matchesAuditResource(ev, auditResource)) return false;
      if (auditWindow !== "all" && !isWithinWindow(ev.timestamp || ev.metadata.creationTimestamp, auditWindow)) {
        return false;
      }
      if (!q) return true;
      return (
        ev.actor.toLowerCase().includes(q) ||
        describeAuditAction(ev).toLowerCase().includes(q) ||
        describeAuditTarget(ev).toLowerCase().includes(q) ||
        (ev.clientIP || "").toLowerCase().includes(q) ||
        (ev.requestID || "").toLowerCase().includes(q)
      );
    });
  }, [audits, auditStatus, auditResource, auditWindow, auditQ]);

  const auditHistogramWindow = auditWindow === "all" ? "30d" : auditWindow;

  const auditHistogramItems = useMemo(
    () =>
      filteredAudits.map((ev) => ({
        timestamp: ev.timestamp || ev.metadata.creationTimestamp,
        isError: ev.statusCode >= 400,
      })),
    [filteredAudits],
  );

  const visibleAudits = useMemo(() => {
    const inBucket = filterByHistogramBucket(
      filteredAudits,
      (ev) => ev.timestamp || ev.metadata.creationTimestamp,
      auditHistogramWindow,
      auditBucket,
    );
    return onlyAuditErrors ? inBucket.filter((ev) => ev.statusCode >= 400) : inBucket;
  }, [filteredAudits, auditHistogramWindow, auditBucket, onlyAuditErrors]);

  const selectedAudit = useMemo(() => {
    if (!selectedAuditId) return null;
    return visibleAudits.find((ev, idx) => auditEventKey(ev, idx) === selectedAuditId) || null;
  }, [visibleAudits, selectedAuditId]);

  const handleExportAudits = () => {
    if (visibleAudits.length === 0) return;
    downloadCSV(
      `${isPlatformMode ? "platform" : tenant}-audits.csv`,
      ["时间", "操作动作", "请求方法", "目标对象", "操作人", "工作区", "客户端 IP", "耗时", "状态码", "请求 ID"],
      visibleAudits.map((ev) => [
        ev.timestamp || ev.metadata.creationTimestamp || "",
        describeAuditAction(ev),
        resolveAuditMethod(ev),
        describeAuditTarget(ev),
        formatAuditActor(ev.actor),
        ev.attributes?.tenant || ev.metadata.tenant || "平台",
        ev.clientIP || "",
        ev.attributes?.duration || "",
        String(ev.statusCode),
        ev.requestID || "",
      ]),
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[#F7F8FC]/40">
      {/* Audit Filter Bar */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[#EEF0F7] bg-white px-4 py-2.5">
        {isPlatformMode && (
          <Select
            aria-label="按工作区筛选审计"
            value={auditTenant}
            onChange={(e) => {
              setAuditTenant(e.target.value);
              setAuditBucket(null);
            }}
            className="h-8.5 w-36 text-xs"
          >
            <option value="all">全部工作区</option>
            {tenants.map((t) => (
              <option key={t.metadata.name} value={t.metadata.name}>
                {t.spec?.displayName || t.metadata.name}
              </option>
            ))}
          </Select>
        )}

        <Select
          aria-label="审计资源类型"
          value={auditResource}
          onChange={(e) => {
            setAuditResource(e.target.value as AuditResourceOption);
            setAuditBucket(null);
          }}
          className="h-8.5 w-32 text-xs"
        >
          <option value="all">全部资源</option>
          <option value="instances">游戏实例</option>
          <option value="backups">备份与文件</option>
          <option value="nodes">节点</option>
          <option value="tenants">工作区</option>
          <option value="users">用户</option>
          <option value="members">成员与角色</option>
        </Select>

        <Select
          aria-label="审计状态"
          value={auditStatus}
          onChange={(e) => {
            setAuditStatus(e.target.value as AuditStatusOption);
            setAuditBucket(null);
            setOnlyAuditErrors(false);
          }}
          className="h-8.5 w-28 text-xs"
        >
          <option value="all">全部状态</option>
          <option value="ok">正常 (&lt;400)</option>
          <option value="error">异常 (≥400)</option>
        </Select>

        <Select
          aria-label="审计时间范围"
          value={auditWindow}
          onChange={(e) => {
            setAuditWindow(e.target.value as AuditWindowOption);
            setAuditBucket(null);
          }}
          className="h-8.5 w-32 text-xs"
        >
          <option value="1h">近 1 小时</option>
          <option value="24h">近 24 小时</option>
          <option value="7d">近 7 天</option>
          <option value="30d">近 30 天</option>
          <option value="all">全部时间</option>
        </Select>

        <div className="relative min-w-[160px] flex-1">
          <Search
            className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]"
            aria-hidden
          />
          <Input
            aria-label="搜索审计事件"
            value={auditQ}
            onChange={(e) => {
              setAuditQ(e.target.value);
              setAuditBucket(null);
            }}
            placeholder="搜索操作人、目标或请求 ID…"
            className="h-8.5 pl-8 text-xs"
          />
        </div>

        <IconButton label="刷新审计事件" onClick={() => void loadAudits()}>
          <RefreshCw className={cx("h-3.5 w-3.5", auditLoading && "animate-spin")} />
        </IconButton>
      </div>

      {/* Full-Width Audit Workspace + On-Demand Detail Drawer */}
      <div className="relative min-h-0 flex-1 overflow-hidden">
        {auditError ? (
          <div className="p-4">
            <div className="rounded-[14px] border border-[#F5C2C7] bg-[#FFF5F5] px-4 py-3 text-xs text-[#C62828]">
              {auditError}
            </div>
          </div>
        ) : auditLoading ? (
          <div className="space-y-3 p-4">
            <Skeleton className="h-24 rounded-[14px]" />
            <Skeleton className="h-48 rounded-[14px]" />
          </div>
        ) : filteredAudits.length === 0 ? (
          <div className="p-4">
            <div className="rounded-[16px] border border-[#E3E6F2] bg-white">
              <EmptyState
                title="未检索到匹配的操作审计记录"
                body="当工作区或平台发生实例控制、文件修改、成员权限或配额变更时，审计记录会自动聚合至此。"
              />
            </div>
          </div>
        ) : (
          <div className="flex h-full min-h-0 flex-col bg-white">
            <TimelineHistogram
              title={isPlatformMode ? "平台审计事件时间线" : "工作区审计事件时间线"}
              windowKey={auditHistogramWindow}
              items={auditHistogramItems}
              selectedBucket={auditBucket}
              onSelectBucket={setAuditBucket}
              onlyErrors={onlyAuditErrors}
              onToggleOnlyErrors={() => setOnlyAuditErrors((v) => !v)}
              className="shrink-0 rounded-none border-b border-[#EEF0F7]"
            />

            {visibleAudits.length === 0 ? (
              <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 py-8 text-center">
                <p className="text-xs text-[#5B6075]">当前筛选条件下暂无审计事件</p>
                <div className="flex items-center gap-3">
                  {auditBucket !== null && (
                    <button
                      type="button"
                      onClick={() => setAuditBucket(null)}
                      className="text-xs font-medium text-[#2B3AE8] hover:underline"
                    >
                      清除筛选
                    </button>
                  )}
                  {onlyAuditErrors && (
                    <button
                      type="button"
                      onClick={() => setOnlyAuditErrors(false)}
                      className="text-xs font-medium text-[#2B3AE8] hover:underline"
                    >
                      显示全部事件
                    </button>
                  )}
                </div>
              </div>
            ) : (
              <>
                <div className="hidden shrink-0 items-center gap-2.5 border-b border-[#E6E8F0] bg-[#FAFBFD] px-4 py-2 text-[11px] font-semibold text-[#697089] sm:grid sm:grid-cols-[88px_minmax(0,1fr)_minmax(0,1.2fr)_96px_58px]">
                  <span>时间</span>
                  <span>操作动作</span>
                  <span>目标对象</span>
                  <span>操作人</span>
                  <span className="text-right">状态码</span>
                </div>
                <div className="min-h-0 flex-1 divide-y divide-[#EEF0F7] overflow-y-auto">
                  {visibleAudits.map((ev, idx) => {
                    const id = auditEventKey(ev, idx);
                    const isSelected = selectedAuditId === id;
                    const ts = ev.timestamp || ev.metadata?.creationTimestamp;
                    const methodText = resolveAuditMethod(ev);
                    const actorLabel = formatAuditActor(ev.actor);
                    return (
                      <button
                        key={id}
                        type="button"
                        aria-pressed={isSelected}
                        onClick={() => setSelectedAuditId((prev) => (prev === id ? null : id))}
                        className={cx(
                          "grid w-full cursor-pointer grid-cols-[88px_minmax(0,1fr)_58px] items-center gap-2 border-l-2 px-4 py-2.5 text-left text-xs transition-colors sm:grid-cols-[88px_minmax(0,1fr)_minmax(0,1.2fr)_96px_58px] sm:gap-2.5",
                          isSelected
                            ? "border-l-[#2B3AE8] bg-[#EEF1FF]/70"
                            : "border-l-transparent hover:bg-[#F7F8FC]",
                          focusRing,
                        )}
                      >
                        <span
                          className="font-mono text-[11px] tabular-nums text-[#697089]"
                          title={formatDateTime(ts)}
                        >
                          {formatLogTime(ts)}
                        </span>
                        <span className="flex min-w-0 items-center gap-1.5">
                          <span className="hidden shrink-0 font-mono text-[11px] text-[#697089] sm:inline">
                            {methodText}
                          </span>
                          <span className="truncate font-medium text-[#14161F]">
                            {describeAuditAction(ev)}
                          </span>
                        </span>
                        <span
                          className="hidden truncate font-mono text-[11px] text-[#5B6075] sm:block"
                          title={describeAuditTarget(ev)}
                        >
                          {describeAuditTarget(ev)}
                        </span>
                        <span
                          className="hidden truncate text-[11px] text-[#5B6075] sm:block"
                          title={[actorLabel, ev.clientIP, ev.attributes?.duration].filter(Boolean).join(" · ")}
                        >
                          {actorLabel}
                        </span>
                        <span className="flex justify-end">
                          <Badge tone={ev.statusCode >= 400 ? "danger" : "success"}>
                            {ev.statusCode}
                          </Badge>
                        </span>
                      </button>
                    );
                  })}
                </div>
              </>
            )}
          </div>
        )}

        {selectedAudit && (
          <div
            className="absolute inset-0 z-20 flex justify-end bg-[#14161F]/18"
            onClick={(e) => {
              if (e.target === e.currentTarget) setSelectedAuditId(null);
            }}
          >
            <div
              role="dialog"
              aria-label="审计详情"
              className="flex h-full w-[min(500px,100%)] flex-col border-l border-[#E3E6F2] bg-[#FAFBFD] shadow-[-18px_0_44px_-16px_rgba(20,22,31,0.22)]"
            >
              <div className="flex h-11 shrink-0 items-center justify-between border-b border-[#E6E8F0] bg-white px-4">
                <span className="truncate text-[13px] font-semibold text-[#14161F]">
                  {describeAuditAction(selectedAudit)}
                </span>
                <button
                  type="button"
                  aria-label="关闭审计详情"
                  onClick={() => setSelectedAuditId(null)}
                  className={cx(
                    "inline-flex h-7 w-7 items-center justify-center rounded-md text-[#697089] transition-colors hover:bg-[#F1F3F9] hover:text-[#14161F]",
                    focusRing,
                  )}
                >
                  <X className="h-4 w-4" aria-hidden />
                </button>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto p-4">
                <AuditEventInspector
                  ev={selectedAudit}
                  isPlatformMode={isPlatformMode}
                  onTraceLogs={onTraceLogs}
                />
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Footer */}
      <div className="flex shrink-0 items-center justify-between border-t border-[#EEF0F7] bg-white px-4 py-2 text-xs text-[#5B6075]">
        <span className="tabular-nums">
          {auditBucket !== null || onlyAuditErrors
            ? `共 ${visibleAudits.length} / ${filteredAudits.length} 条审计记录`
            : `共 ${visibleAudits.length} 条审计记录`}
          {onlyAuditErrors ? " · 仅看异常" : ""}
        </span>
        <button
          type="button"
          disabled={visibleAudits.length === 0}
          onClick={handleExportAudits}
          className={cx(
            "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium transition-colors",
            visibleAudits.length === 0
              ? "cursor-not-allowed text-[#A0A6BA]"
              : "text-[#2B3AE8] hover:bg-[#EEF1FF]",
            focusRing,
          )}
        >
          <Download className="h-3.5 w-3.5" />
          <span>导出 CSV</span>
        </button>
      </div>
    </div>
  );
}

function AuditEventInspector({
  ev,
  isPlatformMode,
  onTraceLogs,
}: {
  ev: AuditEventItem;
  isPlatformMode: boolean;
  onTraceLogs?: (requestId: string) => void;
}) {
  const ts = ev.timestamp || ev.metadata?.creationTimestamp;
  const evTenant = ev.attributes?.tenant || ev.metadata?.tenant || "平台";
  const durationText = ev.attributes?.duration || "—";
  const methodText = resolveAuditMethod(ev);
  const reqPayload = prettyAuditPayload(ev.attributes?.requestBody);
  const respPayload = prettyAuditPayload(ev.attributes?.responseBody);

  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5 rounded-[12px] bg-[#F7F8FC] px-3.5 py-3 text-[11px]">
        <div>
          <dt className="text-[#8A90A6]">发生时间</dt>
          <dd className="mt-0.5 truncate font-mono font-medium text-[#14161F]">
            {formatDateTime(ts)}
          </dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">状态码</dt>
          <dd className="mt-0.5">
            <Badge tone={ev.statusCode >= 400 ? "danger" : "success"}>{ev.statusCode}</Badge>
          </dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">操作人</dt>
          <dd className="mt-0.5 truncate font-mono font-medium text-[#14161F]" title={ev.actor}>
            {formatAuditActor(ev.actor)}
          </dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">工作区</dt>
          <dd className="mt-0.5 truncate font-mono font-medium text-[#14161F]">{evTenant}</dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">请求方法</dt>
          <dd className="mt-0.5 font-mono font-medium text-[#14161F]">{methodText}</dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">请求耗时</dt>
          <dd className="mt-0.5 truncate font-mono font-medium text-[#14161F]">{durationText}</dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">客户端 IP</dt>
          <dd className="mt-0.5 truncate font-mono text-[#14161F]">{ev.clientIP || "—"}</dd>
        </div>
        <div>
          <dt className="text-[#8A90A6]">客户端环境</dt>
          <dd
            className="mt-0.5 truncate font-mono text-[#14161F]"
            title={ev.attributes?.userAgent || ""}
          >
            {summarizeUserAgent(ev.attributes?.userAgent) || "—"}
          </dd>
        </div>
        <div className="col-span-2">
          <dt className="flex items-center justify-between gap-1 text-[#8A90A6]">
            <span>请求 ID</span>
            {isPlatformMode && ev.requestID && onTraceLogs && (
              <button
                type="button"
                onClick={() => onTraceLogs(ev.requestID!)}
                className="font-sans text-[11px] font-medium text-[#2B3AE8] hover:underline"
              >
                追踪日志
              </button>
            )}
          </dt>
          <dd className="mt-0.5 break-all font-mono text-[#14161F]">{ev.requestID || "—"}</dd>
        </div>
        <div className="col-span-2">
          <dt className="text-[#8A90A6]">请求路径</dt>
          <dd className="mt-0.5 break-all font-mono text-[#14161F]">
            {describeAuditMethodPath(ev).replace(/^[A-Z]+\s+/, "")}
          </dd>
        </div>
      </dl>

      <div>
        <div className="mb-1 flex items-center justify-between text-[11px] font-medium text-[#5B6075]">
          <span>请求载荷</span>
          {reqPayload.text && (
            <span className="font-mono text-[10px] text-[#8A90A6]">
              {reqPayload.isJson ? "JSON · 已脱敏" : "文本"}
            </span>
          )}
        </div>
        {reqPayload.text ? (
          <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-all rounded-[10px] bg-[#14161F] p-3 font-mono text-[11px] leading-relaxed text-[#E6E8F0]">
            {reqPayload.text}
          </pre>
        ) : (
          <div className="rounded-[10px] border border-dashed border-[#DDE1EE] bg-white/70 px-3 py-2.5 text-[11px] text-[#8A90A6]">
            暂无请求载荷
          </div>
        )}
      </div>

      <div>
        <div className="mb-1 flex items-center justify-between text-[11px] font-medium text-[#5B6075]">
          <span>响应载荷</span>
          {respPayload.text && (
            <span className="font-mono text-[10px] text-[#8A90A6]">
              {respPayload.isJson ? "JSON · 已脱敏" : "文本"}
            </span>
          )}
        </div>
        {respPayload.text ? (
          <pre className="max-h-52 overflow-auto whitespace-pre-wrap break-all rounded-[10px] bg-[#14161F] p-3 font-mono text-[11px] leading-relaxed text-[#D8E8FF]">
            {respPayload.text}
          </pre>
        ) : (
          <div className="rounded-[10px] border border-dashed border-[#DDE1EE] bg-white/70 px-3 py-2.5 text-[11px] text-[#8A90A6]">
            暂无响应载荷
          </div>
        )}
      </div>
    </div>
  );
}

export default function FloatingToolbox() {
  const pathname = usePathname();
  const { isAdmin, tenant, nodes } = useConsole();

  const isPlatformMode = Boolean(pathname?.startsWith("/console/admin")) && isAdmin;
  const hideDock = Boolean(pathname?.startsWith("/console/servers/"));

  const [open, setOpen] = useState(false);
  const [stage, setStage] = useState<"menu" | "panel">("menu");
  const [activeTool, setActiveTool] = useState<ToolboxTool>("logs");
  const [logSeed, setLogSeed] = useState<{ key: number; q?: string; instance?: string }>({ key: 0 });
  const [auditSeed, setAuditSeed] = useState<{ key: number; q?: string }>({ key: 0 });
  const [selectedNode, setSelectedNode] = useState("");
  const [terminalActivated, setTerminalActivated] = useState(false);
  const [termStatus, setTermStatus] = useState<TerminalConnectionStatus>("idle");
  const terminalRef = useRef<InteractiveTerminalHandle>(null);

  const resolvedNodeName = useMemo(() => {
    if (selectedNode && nodes.some((n) => n.metadata.name === selectedNode)) {
      return selectedNode;
    }
    const readyNode = nodes.find((n) => (n.status?.phase || "") === "Ready");
    return readyNode?.metadata.name || nodes[0]?.metadata.name || selectedNode || "";
  }, [nodes, selectedNode]);

  const selectedNodeObj = useMemo(
    () => nodes.find((n) => n.metadata.name === resolvedNodeName),
    [nodes, resolvedNodeName],
  );
  const selectedNodeReady = (selectedNodeObj?.status?.phase || "") === "Ready";

  useEffect(() => {
    const onCustomOpen = (ev: Event) => {
      const detail = (ev as CustomEvent<OpenToolboxDetail>).detail || {};
      if (detail.tool === "terminal" && isAdmin) {
        if (detail.node) setSelectedNode(detail.node);
        setActiveTool("terminal");
        setTerminalActivated(true);
      } else if (detail.tool === "audits") {
        setAuditSeed((prev) => ({ key: prev.key + 1, q: detail.q }));
        setActiveTool("audits");
      } else {
        setLogSeed((prev) => ({ key: prev.key + 1, q: detail.q, instance: detail.server }));
        setActiveTool("logs");
      }
      setStage("panel");
      setOpen(true);
    };
    window.addEventListener("gp:open-toolbox", onCustomOpen);
    return () => window.removeEventListener("gp:open-toolbox", onCustomOpen);
  }, [isAdmin]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && (stage === "menu" || activeTool !== "terminal")) {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, stage, activeTool]);

  useEffect(() => {
    if (!isAdmin || !open || stage !== "panel" || activeTool !== "terminal") return;
    setTerminalActivated(true);
    const id = setTimeout(() => {
      terminalRef.current?.fit();
      terminalRef.current?.focus();
    }, 20);
    return () => clearTimeout(id);
  }, [isAdmin, open, stage, activeTool, resolvedNodeName]);

  const directoryGroups: ToolboxDirectoryGroup[] = useMemo(() => {
    if (!isAdmin) return [];
    return [
      {
        title: "节点运维",
        badge: nodes.length > 0 ? <span className="font-mono">{nodes.length} 台</span> : undefined,
        items: [
          {
            key: "terminal",
            icon: <Terminal className="h-4 w-4" />,
            title: "节点终端",
            active: stage === "panel" && activeTool === "terminal",
            onSelect: () => {
              setActiveTool("terminal");
              setStage("panel");
              setTerminalActivated(true);
            },
          },
        ],
      },
    ];
  }, [stage, activeTool, isAdmin, nodes.length]);

  if (!isAdmin || hideDock) {
    return null;
  }

  const scopeLabel = isPlatformMode ? "全平台" : tenant || "当前工作区";
  const isTerminalTool = activeTool === "terminal" && isAdmin;
  const isWideModal = !isTerminalTool;
  const activeTitle = isTerminalTool ? "节点终端" : activeTool === "logs" ? "日志查询" : "操作审计";
  const showPanel = open && stage === "panel";

  return (
    <>
      {open && stage === "menu" && (
        <ToolboxDirectoryCard
          subtitle={scopeLabel}
          groups={directoryGroups}
        />
      )}

      {(showPanel || (isAdmin && terminalActivated)) && (
        <>
          {showPanel && isWideModal && (
            <div
              aria-hidden
              onClick={() => setOpen(false)}
              className="fixed inset-0 z-40 bg-[#14161F]/35 backdrop-blur-[2px]"
            />
          )}
          <aside
            role="dialog"
            aria-modal={isWideModal ? true : undefined}
            aria-hidden={!showPanel}
            aria-label={activeTitle}
            className={cx(
              isTerminalTool
                ? "fixed bottom-20 right-6 z-50 flex h-[min(480px,calc(100dvh-8.5rem))] w-[min(620px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white shadow-[0_20px_50px_-16px_rgba(20,22,31,0.22)]"
                : "fixed left-1/2 top-1/2 z-50 flex h-[min(82dvh,820px)] w-[min(1120px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[18px] border border-[#E3E6F2] bg-white shadow-[0_28px_70px_-20px_rgba(20,22,31,0.32)]",
              !showPanel && "hidden",
            )}
          >
            {/* KubeSphere Stage 2 Single-Row Light-Gray Modal Header (46px) */}
            <div className="flex h-[46px] shrink-0 items-center justify-between gap-2 border-b border-[#E3E6F2] bg-[#F8F9FC] px-3">
              <div className="flex min-w-0 items-center gap-2">
                <button
                  type="button"
                  onClick={() => setStage("menu")}
                  className={cx(
                    "inline-flex h-[26px] shrink-0 items-center gap-1 rounded-[7px] border border-[#E3E6F2] bg-white px-2 text-[11px] font-medium text-[#5B6075] transition-colors hover:border-[#CDD3E4] hover:text-[#14161F]",
                    focusRing,
                  )}
                >
                  <ArrowLeft className="h-3 w-3" />
                  <span>工具箱</span>
                </button>
                <span className="h-4 w-px shrink-0 bg-[#E3E6F2]" aria-hidden />
                <div className="flex min-w-0 items-center gap-2 text-[13px] font-semibold text-[#14161F]">
                  <span
                    className={cx(
                      "h-2 w-2 shrink-0 rounded-full",
                      !isTerminalTool
                        ? "bg-[#2B3AE8]"
                        : !selectedNodeReady
                          ? "bg-[#8A90A6]"
                          : termStatus === "connected"
                            ? "bg-[#1F9D55]"
                            : termStatus === "connecting"
                              ? "animate-pulse bg-[#D97706]"
                              : termStatus === "error"
                                ? "bg-[#C83A2B]"
                                : "bg-[#8A90A6]",
                    )}
                    aria-hidden
                  />
                  <span className={cx("truncate", isTerminalTool && "hidden shrink-0 whitespace-nowrap sm:inline")}>
                    {activeTitle}
                  </span>
                  {isTerminalTool ? (
                    <>
                      <Select
                        aria-label="切换管理节点"
                        value={resolvedNodeName}
                        onChange={(e) => setSelectedNode(e.target.value)}
                        disabled={nodes.length === 0}
                        className="!h-[26px] w-auto min-w-[124px] max-w-[180px] shrink-0 rounded-[7px] border-[#E3E6F2] bg-white !py-0 !pl-2.5 !pr-7 font-mono text-[11.5px] font-medium leading-normal text-[#14161F]"
                      >
                        {nodes.length === 0 ? (
                          <option value="">暂无节点</option>
                        ) : (
                          nodes.map((n) => (
                            <option key={n.metadata.name} value={n.metadata.name}>
                              {n.metadata.name}
                              {(n.status?.phase || "") === "Ready" ? "" : " (离线)"}
                            </option>
                          ))
                        )}
                      </Select>
                      {terminalActivated &&
                        selectedNodeReady &&
                        termStatus !== "connected" &&
                        termStatus !== "idle" && (
                          <span
                            className={cx(
                              "shrink-0 text-[11px] font-medium",
                              termStatus === "connecting"
                                ? "text-[#B45309]"
                                : termStatus === "error"
                                  ? "text-[#C83A2B]"
                                  : "text-[#5B6075]",
                            )}
                          >
                            {termStatus === "connecting" ? "连接中" : termStatus === "error" ? "连接异常" : "已断开"}
                          </span>
                        )}
                    </>
                  ) : (
                    <span className="hidden truncate text-[12px] font-medium text-[#5B6075] sm:inline">
                      {scopeLabel}
                    </span>
                  )}
                </div>
              </div>

              <div className="flex shrink-0 items-center gap-1.5">
                {isTerminalTool && (
                  <>
                    {selectedNodeReady && (termStatus === "disconnected" || termStatus === "error") && (
                      <Button
                        variant="secondary"
                        size="sm"
                        icon={<RotateCw className="h-3 w-3" />}
                        onClick={() => terminalRef.current?.reconnect()}
                        className="!h-[26px] !gap-1 !rounded-[7px] !border-[#C6D4FF] !bg-[#EEF1FF] !px-2 !text-[11px] !font-semibold !text-[#2B3AE8] hover:!bg-[#2B3AE8] hover:!text-white"
                      >
                        重新连接
                      </Button>
                    )}
                    <button
                      type="button"
                      title="清空屏幕 (Ctrl+L)"
                      onClick={() => terminalRef.current?.clear()}
                      className={cx(
                        "inline-flex h-[26px] items-center rounded-[7px] border border-[#E3E6F2] bg-white px-2.5 text-[11px] font-medium text-[#14161F] transition-colors hover:bg-[#F2F4FA]",
                        focusRing,
                      )}
                    >
                      清屏
                    </button>
                  </>
                )}
                <button
                  type="button"
                  title="收起工具箱"
                  aria-label="收起工具箱"
                  onClick={() => setOpen(false)}
                  className={cx(
                    "inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-[#5B6075] transition-colors hover:bg-[#E8ECF5] hover:text-[#14161F]",
                    focusRing,
                  )}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>

            {activeTool === "logs" && (
              <ToolboxLogsView
                key={`logs-${logSeed.key}`}
                isPlatformMode={isPlatformMode}
                initialQuery={logSeed.q}
                initialInstance={logSeed.instance}
                onTraceAudit={(reqId) => {
                  setAuditSeed((prev) => ({ key: prev.key + 1, q: reqId }));
                  setActiveTool("audits");
                }}
              />
            )}
            {activeTool === "audits" && (
              <ToolboxAuditsView
                key={`audits-${auditSeed.key}`}
                isPlatformMode={isPlatformMode}
                initialQuery={auditSeed.q}
                onTraceLogs={(reqId) => {
                  setLogSeed((prev) => ({ key: prev.key + 1, q: reqId }));
                  setActiveTool("logs");
                }}
              />
            )}
            {isAdmin && terminalActivated && (
              <div className={cx("min-h-0 flex-1 overflow-hidden bg-[#F7F8FC]", activeTool !== "terminal" && "hidden")}>
                {nodes.length === 0 ? (
                  <div className="flex h-full items-center justify-center p-6 text-xs text-[#8A90A6]">
                    暂无已注册的计算节点
                  </div>
                ) : (
                  <InteractiveTerminal
                    ref={terminalRef}
                    targetScope="node"
                    name={resolvedNodeName}
                    running={selectedNodeReady && Boolean(resolvedNodeName)}
                    activated={terminalActivated}
                    visible={showPanel && isTerminalTool}
                    onStatusChange={setTermStatus}
                  />
                )}
              </div>
            )}
          </aside>
        </>
      )}

      {/* KubeSphere-Style Single Bottom-Right Icon Trigger Button */}
      <div className="fixed bottom-6 right-6 z-40">
        <button
          type="button"
          title={open ? "收起工具箱" : "打开工具箱"}
          aria-label={open ? "收起工具箱" : "打开工具箱"}
          aria-expanded={open}
          onClick={() => {
            if (open) {
              setOpen(false);
            } else {
              setStage("menu");
              setOpen(true);
            }
          }}
          className={cx(
            "inline-flex h-11 w-11 items-center justify-center rounded-full border shadow-[0_16px_38px_-12px_rgba(20,22,31,0.18)] backdrop-blur-md transition-all",
            open
              ? "border-[#2B3AE8] bg-[#2B3AE8] text-white hover:bg-[#2332D6]"
              : "border-[#E3E6F2] bg-white/95 text-[#14161F] hover:bg-[#F2F4FB]",
            focusRing,
          )}
        >
          <Wrench className={cx("h-4.5 w-4.5", open ? "text-white" : "text-[#2B3AE8]")} />
        </button>
      </div>
    </>
  );
}
