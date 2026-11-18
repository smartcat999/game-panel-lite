"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  ArrowLeft,
  ChevronDown,
  ChevronUp,
  Plus,
  RotateCw,
  Star,
  Terminal,
  Trash2,
  Wrench,
  X,
  Zap,
} from "lucide-react";
import {
  ToolboxDirectoryCard,
  type ToolboxDirectoryGroup,
} from "../FloatingToolbox";
import { InteractiveTerminal, type InteractiveTerminalHandle, type TerminalConnectionStatus } from "./InteractiveTerminal";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatBytes, formatRateBytes, parseNumericQuantity, resourceOf } from "../../../console/lib/format";
import { resolveRichApp } from "../../../console/lib/presets";
import type {
  AppConsoleAction,
  AppConsoleParam,
  AppConsoleTarget,
  AppItem,
  AppMetricSpec,
  InstanceCustomMetric,
  InstanceItem,
} from "../../../console/lib/types";
import { Button, IconButton, Input, Meter, Segmented, Select, focusRing } from "../ui";
import { useServerActions, useServerShell } from "./api";
import { CONFIG_OPTIONS_SOURCE_ADDON_MAPS, mergeDynamicFieldOptions, useAddonMapOptions } from "./shared";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export type WorkbenchMode = "metrics" | "commands" | "terminal";
type WindowKey = "5m" | "15m" | "1h";

const isExecOnlyMode = (m: WorkbenchMode) => m === "commands" || m === "terminal";

interface MetricSample {
  timestamp: string;
  cpuMillicores: number;
  memoryBytes: number;
  storageBytes: number;
  networkRxBytesPerSec?: number;
  networkTxBytesPerSec?: number;
  customMetrics?: Record<string, number>;
}

interface WorkbenchPersistedState {
  starredIds?: string[];
  unstarredIds?: string[];
  customActions?: AppConsoleAction[];
}

const WINDOWS: Array<{ value: WindowKey; label: string }> = [
  { value: "5m", label: "5m" },
  { value: "15m", label: "15m" },
  { value: "1h", label: "1h" },
];

const QUICK_VARS: Array<{ name: string; label: string; defaultVal: string; type: "text" | "number" }> = [
  { name: "player", label: "目标玩家", defaultVal: "", type: "text" },
  { name: "item", label: "物品代码", defaultVal: "", type: "text" },
  { name: "count", label: "数量", defaultVal: "1", type: "number" },
  { name: "msg", label: "公告内容", defaultVal: "", type: "text" },
];

function extractTemplateVars(tpl: string): string[] {
  const re = /\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g;
  const out: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(tpl || "")) !== null) {
    if (!out.includes(m[1])) out.push(m[1]);
  }
  return out;
}

function normalizeImageRepo(raw?: string): string {
  const cleaned = (raw || "").trim().toLowerCase().split("@")[0].split("{{")[0];
  const slashIdx = cleaned.lastIndexOf("/");
  const colonIdx = cleaned.indexOf(":", Math.max(0, slashIdx));
  return (colonIdx >= 0 ? cleaned.slice(0, colonIdx) : cleaned).replace(/\/+$/, "");
}

function inferAppId(inst: InstanceItem, catalog: AppItem[]): string | undefined {
  const fromLabel =
    inst.metadata.labels?.["core.gamepanel.io/app"] ||
    inst.metadata.labels?.["gamepanel.io/app"] ||
    inst.metadata.labels?.["gamepanel.io/preset"] ||
    inst.metadata.labels?.["app.gamepanel.io/name"] ||
    inst.metadata.annotations?.["gamepanel.io/app"];
  if (fromLabel) return fromLabel;
  const artifactRepo = inst.spec.serverArtifact?.repository;
  const artifactPkg = inst.spec.serverArtifact?.package;
  if (artifactRepo && artifactPkg) {
    const matched = catalog.find(
      (a) =>
        a.serverBinding?.repository === artifactRepo &&
        a.serverBinding?.package === artifactPkg,
    );
    if (matched?.metadata.name) return matched.metadata.name;
  }
  const instRepo = normalizeImageRepo(inst.spec.image);
  if (instRepo) {
    const matched = catalog.find((a) => {
      const appRepo = normalizeImageRepo(a.image);
      const tplRepo = normalizeImageRepo(a.imageTemplate);
      return (appRepo !== "" && appRepo === instRepo) || (tplRepo !== "" && tplRepo === instRepo);
    });
    if (matched?.metadata.name) return matched.metadata.name;
  }
  return undefined;
}

function formatClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatCores(millicores: number): string {
  const cores = Math.max(0, millicores) / 1000;
  return `${cores.toFixed(2).replace(/\.?0+$/, "")} 核`;
}

function windowDurationMs(w: WindowKey): number {
  if (w === "5m") return 5 * 60_000;
  if (w === "1h") return 60 * 60_000;
  return 15 * 60_000;
}

function encodeBase64Utf8(str: string): string {
  try {
    return typeof window !== "undefined"
      ? window.btoa(unescape(encodeURIComponent(str)))
      : Buffer.from(str, "utf-8").toString("base64");
  } catch {
    return "";
  }
}

