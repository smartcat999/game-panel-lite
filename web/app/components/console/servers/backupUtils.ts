import type {
  InstanceBackupRecord,
  InstanceBackupSaveConfirmation,
  InstanceItem,
} from "../../../console/lib/types";
import type { BackupConfigPreviewItem, BackupInspectResult } from "./api";

export function encodeLabelHex(label: string): string {
  const bytes = new TextEncoder().encode(label.trim());
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export function decodeLabelHex(hex: string): string | null {
  if (!hex || hex.length % 2 !== 0 || !/^[0-9a-f]+$/i.test(hex)) return null;
  try {
    const bytes = new Uint8Array(hex.length / 2);
    for (let i = 0; i < hex.length; i += 2) {
      bytes[i / 2] = parseInt(hex.slice(i, i + 2), 16);
    }
    return new TextDecoder().decode(bytes);
  } catch {
    return null;
  }
}

export function buildArchivePath(instanceName: string, customLabel?: string, ext: ".tar.gz" | ".zip" = ".tar.gz"): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  const ts = `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`;
  const trimmed = (customLabel || "").trim();
  if (trimmed) {
    return `backups/${instanceName}-${ts}--${encodeLabelHex(trimmed)}${ext}`;
  }
  return `backups/${instanceName}-${ts}${ext}`;
}

