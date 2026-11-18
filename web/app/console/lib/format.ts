import type { AppItem, InstanceItem, InstancePort, NodeItem } from "./types";

export function parseNumericQuantity(val?: string): number {
  if (!val) return 0;
  const trimmed = val.trim();
  const num = parseFloat(trimmed);
  if (Number.isNaN(num)) return 0;
  if (trimmed.endsWith("Ti")) return num * 1024;
  if (trimmed.endsWith("Gi") || trimmed.endsWith("G")) return num;
  if (trimmed.endsWith("Mi") || trimmed.endsWith("M")) return num / 1024;
  if (trimmed.endsWith("Ki") || trimmed.endsWith("K")) return num / (1024 * 1024);
  if (trimmed.endsWith("m")) return num / 1000;
  if (num >= 1024 * 1024) return num / (1024 * 1024 * 1024);
  return num;
}

const trimNum = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));

/** "4" -> "4 核", "500m" -> "0.5 核" */
export function formatCpu(val?: string): string {
  return `${trimNum(parseNumericQuantity(val || "0"))} 核`;
}

/** "8Gi" -> "8 GB", "512Mi" -> "0.5 GB" (users think in GB, not GiB). */
export function formatGB(val?: string): string {
  const gib = parseNumericQuantity(val || "0");
  if (gib >= 1024) return `${trimNum(gib / 1024)} TB`;
  return `${trimNum(gib)} GB`;
}

export function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