function MetricAreaChart({
  id,
  samples,
  limit,
  color,
  valueOf,
  formatValue,
  formatSample,
}: {
  id: string;
  samples: MetricSample[];
  limit: number;
  color: string;
  valueOf: (s: MetricSample) => number;
  formatValue: (v: number) => string;
  formatSample?: (s: MetricSample) => string;
}) {
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  const valueOfRef = useRef(valueOf);
  valueOfRef.current = valueOf;
  const width = 456;
  const height = 72;
  const padY = 6;

  const points = useMemo(() => {
    if (!samples.length) return [];
    const maxRenderPoints = 120;
    let displaySamples = samples;
    if (samples.length > maxRenderPoints) {
      const step = (samples.length - 1) / (maxRenderPoints - 1);
      displaySamples = [];
      for (let i = 0; i < maxRenderPoints - 1; i++) {
        displaySamples.push(samples[Math.round(i * step)]);
      }
      displaySamples.push(samples[samples.length - 1]);
    }
    const readVal = valueOfRef.current;
    const values = displaySamples.map((s) => Math.max(0, readVal(s)));
    const maxVal = Math.max(limit > 0 ? limit : 0, ...values, 1);
    return displaySamples.map((s, i) => {
      const x = displaySamples.length === 1 ? width / 2 : (i / (displaySamples.length - 1)) * width;
      const v = readVal(s);
      const ratio = Math.min(1, Math.max(0, v / maxVal));
      const y = height - padY - ratio * (height - padY * 2);
      return { x, y, sample: s, value: v };
    });
  }, [samples, limit]);

  if (!points.length) {
    return (
      <div className="flex h-[72px] items-center justify-center rounded-xl bg-[#F7F8FC] text-xs text-[#8A90A6]">
        暂无采样数据
      </div>
    );
  }

  const linePath =
    points.length === 1
      ? `M 0 ${points[0].y.toFixed(1)} L ${width} ${points[0].y.toFixed(1)}`
      : points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(" ");

  const areaPath =
    points.length === 1
      ? `M 0 ${points[0].y.toFixed(1)} L ${width} ${points[0].y.toFixed(1)} L ${width} ${height} L 0 ${height} Z`
      : `${linePath} L ${points[points.length - 1].x.toFixed(1)} ${height} L ${points[0].x.toFixed(1)} ${height} Z`;

  const active = hoverIdx !== null && points[hoverIdx] ? points[hoverIdx] : null;

  return (
    <div
      className="relative mt-2 select-none"
      onMouseLeave={() => setHoverIdx(null)}
      onMouseMove={(e) => {
        const rect = e.currentTarget.getBoundingClientRect();
        if (rect.width <= 0) return;
        const relX = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
        const idx = Math.round(relX * (points.length - 1));
        setHoverIdx(idx);
      }}
    >
      <svg viewBox={`0 0 ${width} ${height}`} className="h-[72px] w-full overflow-visible">
        <defs>
          <linearGradient id={`grad-${id}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.22" />
            <stop offset="100%" stopColor={color} stopOpacity="0.0" />
          </linearGradient>
        </defs>
        <line x1="0" y1={height * 0.25} x2={width} y2={height * 0.25} stroke="#EEF0F7" strokeDasharray="3 3" />
        <line x1="0" y1={height * 0.5} x2={width} y2={height * 0.5} stroke="#EEF0F7" strokeDasharray="3 3" />
        <line x1="0" y1={height * 0.75} x2={width} y2={height * 0.75} stroke="#EEF0F7" strokeDasharray="3 3" />
        <path d={areaPath} fill={`url(#grad-${id})`} />
        <path d={linePath} fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        {active && (
          <>
            <line
              x1={active.x}
              y1={0}
              x2={active.x}
              y2={height}
              stroke="#B8C0E6"
              strokeWidth="1"
              strokeDasharray="2 2"
            />
            <circle cx={active.x} cy={active.y} r="3.5" fill="#FFFFFF" stroke={color} strokeWidth="2" />
          </>
        )}
      </svg>
      {active && (
        <div className="pointer-events-none absolute -top-1 right-0 flex items-center gap-1.5 rounded-md border border-[#E3E6F2] bg-white/95 px-2 py-0.5 text-[11px] font-medium text-[#14161F] shadow-sm tabular-nums">
          <span className="font-mono text-[#8A90A6]">{formatClock(active.sample.timestamp)}</span>
          <span className="font-semibold" style={{ color }}>
            {formatSample ? formatSample(active.sample) : formatValue(active.value)}
          </span>
        </div>
      )}
    </div>
  );
}

export function FloatingTerminal({
  inst,
  app,
  name,
  running,
  canExec = true,
  open,
  onOpenChange,
  mode: controlledMode,
  onModeChange,
}: {
  inst: InstanceItem;
  app?: AppItem;
  name: string;
  running: boolean;
  canExec?: boolean;
  open: boolean;
  onOpenChange: (next: boolean) => void;
  mode?: WorkbenchMode;
  onModeChange?: (next: WorkbenchMode) => void;
}) {
  const { tenant, apiFetch, apps } = useConsole();
  const { sendGameCommand } = useServerShell(name);
  const { updateSpec } = useServerActions();

  const [internalMode, setInternalMode] = useState<WorkbenchMode>(canExec ? "commands" : "metrics");
  const rawMode: WorkbenchMode = controlledMode ?? internalMode;
  const activeMode: WorkbenchMode = !canExec && isExecOnlyMode(rawMode) ? "metrics" : rawMode;
  const setMode = useCallback(
    (m: WorkbenchMode) => {
      const next = !canExec && isExecOnlyMode(m) ? "metrics" : m;
      setInternalMode(next);
      onModeChange?.(next);
    },
    [canExec, onModeChange],
  );

  // Resolve App & Declarative Console Actions
  const richApp = useMemo(() => {
    const baseCatalog = apps;
    const catalog =
      app && !baseCatalog.some((a) => a.metadata.name === app.metadata.name) ? [app, ...baseCatalog] : baseCatalog;
    const id = app?.metadata.name || inferAppId(inst, catalog);
    return resolveRichApp(id, catalog);
  }, [app, inst, apps]);

  const consoleDelivery = richApp?.consoleDelivery || "stdin";
  const builtInActions = useMemo<AppConsoleAction[]>(() => richApp?.consoleActions || [], [richApp]);
  const consoleTargets = useMemo<AppConsoleTarget[]>(() => richApp?.consoleTargets || [], [richApp]);
  const appMetrics = useMemo<AppMetricSpec[]>(
    () => inst.spec?.metrics || richApp?.metrics || [],
    [inst.spec?.metrics, richApp],
  );

  const appConfig = useMemo<Record<string, string>>(() => {
    const raw =
      inst.metadata.annotations?.["core.gamepanel.io/app-config"] ||
      inst.metadata.annotations?.["gamepanel.io/app-config"];
    if (!raw) return {};
    try {
      const parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? (parsed as Record<string, string>) : {};
    } catch {
      return {};
    }
  }, [inst.metadata.annotations]);

  const [activeTargetId, setActiveTargetId] = useState<string>("master");
  useEffect(() => {
    if (consoleTargets.length > 0 && !consoleTargets.some((t) => t.id === activeTargetId)) {
      setActiveTargetId(consoleTargets[0].id);
    }
  }, [consoleTargets, activeTargetId]);

  const activeTarget = useMemo<AppConsoleTarget | undefined>(
    () => consoleTargets.find((t) => t.id === activeTargetId) || consoleTargets[0],
    [consoleTargets, activeTargetId],
  );

  // Unified Custom Game Metrics (returned by /metrics API and InstanceStatus.Usage.CustomMetrics)
  const [apiCustomMetrics, setApiCustomMetrics] = useState<InstanceCustomMetric[]>(
    () => inst.status?.usage?.customMetrics || [],
  );
  const [metricsSubView, setMetricsSubView] = useState<"resources" | "game" | "items">("resources");
  const [itemSearch, setItemSearch] = useState("");

  useEffect(() => {
    if (inst.status?.usage?.customMetrics?.length) {
      setApiCustomMetrics(inst.status.usage.customMetrics);
    }
  }, [inst.status?.usage?.customMetrics]);

  const evaluatedMetrics = useMemo<InstanceCustomMetric[]>(() => {
    if (apiCustomMetrics.length > 0) {
      return apiCustomMetrics.map((cm) => {
        const specDef = appMetrics.find((m) => m.id === cm.id);
        let maxVal = cm.max || specDef?.maxValue;
        if (!maxVal && specDef?.maxConfigKey) {
          maxVal =
            appConfig[specDef.maxConfigKey] ||
            richApp?.configSchema?.find((f) => f.name === specDef.maxConfigKey)?.default;
        }
        return {
          ...cm,
          display: running ? cm.display || String(cm.value ?? 0) : "0",
          value: running ? cm.value : 0,
          max: maxVal,
          items: running ? cm.items || [] : [],
          bindParam: cm.bindParam || specDef?.bindParam || "player",
          quickActionIds: cm.quickActionIds?.length ? cm.quickActionIds : specDef?.quickActionIds,
          itemActions: cm.itemActions?.length ? cm.itemActions : specDef?.itemActions,
        };
      });
    }
    return appMetrics.map((m) => {
      let maxVal = m.maxValue;
      if (!maxVal && m.maxConfigKey) {
        maxVal =
          appConfig[m.maxConfigKey] ||
          richApp?.configSchema?.find((f) => f.name === m.maxConfigKey)?.default;
      }
      return {
        id: m.id,
        label: m.label,
        unit: m.unit,
        value: 0,
        display: "0",
        max: maxVal,
        items: [],
        bindParam: m.bindParam || "player",
        quickActionIds: m.quickActionIds,
        itemActions: m.itemActions,
      };
    });
  }, [apiCustomMetrics, appMetrics, appConfig, richApp, running]);

  const listBearingMetric = useMemo<InstanceCustomMetric | undefined>(() => {
    return evaluatedMetrics.find((cm) => {
      const specDef = appMetrics.find((m) => m.id === cm.id);
      return (
        (cm.items && cm.items.length > 0) ||
        (cm.itemActions && cm.itemActions.length > 0) ||
        Boolean(specDef?.itemPattern) ||
        specDef?.mode === "regex_count" ||
        specDef?.mode === "log_delta" ||
        cm.id === "online_players"
      );
    });
  }, [evaluatedMetrics, appMetrics]);

  // Persisted Starred + Custom Commands (Instance Annotation + localStorage mirror)
  const storageKey = `gp-console-wb-${tenant}-${name}`;
  const [persisted, setPersisted] = useState<WorkbenchPersistedState>(() => {
    const rawAnno = inst.metadata.annotations?.["gamepanel.io/console-workbench"];
    if (rawAnno) {
      try {
        return JSON.parse(rawAnno) as WorkbenchPersistedState;
      } catch {
        // fallback
      }
    }
    if (typeof window !== "undefined") {
      const local = window.localStorage.getItem(storageKey);
      if (local) {
        try {
          return JSON.parse(local) as WorkbenchPersistedState;
        } catch {
          // ignore
        }
      }
    }
    return {};
  });

  const savePersisted = useCallback(
    (next: WorkbenchPersistedState) => {
      setPersisted(next);
      if (typeof window !== "undefined") {
        window.localStorage.setItem(storageKey, JSON.stringify(next));
      }
      void updateSpec(inst, {}, "", {
        "gamepanel.io/console-workbench": JSON.stringify(next),
      });
    },
    [inst, storageKey, updateSpec],
  );

  const rawMergedActions = useMemo<AppConsoleAction[]>(() => {
    const starredSet = new Set(persisted.starredIds || []);
    const unstarredSet = new Set(persisted.unstarredIds || []);
    const customs = (persisted.customActions || []).map((a) => ({
      ...a,
      starred: unstarredSet.has(a.id) ? false : starredSet.has(a.id) ? true : (a.starred ?? true),
    }));
    const builtins = builtInActions.map((a) => ({
      ...a,
      starred: unstarredSet.has(a.id) ? false : starredSet.has(a.id) ? true : Boolean(a.starred),
    }));
    return [...customs, ...builtins];
  }, [builtInActions, persisted]);

  const needsAddonMaps = useMemo(
    () =>
      rawMergedActions.some((a) =>
        a.params?.some((p) => p.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS),
      ),
    [rawMergedActions],
  );
  const addonMapOptions = useAddonMapOptions(inst.spec.addons, needsAddonMaps);

  const mergedActions = useMemo<AppConsoleAction[]>(() => {
    if (addonMapOptions.length === 0) return rawMergedActions;
    return rawMergedActions.map((act) => {
      if (!act.params?.some((p) => p.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS)) {
        return act;
      }
      return {
        ...act,
        params: act.params.map((p) =>
          p.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS
            ? { ...p, options: mergeDynamicFieldOptions(p.options, addonMapOptions, p.defaultValue) }
            : p,
        ),
      };
    });
  }, [addonMapOptions, rawMergedActions]);

  // Command Workbench UI state
  const [cmdSearch, setCmdSearch] = useState("");
  const [activeCategory, setActiveCategory] = useState<string>("all");
  const [expandedActionId, setExpandedActionId] = useState<string | null>(null);
  const [singleInputs, setSingleInputs] = useState<Record<string, string>>({});
  const [multiInputs, setMultiInputs] = useState<Record<string, Record<string, string>>>({});
  const [rawGameCmd, setRawGameCmd] = useState("");
  const [sendingCmd, setSendingCmd] = useState(false);
  const [sentBadge, setSentBadge] = useState<{ text: string; ok: boolean } | null>(null);
  const sentTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Custom Command Builder Sub-View state
  const [builderOpen, setBuilderOpen] = useState(false);
  const [customTitle, setCustomTitle] = useState("");
  const [customGroup, setCustomGroup] = useState("常用指令");
  const [customTemplate, setCustomTemplate] = useState("");
  const [customParamMeta, setCustomParamMeta] = useState<Record<string, { label: string; defaultValue: string }>>({
    player: { label: "目标玩家", defaultValue: "" },
    item: { label: "物品代码", defaultValue: "" },
    count: { label: "数量", defaultValue: "1" },
    msg: { label: "公告内容", defaultValue: "" },
  });
  const customInputRef = useRef<HTMLInputElement>(null);
  const cursorPosRef = useRef<{ start: number; end: number } | null>(null);

  const categories = useMemo(() => {
    const groups: string[] = [];
    for (const a of mergedActions) {
      const g = (a.group || "常用指令").trim();
      if (g && !groups.includes(g)) groups.push(g);
    }
    return groups;
  }, [mergedActions]);

  const filteredActions = useMemo(() => {
    const q = cmdSearch.trim().toLowerCase();
    return mergedActions
      .filter((a) => {
        if (activeCategory === "fav" && !a.starred) return false;
        if (activeCategory !== "all" && activeCategory !== "fav" && (a.group || "常用指令") !== activeCategory) {
          return false;
        }
        if (q && !a.title.toLowerCase().includes(q) && !a.command.toLowerCase().includes(q)) {
          return false;
        }
        return true;
      })
      .sort((a, b) => Number(Boolean(b.starred)) - Number(Boolean(a.starred)));
  }, [mergedActions, activeCategory, cmdSearch]);

  const triggerSendGameCommand = useCallback(
    async (resolvedCmd: string) => {
      const trimmed = resolvedCmd.trim();
      if (!trimmed || !running || sendingCmd) return;
      setSendingCmd(true);
      let payload = trimmed;
      if (activeTarget?.format) {
        payload = activeTarget.format
          .replace(/\{\{\s*command\s*\}\}/g, trimmed)
          .replace(/\{\{\s*command_base64\s*\}\}/g, encodeBase64Utf8(trimmed));
      }
      const res = await sendGameCommand(payload, consoleDelivery);
      setSendingCmd(false);
      if (sentTimerRef.current) clearTimeout(sentTimerRef.current);
      const targetPrefix = consoleTargets.length > 1 && activeTarget ? `[${activeTarget.label}] ` : "";
      setSentBadge({
        text: res.ok ? `✓ 已发送 ${targetPrefix}${trimmed}` : `发送失败: ${res.output}`,
        ok: res.ok,
      });
      sentTimerRef.current = setTimeout(() => setSentBadge(null), 3000);
    },
    [running, sendingCmd, activeTarget, consoleTargets.length, sendGameCommand, consoleDelivery],
  );

  const toggleStarAction = useCallback(
    (action: AppConsoleAction) => {
      const starredSet = new Set(persisted.starredIds || []);
      const unstarredSet = new Set(persisted.unstarredIds || []);
      if (action.starred) {
        starredSet.delete(action.id);
        unstarredSet.add(action.id);
      } else {
        unstarredSet.delete(action.id);
        starredSet.add(action.id);
      }
      savePersisted({
        ...persisted,
        starredIds: Array.from(starredSet),
        unstarredIds: Array.from(unstarredSet),
      });
    },
    [persisted, savePersisted],
  );

  const deleteCustomAction = useCallback(
    (id: string) => {
      const nextCustoms = (persisted.customActions || []).filter((a) => a.id !== id);
      savePersisted({
        ...persisted,
        customActions: nextCustoms,
      });
    },
    [persisted, savePersisted],
  );

  const insertBuilderVar = (varName: string) => {
    const input = customInputRef.current;
    const token = `{{${varName}}}`;
    const current = customTemplate;
    const start =
      input && typeof input.selectionStart === "number"
        ? input.selectionStart
        : (cursorPosRef.current?.start ?? current.length);
    const end =
      input && typeof input.selectionEnd === "number" ? input.selectionEnd : (cursorPosRef.current?.end ?? start);
    const nextVal = current.slice(0, start) + token + current.slice(end);
    setCustomTemplate(nextVal);
    const nextPos = start + token.length;
    cursorPosRef.current = { start: nextPos, end: nextPos };
    setTimeout(() => {
      if (customInputRef.current) {
        customInputRef.current.focus();
        customInputRef.current.setSelectionRange(nextPos, nextPos);
      }
    }, 0);
  };

  const detectedCustomVars = useMemo(() => extractTemplateVars(customTemplate), [customTemplate]);

  const handleSaveCustomCommand = () => {
    const title = customTitle.trim();
    const cmd = customTemplate.trim();
    if (!title || !cmd) return;
    const params: AppConsoleParam[] = detectedCustomVars.map((v) => {
      const meta = customParamMeta[v] || { label: v, defaultValue: "" };
      return {
        name: v,
        label: meta.label.trim() || v,
        type: v === "count" ? "number" : "text",
        defaultValue: meta.defaultValue,
      };
    });
    const newAction: AppConsoleAction = {
      id: `custom-${Date.now()}`,
      title,
      group: customGroup.trim() || "常用指令",
      command: cmd,
      starred: true,
      params,
    };
    savePersisted({
      ...persisted,
      customActions: [newAction, ...(persisted.customActions || [])],
    });
    setCustomTitle("");
    setCustomTemplate("");
    setBuilderOpen(false);
  };

  // Metrics & Terminal state
  const [windowKey, setWindowKey] = useState<WindowKey>("15m");
  const [samples, setSamples] = useState<MetricSample[]>([]);
  const [terminalActivated, setTerminalActivated] = useState(false);
  const [termStatus, setTermStatus] = useState<TerminalConnectionStatus>("idle");
  const terminalRef = useRef<InteractiveTerminalHandle>(null);

  const fetchMetrics = useCallback(async () => {
    if (!tenant || !name) return;
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/instances/${encodeURIComponent(name)}/metrics?window=${windowKey}`,
      );
      if (!res.ok) return;
      const body = await res.json();
      const list: MetricSample[] = body?.samples || [];
      if (Array.isArray(list)) {
        setSamples((prev) => {
          if (list.length >= 2 || list.length === 0) return list;
          const cutoff = Date.now() - windowDurationMs(windowKey);
          const kept = prev.filter((s) => new Date(s.timestamp).getTime() >= cutoff);
          if (!kept.length) return list;
          const incoming = list[0];
          const last = kept[kept.length - 1];
          if (new Date(incoming.timestamp).getTime() - new Date(last.timestamp).getTime() >= 4000) {
            return [...kept.slice(-239), incoming];
          }
          return [...kept.slice(0, -1), incoming];
        });
      }
      if (Array.isArray(body?.customMetrics)) {
        setApiCustomMetrics(body.customMetrics as InstanceCustomMetric[]);
      }
    } catch {
      // ignore transient poll error
    }
  }, [apiFetch, tenant, name, windowKey]);

  useEffect(() => {
    if (!open || activeMode !== "metrics") return;
    void fetchMetrics();
    const timer = setInterval(() => void fetchMetrics(), 5000);
    return () => clearInterval(timer);
  }, [open, activeMode, fetchMetrics]);

  useEffect(() => {
    const u = inst.status?.usage;
    if (!u) return;
    const nowIso = new Date().toISOString();
    const customMap: Record<string, number> = {};
    for (const cm of u.customMetrics || []) {
      customMap[cm.id] = Number(cm.value) || 0;
    }
    setSamples((prev) => {
      const nextSample: MetricSample = {
        timestamp: nowIso,
        cpuMillicores: u.cpuMillicores || 0,
        memoryBytes: u.memoryBytes || 0,
        storageBytes: u.storageBytes || 0,
        networkRxBytesPerSec: u.networkRxBytesPerSec || 0,
        networkTxBytesPerSec: u.networkTxBytesPerSec || 0,
        customMetrics: Object.keys(customMap).length > 0 ? customMap : undefined,
      };
      if (!prev.length) return [nextSample];
      const last = prev[prev.length - 1];
      const sameCustom =
        JSON.stringify(last.customMetrics || {}) === JSON.stringify(nextSample.customMetrics || {});
      if (
        last.cpuMillicores === nextSample.cpuMillicores &&
        last.memoryBytes === nextSample.memoryBytes &&
        (last.networkRxBytesPerSec || 0) === (nextSample.networkRxBytesPerSec || 0) &&
        (last.networkTxBytesPerSec || 0) === (nextSample.networkTxBytesPerSec || 0) &&
        sameCustom &&
        Date.now() - new Date(last.timestamp).getTime() < 4000
      ) {
        return prev;
      }
      return [...prev.slice(-239), nextSample];
    });
  }, [inst.status?.usage]);

  const [stage, setStage] = useState<"menu" | "panel">("menu");
  const openedByFabRef = useRef(false);

  useEffect(() => {
    if (!open) return;
    if (openedByFabRef.current) {
      openedByFabRef.current = false;
      setStage("menu");
    } else {
      setStage("panel");
    }
  }, [open]);

  // Close toolbox on Escape when browsing directory card or outside interactive terminal
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && open && (stage === "menu" || activeMode !== "terminal")) {
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, stage, activeMode, onOpenChange]);

  useEffect(() => {
    if (!canExec || !open || stage !== "panel" || activeMode !== "terminal") return;
    setTerminalActivated(true);
    const id = setTimeout(() => {
      terminalRef.current?.fit();
      terminalRef.current?.focus();
    }, 20);
    return () => clearTimeout(id);
  }, [canExec, open, stage, activeMode]);

  const cpuLimitMilli = Math.round(parseNumericQuantity(resourceOf(inst, "cpu")) * 1000) || 1000;
  const memLimitBytes = Math.round(parseNumericQuantity(resourceOf(inst, "memory")) * 1024 ** 3) || 1024 ** 3;
  const diskLimitBytes = Math.round(parseNumericQuantity(resourceOf(inst, "storage")) * 1024 ** 3) || 10 * 1024 ** 3;

  const latestSample = samples[samples.length - 1];
  const curCpuMilli = running ? (inst.status?.usage?.cpuMillicores ?? latestSample?.cpuMillicores ?? 0) : 0;
  const curMemBytes = running ? (inst.status?.usage?.memoryBytes ?? latestSample?.memoryBytes ?? 0) : 0;
  const curDiskBytes = inst.status?.usage?.storageBytes ?? latestSample?.storageBytes ?? 0;
  const curNetRx = running ? (inst.status?.usage?.networkRxBytesPerSec ?? latestSample?.networkRxBytesPerSec ?? 0) : 0;
  const curNetTx = running ? (inst.status?.usage?.networkTxBytesPerSec ?? latestSample?.networkTxBytesPerSec ?? 0) : 0;

  const netRxPeak = useMemo(
    () => (samples.length ? Math.max(...samples.map((s) => s.networkRxBytesPerSec || 0), curNetRx) : curNetRx),
    [samples, curNetRx],
  );
  const netTxPeak = useMemo(
    () => (samples.length ? Math.max(...samples.map((s) => s.networkTxBytesPerSec || 0), curNetTx) : curNetTx),
    [samples, curNetTx],
  );
  const netScaleMax = useMemo(
    () =>
      Math.max(
        1024,
        curNetRx + curNetTx,
        ...samples.map((s) => (s.networkRxBytesPerSec || 0) + (s.networkTxBytesPerSec || 0)),
      ),
    [samples, curNetRx, curNetTx],
  );

  const cpuPeakMilli = useMemo(
    () => (samples.length ? Math.max(...samples.map((s) => s.cpuMillicores || 0)) : curCpuMilli),
    [samples, curCpuMilli],
  );
  const cpuAvgMilli = useMemo(
    () =>
      samples.length
        ? Math.round(samples.reduce((acc, s) => acc + (s.cpuMillicores || 0), 0) / samples.length)
        : curCpuMilli,
    [samples, curCpuMilli],
  );
  const memPeakBytes = useMemo(
    () => (samples.length ? Math.max(...samples.map((s) => s.memoryBytes || 0)) : curMemBytes),
    [samples, curMemBytes],
  );
  const memAvgBytes = useMemo(
    () =>
      samples.length
        ? Math.round(samples.reduce((acc, s) => acc + (s.memoryBytes || 0), 0) / samples.length)
        : curMemBytes,
    [samples, curMemBytes],
  );
  const cpuPct = cpuLimitMilli > 0 ? Math.min(100, Math.round((curCpuMilli / cpuLimitMilli) * 100)) : 0;
  const memPct = memLimitBytes > 0 ? Math.min(100, Math.round((curMemBytes / memLimitBytes) * 100)) : 0;
  const diskPct = diskLimitBytes > 0 ? Math.min(100, Math.round((curDiskBytes / diskLimitBytes) * 100)) : 0;

  const directoryGroups: ToolboxDirectoryGroup[] = useMemo(() => {
    const openTool = (tool: WorkbenchMode) => {
      setBuilderOpen(false);
      setMode(tool);
      setStage("panel");
    };
    const serverItems: ToolboxDirectoryGroup["items"] = [
      {
        key: "metrics",
        icon: <Activity className="h-4 w-4" />,
        title: "实时监控",
        active: stage === "panel" && activeMode === "metrics",
        onSelect: () => openTool("metrics"),
      },
    ];
    if (canExec) {
      serverItems.push(
        {
          key: "commands",
          icon: <Zap className="h-4 w-4" />,
          title: "快捷指令",
          active: stage === "panel" && activeMode === "commands",
          onSelect: () => openTool("commands"),
        },
        {
          key: "terminal",
          icon: <Terminal className="h-4 w-4" />,
          title: "Web 终端",
          active: stage === "panel" && activeMode === "terminal",
          onSelect: () => openTool("terminal"),
        },
      );
    }
    return [
      {
        title: "实例工具",
        badge: (
          <span className={cx("font-semibold", running ? "text-[#1F9D55]" : "text-[#8C93A8]")}>
            {running ? "● 运行中" : "● 未运行"}
          </span>
        ),
        items: serverItems,
      },
    ];
  }, [canExec, running, stage, activeMode, setMode]);

  const resolveMultiActionCommand = (action: AppConsoleAction) => {
    let out = action.command;
    const values = multiInputs[action.id] || {};
    for (const p of action.params || []) {
      const val = values[p.name] ?? p.defaultValue ?? p.options?.[0]?.value ?? "";
      out = out.replace(new RegExp(`\\{\\{\\s*${p.name}\\s*\\}\\}`, "g"), val);
    }
    return out;
  };

  const activeToolTitle =
    activeMode === "metrics"
      ? "实时监控"
      : activeMode === "commands"
        ? "快捷指令"
        : "Web 终端";

  return (
    <>
      {/* KubeSphere Stage 1: Grouped Directory Card */}
      {open && stage === "menu" && (
        <ToolboxDirectoryCard
          subtitle={name}
          groups={directoryGroups}
        />
      )}

      {/* KubeSphere Stage 2: Tool Panel (500px Docked, keeping terminal mounted) */}
      {(open || terminalActivated) && (
        <aside
          role="dialog"
          aria-hidden={!open || stage !== "panel"}
          aria-label={activeToolTitle}
          className={cx(
            "fixed bottom-20 right-6 z-50 flex h-[min(460px,calc(100dvh-8.5rem))] w-[min(500px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white shadow-[0_20px_50px_-16px_rgba(20,22,31,0.22)]",
            (!open || stage !== "panel") && "hidden",
          )}
        >
          {/* KubeSphere Stage 2 Single-Row Light-Gray Modal Header (46px) */}
          <div className="flex h-[46px] shrink-0 items-center justify-between gap-2 border-b border-[#E3E6F2] bg-[#F8F9FC] px-3">
            <div className="flex min-w-0 items-center gap-2">
              {activeMode === "commands" && builderOpen ? (
                <>
                  <button
                    type="button"
                    onClick={() => setBuilderOpen(false)}
                    className={cx(
                      "inline-flex h-[26px] shrink-0 items-center gap-1 rounded-[7px] border border-[#E3E6F2] bg-white px-2 text-[11px] font-medium text-[#5B6075] transition-colors hover:border-[#CDD3E4] hover:text-[#14161F]",
                      focusRing,
                    )}
                  >
                    <ArrowLeft className="h-3 w-3" />
                    <span>返回指令</span>
                  </button>
                  <span className="h-4 w-px shrink-0 bg-[#E3E6F2]" aria-hidden />
                  <span className="truncate text-[13px] font-semibold text-[#14161F]">添加自定义指令</span>
                </>
              ) : (
                <>
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
                  <span
                    className={cx(
                      "h-2 w-2 shrink-0 rounded-full",
                      !running
                        ? "bg-[#8A90A6]"
                        : activeMode === "terminal" && terminalActivated
                          ? termStatus === "connected"
                            ? "bg-[#1F9D55]"
                            : termStatus === "connecting"
                              ? "animate-pulse bg-[#D97706]"
                              : termStatus === "error"
                                ? "bg-[#C83A2B]"
                                : "bg-[#8A90A6]"
                          : "bg-[#1F9D55]",
                    )}
                    aria-hidden
                  />
                  <span className="truncate text-[13px] font-semibold text-[#14161F]">{activeToolTitle}</span>
                  {activeMode === "commands" && sentBadge && (
                    <span
                      className={cx(
                        "max-w-[100px] truncate rounded-full px-2 py-0.5 font-mono text-[11px] font-medium",
                        sentBadge.ok ? "bg-[#E7F6EC] text-[#1F7A45]" : "bg-[#FDECEC] text-[#C83A2B]",
                      )}
                    >
                      {sentBadge.text}
                    </span>
                  )}
                  {activeMode === "terminal" &&
                    terminalActivated &&
                    running &&
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
              )}
            </div>

            <div className="flex shrink-0 items-center gap-1.5">
              {activeMode === "metrics" && (
                <Segmented size="sm" label="监控时间窗口" value={windowKey} onChange={setWindowKey} items={WINDOWS} />
              )}

              {activeMode === "commands" && !builderOpen && (
                <>
                  <Input
                    type="search"
                    aria-label="搜索游戏指令"
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    value={cmdSearch}
                    onChange={(e) => setCmdSearch(e.target.value)}
                    placeholder="搜索指令..."
                    className="h-[26px] w-28 rounded-[7px] border-[#E3E6F2] bg-white px-2 text-xs [&::-webkit-search-cancel-button]:hidden"
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    icon={<Plus className="h-3 w-3" />}
                    onClick={() => {
                      if (categories.length && !categories.includes(customGroup)) {
                        setCustomGroup(categories[0]);
                      }
                      setBuilderOpen(true);
                    }}
                    className="!h-[26px] !gap-1 !rounded-[7px] !border-[#E3E6F2] !bg-white !px-2 !text-[11px] !font-medium !text-[#14161F] hover:!bg-[#EEF1FA]"
                  >
                    自定义指令
                  </Button>
                </>
              )}

              {activeMode === "terminal" && (
                <>
                  {running && (termStatus === "disconnected" || termStatus === "error") && (
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
                onClick={() => onOpenChange(false)}
                className={cx(
                  "inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-[#5B6075] transition-colors hover:bg-[#E8ECF5] hover:text-[#14161F]",
                  focusRing,
                )}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          {activeMode === "metrics" && (
            <div className="flex flex-1 flex-col overflow-hidden bg-[#F7F8FC]">
              {/* Sub-View Switcher Bar inside Instance Monitoring (系统资源 | 游戏指标 | 在线玩家) */}
              {evaluatedMetrics.length > 0 && (
                <div className="flex shrink-0 items-center justify-between gap-2 border-b border-[#E3E6F2] bg-[#F7F8FC] px-3.5 py-2">
                  <Segmented
                    size="sm"
                    label="实例监控视图"
                    value={metricsSubView}
                    onChange={setMetricsSubView}
                    items={[
                      { value: "resources", label: "系统资源" },
                      { value: "game", label: "游戏指标" },
                      ...(listBearingMetric
                        ? [
                            {
                              value: "items" as const,
                              label: listBearingMetric.label,
                            },
                          ]
                        : []),
                    ]}
                  />

                  {metricsSubView === "items" && (
                    <Input
                      type="search"
                      aria-label="搜索监控条目"
                      autoCorrect="off"
                      autoCapitalize="none"
                      spellCheck={false}
                      value={itemSearch}
                      onChange={(e) => setItemSearch(e.target.value)}
                      placeholder={`搜索${listBearingMetric?.label.replace(/^在线/, "") || "条目"}...`}
                      className="h-7 w-32 rounded-full border-[#E3E6F2] px-2.5 text-xs sm:w-40 [&::-webkit-search-cancel-button]:hidden"
                    />
                  )}
                </div>
              )}

              {metricsSubView === "game" && evaluatedMetrics.length > 0 ? (
                /* Sub-View 2: Adaptive Multi-Metric Bento Grid (full-width for 1-2 metrics, 2-col grid for 3+ metrics) */
                <div className="flex-1 overflow-y-auto p-3.5">
                  <div
                    className={cx(
                      "grid grid-cols-1 gap-2.5",
                      evaluatedMetrics.length >= 3 && "sm:grid-cols-2",
                    )}
                  >
                    {evaluatedMetrics.map((m, idx) => {
                      const limitNum = m.max ? Number(m.max) || 0 : 0;
                      const color = idx % 2 === 0 ? "#2B3AE8" : "#1F9D55";
                      const seriesVals = samples.map((s) => s.customMetrics?.[m.id] ?? m.value ?? 0);
                      const peakVal = seriesVals.length ? Math.max(...seriesVals, m.value || 0) : m.value || 0;
                      const avgVal = seriesVals.length
                        ? seriesVals.reduce((acc, v) => acc + v, 0) / seriesVals.length
                        : m.value || 0;
                      const fmtStat = (v: number) =>
                        Number.isInteger(v) ? String(v) : String(Number(v.toFixed(1)));

                      return (
                        <div key={m.id} className="rounded-2xl border border-[#E3E6F2] bg-white p-3">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div className="flex items-baseline gap-2">
                              <span className="text-xs font-medium text-[#5B6075]">{m.label}</span>
                              <span className="font-mono text-sm font-semibold text-[#14161F]">{m.display}</span>
                              {m.max ? (
                                <span className="font-mono text-[11px] text-[#8A90A6]">
                                  / {m.max} {m.unit || ""}
                                </span>
                              ) : m.unit ? (
                                <span className="text-[11px] text-[#8A90A6]">{m.unit}</span>
                              ) : null}
                            </div>
                            <div className="flex items-center gap-1.5 text-[11px] text-[#5B6075]">
                              <span>
                                均值 {fmtStat(avgVal)}
                                {m.unit ? ` ${m.unit}` : ""}
                              </span>
                              <span className="text-[#C4C8D8]">·</span>
                              <span>
                                峰值 {fmtStat(peakVal)}
                                {m.unit ? ` ${m.unit}` : ""}
                              </span>
                              {listBearingMetric?.id === m.id && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => setMetricsSubView("items")}
                                  className="ml-0.5 !h-6 !bg-[#EEF1FF] !px-2 !text-[10.5px] !font-semibold !text-[#2B3AE8] hover:!bg-[#E0E6FF]"
                                >
                                  查看列表 ▸
                                </Button>
                              )}
                            </div>
                          </div>
                          <MetricAreaChart
                            id={`custom-${m.id}`}
                            samples={samples}
                            limit={limitNum}
                            color={color}
                            valueOf={(s) => s.customMetrics?.[m.id] ?? m.value ?? 0}
                            formatValue={(v) => `${Number(v.toFixed(1))} ${m.unit || ""}`.trim()}
                          />
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : metricsSubView === "items" && listBearingMetric ? (
                /* Sub-View 3: Dedicated Entity / Player List with Declarative Quick Action Buttons */
                <div className="flex flex-1 flex-col overflow-hidden bg-white">
                  <div className="flex-1 divide-y divide-[#EEF0F6] overflow-y-auto">
                    {(() => {
                      const rawItems = listBearingMetric.items || [];
                      const q = itemSearch.trim().toLowerCase();
                      const filtered = q ? rawItems.filter((x) => x.toLowerCase().includes(q)) : rawItems;
                      const bindParamName = listBearingMetric.bindParam || "player";

                      // Resolve bound consoleActions from quickActionIds (or single-param player actions)
                      const boundConsoleActions = mergedActions.filter((act) => {
                        if (listBearingMetric.quickActionIds?.length) {
                          return listBearingMetric.quickActionIds.includes(act.id);
                        }
                        return (
                          act.params?.length === 1 &&
                          (act.params[0].name === bindParamName || act.params[0].name === "player" || act.params[0].name === "steamid") &&
                          (!act.params[0].options || act.params[0].options.length === 0)
                        );
                      });

                      if (filtered.length === 0) {
                        return (
                          <div className="flex h-full flex-col items-center justify-center px-6 text-center text-xs text-[#8A90A6]">
                            <p>{q ? "未找到匹配的在线玩家" : `当前暂无${listBearingMetric.label}`}</p>
                          </div>
                        );
                      }

                      return filtered.map((itemStr) => {
                        const matchId = /^(.+?)\s*\(([^)]+)\)$/.exec(itemStr);
                        const displayName = matchId ? matchId[1].trim() : itemStr;
                        const targetToken = matchId ? matchId[2].trim() : itemStr;

                        return (
                          <div key={itemStr} className="flex items-center justify-between gap-2 px-4 py-2.5 hover:bg-[#FAFBFD]">
                            <div className="flex min-w-0 items-center gap-2">
                              <span className="h-2 w-2 shrink-0 rounded-full bg-[#12B76A]" />
                              <span className="truncate font-mono text-xs font-semibold text-[#14161F]">{displayName}</span>
                              {matchId && (
                                <span className="shrink-0 rounded bg-[#F1F3FB] px-1.5 py-0.5 font-mono text-[10.5px] text-[#5B6075]">
                                  {targetToken}
                                </span>
                              )}
                            </div>

                            {canExec && (
                              <div className="flex shrink-0 flex-wrap items-center gap-1.5">
                                {boundConsoleActions.slice(0, 2).map((act) => {
                                  const isDanger =
                                    Boolean(act.danger) ||
                                    /(^|[-_])(kick|ban|kill|deop)($|[-_])/i.test(act.id) ||
                                    /^(kick|ban|kill)\b/i.test(act.command.trim());
                                  return (
                                    <Button
                                      key={act.id}
                                      size="sm"
                                      variant={isDanger ? "danger" : "secondary"}
                                      disabled={!running || sendingCmd}
                                      onClick={() => {
                                        if ((act.params?.length || 0) > 1) {
                                          setMultiInputs((prev) => ({
                                            ...prev,
                                            [act.id]: {
                                              ...(prev[act.id] || {}),
                                              [bindParamName]: targetToken,
                                              player: targetToken,
                                              steamid: targetToken,
                                            },
                                          }));
                                          setExpandedActionId(act.id);
                                          setMode("commands");
                                          return;
                                        }
                                        const re = new RegExp(`\\{\\{\\s*(${bindParamName}|player|steamid|item)\\s*\\}\\}`, "g");
                                        const cmd = act.command.replace(re, targetToken);
                                        void triggerSendGameCommand(cmd);
                                      }}
                                      className="!h-6 !px-2.5 !text-[11px]"
                                    >
                                      {act.title}
                                    </Button>
                                  );
                                })}

                                {(listBearingMetric.itemActions || []).map((act) => {
                                  const normCmd = (c: string) =>
                                    c
                                      .replace(/\{\{\s*\w+\s*\}\}/g, "{{item}}")
                                      .replace(/["']/g, "")
                                      .trim()
                                      .toLowerCase();
                                  const isDuplicate = boundConsoleActions.slice(0, 2).some(
                                    (b) =>
                                      b.id === act.id ||
                                      b.id.endsWith(`-${act.id}`) ||
                                      b.title === act.title ||
                                      b.title.includes(act.title) ||
                                      act.title.includes(b.title) ||
                                      normCmd(b.command) === normCmd(act.command),
                                  );
                                  if (isDuplicate) return null;
                                  return (
                                    <Button
                                      key={act.id}
                                      size="sm"
                                      variant={act.danger ? "danger" : "secondary"}
                                      disabled={!running || sendingCmd}
                                      onClick={() => {
                                        const cmd = act.command.replace(/\{\{\s*(item|player|steamid)\s*\}\}/g, targetToken);
                                        void triggerSendGameCommand(cmd);
                                      }}
                                      className="!h-6 !px-2.5 !text-[11px]"
                                    >
                                      {act.title}
                                    </Button>
                                  );
                                })}

                                <Button
                                  size="sm"
                                  variant="ghost"
                                  onClick={() => {
                                    const nextSingle: Record<string, string> = { ...singleInputs };
                                    const nextMulti: Record<string, Record<string, string>> = { ...multiInputs };
                                    for (const act of mergedActions) {
                                      if (
                                        act.params?.some(
                                          (p) => p.name === bindParamName || p.name === "player" || p.name === "steamid",
                                        )
                                      ) {
                                        nextSingle[act.id] = targetToken;
                                        nextMulti[act.id] = {
                                          ...(nextMulti[act.id] || {}),
                                          [bindParamName]: targetToken,
                                          player: targetToken,
                                          steamid: targetToken,
                                        };
                                      }
                                    }
                                    setSingleInputs(nextSingle);
                                    setMultiInputs(nextMulti);
                                    setMode("commands");
                                  }}
                                  className="!h-6 !bg-[#EEF1FF] !px-2.5 !text-[11px] !font-semibold !text-[#2B3AE8] hover:!bg-[#E0E6FF]"
                                >
                                  更多指令 ▸
                                </Button>
                              </div>
                            )}
                          </div>
                        );
                      });
                    })()}
                  </div>
                </div>
              ) : (
                /* Sub-View 1 (Default): System Resources (CPU, Memory, Storage) */
                <div className="flex-1 space-y-2.5 overflow-y-auto p-3.5">
                  {/* CPU Trend Card */}
                  <div className="rounded-2xl border border-[#E3E6F2] bg-white p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-baseline gap-2">
                    <span className="text-xs font-medium text-[#5B6075]">CPU 负载</span>
                    <span className="text-sm font-semibold text-[#14161F]">{formatCores(curCpuMilli)}</span>
                    <span className="text-[11px] text-[#8A90A6]">/ {formatCores(cpuLimitMilli)}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-[#5B6075]">
                    <span>均值 {formatCores(cpuAvgMilli)}</span>
                    <span className="text-[#C4C8D8]">·</span>
                    <span>峰值 {formatCores(cpuPeakMilli)}</span>
                    <span className="rounded-full bg-[#EEF1FF] px-2 py-0.5 font-mono font-semibold text-[#2B3AE8]">
                      {cpuPct}%
                    </span>
                  </div>
                </div>
                <MetricAreaChart
                  id="cpu"
                  samples={samples}
                  limit={cpuLimitMilli}
                  color="#2B3AE8"
                  valueOf={(s) => s.cpuMillicores}
                  formatValue={formatCores}
                />
              </div>

              {/* Memory Trend Card */}
              <div className="rounded-2xl border border-[#E3E6F2] bg-white p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-baseline gap-2">
                    <span className="text-xs font-medium text-[#5B6075]">内存占用</span>
                    <span className="text-sm font-semibold text-[#14161F]">{formatBytes(curMemBytes)}</span>
                    <span className="text-[11px] text-[#8A90A6]">/ {formatBytes(memLimitBytes)}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-[#5B6075]">
                    <span>均值 {formatBytes(memAvgBytes)}</span>
                    <span className="text-[#C4C8D8]">·</span>
                    <span>峰值 {formatBytes(memPeakBytes)}</span>
                    <span className="rounded-full bg-[#E7F6EC] px-2 py-0.5 font-mono font-semibold text-[#1F7A45]">
                      {memPct}%
                    </span>
                  </div>
                </div>
                <MetricAreaChart
                  id="mem"
                  samples={samples}
                  limit={memLimitBytes}
                  color="#1F9D55"
                  valueOf={(s) => s.memoryBytes}
                  formatValue={formatBytes}
                />
              </div>

              {/* Network Throughput Trend Card */}
              <div className="rounded-2xl border border-[#E3E6F2] bg-white p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-baseline gap-2">
                    <span className="text-xs font-medium text-[#5B6075]">网络速率</span>
                    <span className="text-sm font-semibold text-[#14161F] tabular-nums">
                      ↓ {formatRateBytes(curNetRx)} · ↑ {formatRateBytes(curNetTx)}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-[#5B6075] tabular-nums">
                    <span>
                      峰值 ↓ {formatRateBytes(netRxPeak)} · ↑ {formatRateBytes(netTxPeak)}
                    </span>
                  </div>
                </div>
                <MetricAreaChart
                  id="net"
                  samples={samples}
                  limit={netScaleMax}
                  color="#0EA5E9"
                  valueOf={(s) => (s.networkRxBytesPerSec || 0) + (s.networkTxBytesPerSec || 0)}
                  formatValue={formatRateBytes}
                  formatSample={(s) =>
                    `↓ ${formatRateBytes(s.networkRxBytesPerSec)} · ↑ ${formatRateBytes(s.networkTxBytesPerSec)}`
                  }
                />
              </div>

              {/* Compact Storage Bar */}
              <div className="flex items-center justify-between gap-3 rounded-2xl border border-[#E3E6F2] bg-white px-3.5 py-2.5">
                <div className="flex items-baseline gap-2">
                  <span className="text-xs font-medium text-[#5B6075]">存储空间</span>
                  <span className="text-xs font-semibold text-[#14161F]">{formatBytes(curDiskBytes)}</span>
                  <span className="text-[11px] text-[#8A90A6]">/ {formatBytes(diskLimitBytes)}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Meter label="存储空间" used={curDiskBytes} max={diskLimitBytes} hideHeader className="w-28" />
                  <span className="font-mono text-[11px] font-medium text-[#5B6075]">{diskPct}%</span>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
      {activeMode === "commands" && (
            builderOpen ? (
              /* Custom Command Builder Sub-View */
              <div className="flex flex-1 flex-col justify-between overflow-y-auto bg-white">
                <div className="space-y-3.5 p-4">
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1.3fr_1fr]">
                    <div>
                      <label htmlFor={`ft-title-${name}`} className="mb-1 block text-[11.5px] font-semibold text-[#5B6075]">
                        指令显示名称
                      </label>
                      <Input
                        id={`ft-title-${name}`}
                        type="text"
                        value={customTitle}
                        onChange={(e) => setCustomTitle(e.target.value)}
                        placeholder="如：给指定玩家刷物品"
                        className="h-8.5 rounded-lg border-[#E3E6F2] px-3 text-xs"
                      />
                    </div>
                    <div>
                      <label htmlFor={`ft-group-${name}`} className="mb-1 block text-[11.5px] font-semibold text-[#5B6075]">
                        归属分类
                      </label>
                      <Input
                        id={`ft-group-${name}`}
                        type="text"
                        list={`custom-groups-${name}`}
                        value={customGroup}
                        onChange={(e) => setCustomGroup(e.target.value)}
                        placeholder="如：玩家管理"
                        className="h-8.5 rounded-lg border-[#E3E6F2] px-3 text-xs"
                      />
                      <datalist id={`custom-groups-${name}`}>
                        {categories.map((c) => (
                          <option key={c} value={c} />
                        ))}
                      </datalist>
                    </div>
                  </div>

                  <div>
                    <label htmlFor={`ft-tpl-${name}`} className="mb-1 block text-[11.5px] font-semibold text-[#5B6075]">
                      游戏内指令模板（支持在光标处插入 1 个或多个动态参数）
                    </label>
                    <Input
                      id={`ft-tpl-${name}`}
                      ref={customInputRef}
                      type="text"
                      autoCorrect="off"
                      autoCapitalize="none"
                      spellCheck={false}
                      value={customTemplate}
                      onChange={(e) => setCustomTemplate(e.target.value)}
                      onKeyUp={(e) => {
                        cursorPosRef.current = {
                          start: e.currentTarget.selectionStart ?? e.currentTarget.value.length,
                          end: e.currentTarget.selectionEnd ?? e.currentTarget.value.length,
                        };
                      }}
                      onClick={(e) => {
                        cursorPosRef.current = {
                          start: e.currentTarget.selectionStart ?? e.currentTarget.value.length,
                          end: e.currentTarget.selectionEnd ?? e.currentTarget.value.length,
                        };
                      }}
                      placeholder='如：c_give("{{item}}", {{count}})'
                      className="h-9 rounded-lg border-[#E3E6F2] px-3 font-mono text-xs"
                    />
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                      <span className="text-[11px] text-[#8A90A6]">光标处插入变量：</span>
                      {QUICK_VARS.map((v) => (
                        <button
                          key={v.name}
                          type="button"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => insertBuilderVar(v.name)}
                          className={cx(
                            "rounded-md border border-dashed border-[#C6D4FF] bg-[#EEF1FF] px-2 py-0.5 font-mono text-[11px] font-semibold text-[#2B3AE8] hover:bg-[#2B3AE8] hover:text-white",
                            focusRing,
                          )}
                        >
                          + {`{{${v.name}}}`} {v.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {detectedCustomVars.length > 0 && (
                    <div className="rounded-xl bg-[#F7F8FC] p-3">
                      <div className="mb-2 grid grid-cols-[90px_1fr_1fr] gap-2 text-[11px] font-semibold text-[#5B6075]">
                        <span>模板变量</span>
                        <span>输入框中文标题</span>
                        <span>默认初始值（选填）</span>
                      </div>
                      <div className="space-y-2">
                        {detectedCustomVars.map((v) => {
                          const meta = customParamMeta[v] || { label: v, defaultValue: "" };
                          return (
                            <div key={v} className="grid grid-cols-[90px_1fr_1fr] items-center gap-2">
                              <code className="rounded-md bg-[#EEF1FF] px-2 py-1 text-center font-mono text-xs font-semibold text-[#2B3AE8]">
                                {`{{${v}}}`}
                              </code>
                              <Input
                                type="text"
                                aria-label={`${v} 参数中文标题`}
                                value={meta.label}
                                onChange={(e) =>
                                  setCustomParamMeta((prev) => ({
                                    ...prev,
                                    [v]: { ...meta, label: e.target.value },
                                  }))
                                }
                                placeholder="参数中文名称"
                                className="h-7.5 rounded-lg border-[#E3E6F2] px-2.5 text-xs"
                              />
                              <Input
                                type="text"
                                aria-label={`${v} 默认初始值`}
                                value={meta.defaultValue}
                                onChange={(e) =>
                                  setCustomParamMeta((prev) => ({
                                    ...prev,
                                    [v]: { ...meta, defaultValue: e.target.value },
                                  }))
                                }
                                placeholder="选填默认值"
                                className="h-7.5 rounded-lg border-[#E3E6F2] px-2.5 text-xs"
                              />
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>

                <div className="flex items-center justify-end gap-2 border-t border-[#E3E6F2] bg-[#F7F8FC] px-4 py-2.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setBuilderOpen(false)}
                    className="!text-xs"
                  >
                    取消
                  </Button>
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={!customTitle.trim() || !customTemplate.trim()}
                    onClick={handleSaveCustomCommand}
                    className="!text-xs"
                  >
                    保存自定义指令
                  </Button>
                </div>
              </div>
            ) : (
              /* Main Categorized Game Command Workbench */
              <div className="flex flex-1 flex-col overflow-hidden bg-white">
                {/* Category Filter Pill Bar + Optional Multi-World Target Switcher */}
                <div className="flex shrink-0 items-center justify-between gap-2 border-b border-[#E3E6F2] bg-[#F7F8FC] px-3 py-2">
                  <Segmented
                    size="sm"
                    label="游戏指令分类"
                    value={activeCategory}
                    onChange={setActiveCategory}
                    className="min-w-0 flex-1"
                    items={[
                      { value: "all", label: "全部" },
                      { value: "fav", label: "收藏" },
                      ...categories.map((cat) => ({ value: cat, label: cat })),
                    ]}
                  />

                  {consoleTargets.length > 1 && (
                    <Segmented
                      size="sm"
                      label="指令投递世界"
                      value={activeTarget?.id || consoleTargets[0]?.id || ""}
                      onChange={setActiveTargetId}
                      className="shrink-0"
                      items={consoleTargets.map((target) => ({ value: target.id, label: target.label }))}
                    />
                  )}
                </div>

                {/* Categorized Action Rows */}
                <div className="flex-1 divide-y divide-[#EEF0F6] overflow-y-auto">
                  {filteredActions.length === 0 ? (
                    <div className="flex h-full flex-col items-center justify-center px-6 text-center text-xs text-[#8A90A6]">
                      <p>当前分类下暂无匹配的游戏指令</p>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setBuilderOpen(true)}
                        className="mt-2 !text-xs !font-semibold !text-[#2B3AE8]"
                      >
                        + 添加第一条自定义指令
                      </Button>
                    </div>
                  ) : (
                    filteredActions.map((action) => {
                      const params = action.params || [];
                      const isEnumSingle =
                        params.length === 1 && params[0].type === "select" && Boolean(params[0].options?.length);
                      const isMulti = params.length >= 2;
                      const isExpanded = isMulti && expandedActionId === action.id;
                      const isCustom = action.id.startsWith("custom-");

                      return (
                        <div key={action.id} className="transition-colors hover:bg-[#FAFBFD]">
                          <div className="flex items-center justify-between gap-2.5 px-3.5 py-2.5">
                            <div className="flex min-w-0 items-center gap-2">
                              <button
                                type="button"
                                aria-label={action.starred ? "取消收藏" : "加入收藏"}
                                title={action.starred ? "取消收藏" : "加入收藏"}
                                onClick={() => toggleStarAction(action)}
                                className={cx(
                                  "shrink-0 rounded p-0.5 transition-colors",
                                  action.starred ? "text-[#F59E0B]" : "text-[#CBD5E1] hover:text-[#8A90A6]",
                                  focusRing,
                                )}
                              >
                                <Star className={cx("h-3.5 w-3.5", action.starred && "fill-current")} />
                              </button>
                              <div className="min-w-0">
                                <div className="flex items-center gap-1.5">
                                  <span className="truncate text-[12.5px] font-semibold text-[#14161F]">
                                    {action.title}
                                  </span>
                                  {activeCategory === "all" && action.group && (
                                    <span className="shrink-0 rounded bg-[#F1F3F9] px-1.5 py-0.2 text-[10.5px] text-[#8A90A6]">
                                      {action.group}
                                    </span>
                                  )}
                                </div>
                                <div className="truncate font-mono text-[11px] text-[#8A90A6]">{action.command}</div>
                              </div>
                            </div>

                            <div className="flex shrink-0 items-center gap-1.5">
                              {params.length === 0 ? (
                                <Button
                                  variant="secondary"
                                  size="sm"
                                  disabled={!running || sendingCmd}
                                  onClick={() => void triggerSendGameCommand(action.command)}
                                  className="!h-7 !px-2.5 !text-xs"
                                >
                                  执行
                                </Button>
                              ) : isEnumSingle ? (
                                (params[0].options?.length || 0) > 5 ? (
                                  <div className="flex items-center gap-1.5">
                                    <Select
                                      aria-label={params[0].label || params[0].name}
                                      value={
                                        singleInputs[action.id] ??
                                        params[0].defaultValue ??
                                        params[0].options?.[0]?.value ??
                                        ""
                                      }
                                      disabled={!running || sendingCmd}
                                      onChange={(e) =>
                                        setSingleInputs((prev) => ({ ...prev, [action.id]: e.target.value }))
                                      }
                                      className="h-7 w-40 rounded-lg border-[#E3E6F2] pl-2 text-xs"
                                    >
                                      {(params[0].options || []).map((opt) => (
                                        <option key={opt.value} value={opt.value}>
                                          {opt.label}
                                        </option>
                                      ))}
                                    </Select>
                                    <Button
                                      variant="primary"
                                      size="sm"
                                      disabled={!running || sendingCmd}
                                      onClick={() => {
                                        const val = (
                                          singleInputs[action.id] ??
                                          params[0].defaultValue ??
                                          params[0].options?.[0]?.value ??
                                          ""
                                        ).trim();
                                        if (!val) return;
                                        const resolved = action.command.replace(
                                          new RegExp(`\\{\\{\\s*${params[0].name}\\s*\\}\\}`, "g"),
                                          val,
                                        );
                                        void triggerSendGameCommand(resolved);
                                      }}
                                      className="!h-7 !px-2.5 !text-xs"
                                    >
                                      发送
                                    </Button>
                                  </div>
                                ) : (
                                  <div className="inline-flex items-center gap-0.5 rounded-lg bg-[#F1F3F9] p-0.5">
                                    {(params[0].options || []).map((opt) => {
                                      const resolved = action.command.replace(
                                        new RegExp(`\\{\\{\\s*${params[0].name}\\s*\\}\\}`, "g"),
                                        opt.value,
                                      );
                                      return (
                                        <button
                                          key={opt.value}
                                          type="button"
                                          disabled={!running || sendingCmd}
                                          onClick={() => void triggerSendGameCommand(resolved)}
                                          className={cx(
                                            "rounded-md px-2 py-1 text-[11.5px] font-medium text-[#14161F] transition-colors hover:bg-[#2B3AE8] hover:text-white disabled:opacity-50",
                                            focusRing,
                                          )}
                                        >
                                          {opt.label}
                                        </button>
                                      );
                                    })}
                                  </div>
                                )
                              ) : params.length === 1 ? (
                                <div className="flex items-center gap-1.5">
                                  <Input
                                    type={params[0].type === "number" ? "number" : "text"}
                                    aria-label={params[0].label || params[0].name}
                                    value={singleInputs[action.id] ?? params[0].defaultValue ?? ""}
                                    onChange={(e) =>
                                      setSingleInputs((prev) => ({ ...prev, [action.id]: e.target.value }))
                                    }
                                    onKeyDown={(e) => {
                                      if (e.key === "Enter") {
                                        const val = (singleInputs[action.id] ?? params[0].defaultValue ?? "").trim();
                                        if (!val) return;
                                        const resolved = action.command.replace(
                                          new RegExp(`\\{\\{\\s*${params[0].name}\\s*\\}\\}`, "g"),
                                          val,
                                        );
                                        void triggerSendGameCommand(resolved);
                                      }
                                    }}
                                    placeholder={params[0].label || params[0].name}
                                    className="h-7 w-32 rounded-lg border-[#E3E6F2] px-2 text-xs"
                                  />
                                  <Button
                                    variant="primary"
                                    size="sm"
                                    disabled={!running || sendingCmd}
                                    onClick={() => {
                                      const val = (singleInputs[action.id] ?? params[0].defaultValue ?? "").trim();
                                      if (!val) return;
                                      const resolved = action.command.replace(
                                        new RegExp(`\\{\\{\\s*${params[0].name}\\s*\\}\\}`, "g"),
                                        val,
                                      );
                                      void triggerSendGameCommand(resolved);
                                    }}
                                    className="!h-7 !px-2.5 !text-xs"
                                  >
                                    发送
                                  </Button>
                                </div>
                              ) : (
                                <Button
                                  variant="secondary"
                                  size="sm"
                                  onClick={() => setExpandedActionId(isExpanded ? null : action.id)}
                                  className={cx(
                                    "!h-7 !gap-1 !px-2.5 !text-xs",
                                    isExpanded && "!border-[#C6D4FF] !bg-[#EEF1FF] !text-[#2B3AE8]",
                                  )}
                                >
                                  <span>{params.length} 个参数</span>
                                  {isExpanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
                                </Button>
                              )}

                              {isCustom && (
                                <IconButton
                                  label="删除自定义指令"
                                  title="删除自定义指令"
                                  onClick={() => deleteCustomAction(action.id)}
                                  className="!h-7 !w-7 !bg-transparent text-[#8A90A6] hover:!bg-[#FDECEC] hover:!text-[#C83A2B]"
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </IconButton>
                              )}
                            </div>
                          </div>

                          {/* Borderless #F7F8FC Multi-Parameter Row Expansion */}
                          {isExpanded && (
                            <div className="border-t border-dashed border-[#E3E6F2] bg-[#F7F8FC] py-2.5 pr-3.5 pl-9">
                              <div
                                className={cx(
                                  "grid gap-2",
                                  params.length >= 3 ? "grid-cols-3" : "grid-cols-2",
                                )}
                              >
                                {params.map((p) => {
                                  const curVal =
                                    multiInputs[action.id]?.[p.name] ?? p.defaultValue ?? p.options?.[0]?.value ?? "";
                                  return (
                                    <div key={p.name}>
                                      <label
                                        htmlFor={`ft-mp-${action.id}-${p.name}`}
                                        className="mb-1 block text-[11px] font-semibold text-[#5B6075]"
                                      >
                                        {p.label || p.name}
                                      </label>
                                      {p.type === "select" && p.options?.length ? (
                                        <Select
                                          id={`ft-mp-${action.id}-${p.name}`}
                                          value={curVal}
                                          onChange={(e) =>
                                            setMultiInputs((prev) => ({
                                              ...prev,
                                              [action.id]: { ...(prev[action.id] || {}), [p.name]: e.target.value },
                                            }))
                                          }
                                          className="h-7.5 rounded-lg border-[#E3E6F2] pl-2 text-xs"
                                        >
                                          {p.options.map((o) => (
                                            <option key={o.value} value={o.value}>
                                              {o.label}
                                            </option>
                                          ))}
                                        </Select>
                                      ) : (
                                        <Input
                                          id={`ft-mp-${action.id}-${p.name}`}
                                          type={p.type === "number" ? "number" : "text"}
                                          value={curVal}
                                          onChange={(e) =>
                                            setMultiInputs((prev) => ({
                                              ...prev,
                                              [action.id]: { ...(prev[action.id] || {}), [p.name]: e.target.value },
                                            }))
                                          }
                                          className="h-7.5 rounded-lg border-[#E3E6F2] px-2 text-xs"
                                        />
                                      )}
                                    </div>
                                  );
                                })}
                              </div>
                              <div className="mt-2.5 flex items-center justify-between gap-2">
                                <code className="truncate rounded bg-[#EEF1FF] px-2 py-0.5 font-mono text-[11px] text-[#2B3AE8]">
                                  {resolveMultiActionCommand(action)}
                                </code>
                                <Button
                                  variant="primary"
                                  size="sm"
                                  disabled={!running || sendingCmd}
                                  onClick={() => void triggerSendGameCommand(resolveMultiActionCommand(action))}
                                  className="!h-7 shrink-0 !px-3 !text-xs"
                                >
                                  发送指令
                                </Button>
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    })
                  )}
                </div>

                {/* Single-line Raw Game Command Fallback Bar */}
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (!rawGameCmd.trim()) return;
                    void triggerSendGameCommand(rawGameCmd);
                    setRawGameCmd("");
                  }}
                  className="flex shrink-0 items-center gap-2 border-t border-[#E3E6F2] bg-[#F7F8FC] px-3.5 py-2"
                >
                  <span className="select-none font-mono text-xs text-[#8A90A6]">&gt;</span>
                  <Input
                    type="text"
                    aria-label="原生游戏指令"
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    value={rawGameCmd}
                    disabled={!running || sendingCmd}
                    onChange={(e) => setRawGameCmd(e.target.value)}
                    placeholder={!running ? "服务器未运行" : "输入任意原生游戏指令回车发送..."}
                    className="h-7.5 min-w-0 flex-1 rounded-lg border-[#E3E6F2] px-2.5 font-mono text-xs"
                  />
                  <Button
                    type="submit"
                    variant="primary"
                    size="sm"
                    disabled={!running || sendingCmd || !rawGameCmd.trim()}
                    className="!h-7.5 !px-3 !text-xs"
                  >
                    发送
                  </Button>
                </form>
              </div>
            )
          )}
          {canExec && terminalActivated && (
            <div className={cx("min-h-0 flex-1 overflow-hidden bg-[#F7F8FC]", activeMode !== "terminal" && "hidden")}>
              <InteractiveTerminal
                ref={terminalRef}
                name={name}
                running={running}
                activated={terminalActivated}
                visible={open && stage === "panel" && activeMode === "terminal"}
                onStatusChange={setTermStatus}
              />
            </div>
          )}
        </aside>
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
              onOpenChange(false);
            } else {
              openedByFabRef.current = true;
              setStage("menu");
              onOpenChange(true);
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