export function formatBackupDisplayName(rawPath: string, instanceName: string): string {
  let stem = rawPath
    .replace(/^backups\//, "")
    .replace(/^tenants\/[^/]+\/instances\/[^/]+\//, "")
    .replace(/^backup-[0-9a-f]+-/i, "")
    .replace(/\.(tar\.gz|tgz|zip)$/i, "");
  if (stem.startsWith(`${instanceName}-`)) {
    stem = stem.slice(instanceName.length + 1);
  }
  const hexMatch = stem.match(/^(?:(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2}))?--([0-9a-f]+)$/i);
  if (hexMatch) {
    const decoded = decodeLabelHex(hexMatch[7]);
    if (decoded) return decoded;
  }
  const autoMatch = stem.match(/^auto-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/);
  if (autoMatch) {
    const [, y, m, d, hh, mm] = autoMatch;
    return `自动备份 ${y}-${m}-${d} ${hh}:${mm}`;
  }
  const tsMatch = stem.match(/^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/);
  if (tsMatch) {
    const [, y, m, d, hh, mm] = tsMatch;
    return `存档备份 ${y}-${m}-${d} ${hh}:${mm}`;
  }
  return stem;
}

export function isTerminalBackupPhase(phase?: string): boolean {
  return !phase || phase === "Completed" || phase === "Failed" || phase === "InspectFailed";
}

export function formatBackupStageLabel(stage?: string, isRestoring?: boolean): string {
  switch (stage) {
    case "Quota":
      return isRestoring ? "正在准备恢复任务" : "正在校验配额";
    case "PreHook":
      return "正在保存世界存档";
    case "Archiving":
      return "正在打包存档";
    case "Uploading":
      return "正在上传至对象存储";
    case "Inspecting":
      return "正在解析归档目录与游戏配置";
    case "Stopping":
      return "正在安全停服";
    case "Downloading":
      return "正在拉取备份归档";
    case "Extracting":
      return "正在还原存档文件";
    case "Restarting":
      return "正在重启服务器";
    case "Settling":
      return "正在结算配额";
    case "Completed":
      return "";
    case "Failed":
      return "执行失败";
    default:
      if (!stage || /^[\x00-\x7F]+$/.test(stage)) return "";
      return stage;
  }
}

export function splitSubtreeDirs(raw?: string): string[] {
  if (!raw) return [];
  const out: string[] = [];
  for (const part of raw.split(",")) {
    const norm = part.trim().replace(/\\/g, "/").replace(/^\/+|\/+$/g, "");
    if (!norm || norm === ".") continue;
    if (!out.includes(norm)) out.push(norm);
  }
  return out;
}

export function filterManagedFilesForTarget(
  files: Array<{ path: string }> | undefined,
  target: string,
  fallback: string[],
): string[] {
  if (!files || files.length === 0) return fallback;
  const matched: string[] = [];
  for (const f of files) {
    const norm = (f.path || "").trim().replace(/\\/g, "/").replace(/^\/+|\/+$/g, "");
    if (!norm || norm === ".") continue;
    if (target === "." || norm === target || norm.startsWith(`${target}/`)) {
      if (!matched.includes(norm)) matched.push(norm);
    }
  }
  return matched.sort();
}

export function formatConfigSideValue(
  item: BackupConfigPreviewItem,
  side: "archive" | "current",
): { text: string; muted: boolean } {
  const raw = (side === "archive" ? item.archiveValue : item.currentValue) || "";
  const isSet = side === "archive" ? (item.archiveSet ?? Boolean(raw)) : (item.currentSet ?? Boolean(raw));
  if (item.secret) {
    return { text: isSet ? "已配置" : "未设置", muted: !isSet };
  }
  if (!isSet) {
    return { text: "未设置", muted: true };
  }
  if (item.type === "boolean") {
    return { text: raw === "true" ? "开启" : raw === "false" ? "关闭" : raw, muted: false };
  }
  return { text: raw, muted: false };
}

export function formatArchiveFormatLabel(rawPath: string, explicitFormat?: string): string {
  const fmt = (explicitFormat || "").trim().toLowerCase();
  if (fmt === "zip" || /\.zip$/i.test(rawPath)) return "ZIP";
  if (fmt === "tar.gz" || fmt === "tgz" || /\.(tar\.gz|tgz)$/i.test(rawPath)) return "TAR.GZ";
  return fmt ? fmt.toUpperCase() : "";
}

export function formatBackupDateTime(iso?: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "-";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function formatBackupAddonVersion(version?: string): string {
  const v = (version || "").trim();
  if (!v || v === "1" || v.toLowerCase() === "latest") return "";
  if (/^\d{9,11}$/.test(v)) {
    const sec = Number(v);
    if (Number.isFinite(sec) && sec > 946684800) {
      const d = new Date(sec * 1000);
      const p = (n: number) => String(n).padStart(2, "0");
      return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
    }
  }
  return v;
}

export function formatAddonScopeLabel(scope: string): string {
  if (scope === "Master") return "地上";
  if (scope === "Caves") return "洞穴";
  return scope;
}

export function formatBackupShardList(shards?: string[]): string {
  if (!shards || shards.length === 0) return "";
  return shards.map((s) => (s === "master" ? "地上" : s === "caves" ? "洞穴" : s)).join(" + ");
}

export function formatBackupSaveConfirmationSummary(
  conf?: InstanceBackupSaveConfirmation,
  options?: { isLegacyDST?: boolean },
): { status: "Confirmed" | "Unconfirmed" | "Unrecorded" | null; label: string; warning?: string } {
  if (conf?.status === "Unconfirmed") {
    return {
      status: "Unconfirmed",
      label: "已归档，存档未确认",
      warning: conf.warning || "未收到完整存档落盘回执",
    };
  }
  if (conf?.status === "Confirmed") {
    const shards = formatBackupShardList(conf.confirmedShards || conf.expectedShards);
    return {
      status: "Confirmed",
      label: shards ? `存档已确认（${shards}）` : "存档已确认",
    };
  }
  if (options?.isLegacyDST) {
    return { status: "Unrecorded", label: "存档确认：未记录" };
  }
  return { status: null, label: "" };
}

export function buildBackupRowMeta(record: InstanceBackupRecord, addonNoun = "模组"): string {
  const parts: string[] = [];
  const format = formatArchiveFormatLabel(record.path, record.inspection?.archiveFormat);
  if (format) parts.push(format);
  const ver = record.snapshot?.serverVersion;
  if (ver) parts.push(ver);
  const addonCount =
    record.snapshot?.addonCount ?? record.snapshot?.addons?.length ?? record.inspection?.extractedAddons?.length ?? 0;
  if (addonCount > 0) parts.push(`${addonCount} 个${addonNoun}`);
  const fileCount = record.inspection?.fileCount ?? 0;
  if (fileCount > 0) parts.push(`${fileCount} 个文件`);
  return parts.join(" · ");
}

export function resolveDefaultRestoreTargetPath(inst: InstanceItem, subtreeFallback?: string): string {
  const dirs = splitSubtreeDirs(inst.spec.backupPolicy?.subtree || subtreeFallback);
  if (dirs.length === 1) return dirs[0];
  return ".";
}

export function parseRestoreTargetInput(raw: string): { normalized: string; empty: boolean; error?: string } {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { normalized: "", empty: true };
  }
  if (trimmed.includes("\0")) {
    return { normalized: "", empty: false, error: "目标路径包含非法字符" };
  }
  const slash = trimmed.replace(/\\/g, "/");
  if (/^[a-zA-Z]:/.test(slash)) {
    return { normalized: "", empty: false, error: "请填写相对于实例工作区的目录路径" };
  }
  let norm = slash.replace(/\/+/g, "/");
  while (norm.startsWith("./")) norm = norm.slice(2);
  while (norm.startsWith("/")) norm = norm.slice(1);
  while (norm.startsWith("./")) norm = norm.slice(2);
  norm = norm.replace(/\/+$/, "");
  if (!norm || norm === ".") {
    return { normalized: ".", empty: false };
  }
  const parts = norm.split("/").filter((seg) => seg !== "" && seg !== ".");
  if (parts.some((seg) => seg === "..")) {
    return { normalized: "", empty: false, error: "目标路径不能包含 .. 上级目录" };
  }
  const cleaned = parts.join("/");
  if (!cleaned || cleaned === ".") {
    return { normalized: ".", empty: false };
  }
  if (parts.some((seg) => seg.startsWith(".gp-"))) {
    return { normalized: "", empty: false, error: "不能使用系统保留临时目录" };
  }
  return { normalized: cleaned, empty: false };
}

const MAX_INSPECT_CACHE = 64;
const backupInspectCache = new Map<string, BackupInspectResult>();

function inspectCacheKey(instanceName: string, backupPath: string, revisionKey = ""): string {
  return `${instanceName}\u0000${backupPath}\u0000${revisionKey}`;
}

export function getLatestBackupInspect(
  instanceName: string,
  backupPath: string,
  revisionKey = "",
): BackupInspectResult | null {
  if (!instanceName || !backupPath) return null;
  const cached = backupInspectCache.get(inspectCacheKey(instanceName, backupPath, revisionKey));
  return cached ?? null;
}

export function setCachedBackupInspect(
  instanceName: string,
  backupPath: string,
  data: BackupInspectResult,
  revisionKey = "",
): void {
  if (!instanceName || !backupPath || data.phase === "Pending") return;
  const key = inspectCacheKey(instanceName, backupPath, revisionKey);
  backupInspectCache.delete(key);
  backupInspectCache.set(key, data);
  if (backupInspectCache.size > MAX_INSPECT_CACHE) {
    const oldest = backupInspectCache.keys().next().value;
    if (oldest !== undefined) backupInspectCache.delete(oldest);
  }
}