export function formatRateBytes(bytesPerSec?: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return "0 B/s";
  if (bytesPerSec < 1024) return `${Math.round(bytesPerSec)} B/s`;
  const kb = bytesPerSec / 1024;
  if (kb < 1024) return `${kb < 10 ? kb.toFixed(1) : Math.round(kb)} KB/s`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB/s`;
  return `${(mb / 1024).toFixed(2)} GB/s`;
}

export type ServerPhase = "Running" | "Stopped" | "Pending" | "Failed" | string;

export function phaseOf(inst: InstanceItem): ServerPhase {
  if (inst.metadata.deletionTimestamp) return "Terminating";
  const p = inst.status?.phase;
  const readyReason = inst.status?.conditions?.find((c) => c.type === "Ready")?.reason;
  if (p === "Starting" && (readyReason === "Restarting" || readyReason === "RestartPending")) {
    return "Restarting";
  }
  const restartAt = inst.metadata.annotations?.["core.gamepanel.io/restart-requested-at"];
  if (
    inst.spec.state === "Running" &&
    p === "Running" &&
    restartAt &&
    inst.status?.startedAt &&
    new Date(restartAt).getTime() > new Date(inst.status.startedAt).getTime()
  ) {
    return "Restarting";
  }
  if (
    inst.spec.state === "Running" &&
    p === "Running" &&
    inst.metadata.generation &&
    inst.status?.observedGeneration &&
    inst.metadata.generation > inst.status.observedGeneration
  ) {
    return "Restarting";
  }
  if (p === "Stopped" && inst.spec.state === "Running") return "Pending";
  if (p === "Running" && inst.spec.state === "Stopped") return "Stopping";
  if (inst.spec.state === "Stopped" && (p === "Pending" || p === "Scheduled" || !p)) return "Stopped";
  return p || (inst.spec.state === "Stopped" ? "Stopped" : "Pending");
}

export const PHASE_LABEL: Record<string, string> = {
  Running: "运行中",
  Stopped: "已停止",
  Pending: "启动中",
  Starting: "启动中",
  Restarting: "重启中",
  Scheduled: "启动中",
  Stopping: "停止中",
  Scheduling: "调度中",
  Creating: "创建中",
  Failed: "异常",
  Terminating: "删除中",
  // the hosting node stopped reporting (heartbeat timeout)
  Unknown: "节点失联",
};

export function phaseLabel(phase: string) {
  return PHASE_LABEL[phase] ?? phase;
}

/**
 * Spec ports merged with published ports reported in status.endpoint,
 * placing the primary port defined first in the game App YAML (`app.ports[0]`) at index 0.
 */
export function portsOf(inst: InstanceItem, apps?: AppItem[]): InstancePort[] {
  const reported = inst.status?.endpoint?.ports || [];
  const base = inst.spec.ports?.length ? inst.spec.ports : reported;
  const merged = base.map((p) => {
    const proto = (p.protocol || "TCP").toUpperCase();
    const r = reported.find(
      (x) =>
        (p.name && x.name === p.name) ||
        (x.targetPort === p.targetPort && (!x.protocol || x.protocol.toUpperCase() === proto)),
    );
    const published = r?.publishedPort || p.publishedPort || 0;
    return { ...p, publishedPort: published > 0 ? published : 0 };
  });
  const appId =
    inst.metadata.labels?.["gamepanel.io/app"]?.trim() ||
    inst.metadata.labels?.["gamepanel.io/preset"]?.trim() ||
    "";
  const yamlPrimary = appId && apps?.length ? apps.find((a) => a.metadata.name === appId)?.ports?.[0] : undefined;
  if (!yamlPrimary || merged.length <= 1) return merged;
  const primaryProto = (yamlPrimary.protocol || "TCP").toUpperCase();
  const idx = merged.findIndex(
    (p) =>
      (yamlPrimary.name && p.name === yamlPrimary.name) ||
      (p.targetPort === yamlPrimary.targetPort && (p.protocol || "TCP").toUpperCase() === primaryProto),
  );
  if (idx <= 0) return merged;
  return [merged[idx], ...merged.slice(0, idx), ...merged.slice(idx + 1)];
}

function isPrivateOrLoopbackHost(host: string): boolean {
  const h = host.trim();
  if (!h) return true;
  if (h === "localhost" || h.startsWith("127.") || h.startsWith("10.") || h.startsWith("192.168.")) return true;
  if (h.startsWith("172.")) {
    const second = Number(h.split(".")[1]);
    if (second >= 16 && second <= 31) return true;
  }
  return false;
}

export function endpointOf(inst: InstanceItem, nodes: NodeItem[], apps?: AppItem[]): string | null {
  const ep = inst.status?.endpoint;
  const node = inst.spec.nodeName ? nodes.find((n) => n.metadata.name === inst.spec.nodeName) : undefined;
  const nodeExternalIP =
    node?.spec?.externalIP?.trim() ||
    node?.status?.addresses?.find((a) => a.type === "ExternalIP")?.address?.trim() ||
    "";
  const nodeInternalIP =
    node?.status?.addresses?.find((a) => a.type === "InternalIP")?.address?.trim() || "";
  let host = node ? nodeExternalIP || nodeInternalIP || ep?.hostIP || "" : ep?.hostIP || "";
  const hasExternalIP = Boolean(nodeExternalIP);
  if (
    !hasExternalIP &&
    isPrivateOrLoopbackHost(host) &&
    typeof window !== "undefined" &&
    window.location.hostname &&
    window.location.hostname !== "localhost" &&
    window.location.hostname !== "127.0.0.1"
  ) {
    host = window.location.hostname;
  }
  const ports = portsOf(inst, apps);
  if (!host || !ports.length) return null;
  const p = ports[0];
  const pub = p.publishedPort && p.publishedPort > 0 ? p.publishedPort : null;
  if (!pub) return null;
  return `${host}:${pub}`;
}

export function resourceOf(inst: InstanceItem, key: "cpu" | "memory" | "storage") {
  return inst.spec.resources?.requests?.[key] || inst.spec.resources?.limits?.[key] || "";
}

export function relativeTime(iso?: string): string {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.round(diff / 60000);
  if (m < 1) return "刚刚";
  if (m < 60) return `${m} 分钟前`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} 小时前`;
  return `${Math.round(h / 24)} 天前`;
}

export function formatDateTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
