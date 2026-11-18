import type { AppConfigField, AppItem, ArtifactRef, InstanceFile, InstancePort } from "./types";

export function resolveRichApp(id?: string | null, apiApps?: AppItem[]): AppItem | undefined {
  if (!id) return undefined;
  return (apiApps || []).find((a) => a.metadata.name === id);
}

export interface GamePreset {
  id: string;
  name: string;
  category: string;
  tagline?: string;
  summary: string;
  defaultName: string;
  image: string;
  command: string;
  portName: string;
  protocol: "TCP" | "UDP";
  targetPort: number;
  cpu: string;
  memory: string;
  storage: string;
  env: Array<{ name: string; value: string }>;
  coverImage?: string;
}

export const APP_CATEGORY_LABELS: Record<string, string> = {
  sandbox: "沙盒建造",
  survival: "生存合作",
  custom: "自定义",
  fps: "动作射击",
  shooter: "动作射击",
  rpg: "角色扮演",
  strategy: "策略模拟",
  simulation: "模拟经营",
  action: "动作冒险",
  other: "其他",
};

export const APP_CATEGORY_ORDER = ["sandbox", "survival", "fps", "custom"] as const;

export function formatAppCategory(raw?: string | null): string {
  const key = (raw || "").trim();
  if (!key) return APP_CATEGORY_LABELS.survival;
  return APP_CATEGORY_LABELS[key.toLowerCase()] || key;
}

export function sortAppCategories(categories: Iterable<string>): string[] {
  const unique = Array.from(new Set(Array.from(categories).map((c) => c.trim()).filter(Boolean)));
  return unique.sort((a, b) => {
    const ai = APP_CATEGORY_ORDER.indexOf(a as (typeof APP_CATEGORY_ORDER)[number]);
    const bi = APP_CATEGORY_ORDER.indexOf(b as (typeof APP_CATEGORY_ORDER)[number]);
    if (ai !== -1 && bi !== -1) return ai - bi;
    if (ai !== -1) return -1;
    if (bi !== -1) return 1;
    return formatAppCategory(a).localeCompare(formatAppCategory(b), "zh-CN");
  });
}

// Player capacity varies by game, so plans describe sizing only; the wizard
// recommends a tier from each game's own `resources` instead of a fixed head count.
export const RESOURCE_PLANS = [
  { id: "starter", label: "轻量入门型", cpu: "1", memory: "2Gi", storage: "10Gi", desc: "适合泰拉瑞亚原版等轻量游戏与测试服" },
  { id: "standard", label: "标准均衡型", cpu: "2", memory: "4Gi", storage: "20Gi", desc: "适合 Minecraft、饥荒联机版等主流联机服" },
  { id: "performance", label: "高频性能型", cpu: "4", memory: "8Gi", storage: "40Gi", desc: "适合幻兽帕鲁、僵尸毁灭工程等大型生存游戏" },
  { id: "extreme", label: "旗舰尊享型", cpu: "8", memory: "16Gi", storage: "80Gi", desc: "适合大型整合包与高并发社区服" },
];

export type ResourcePlan = (typeof RESOURCE_PLANS)[number];

export const RECOMMENDED_PLAN_ID = "performance";

export const DEFAULT_GAME_COVER = "/landing/games/default-cover.jpg";

export function defaultGameCover(gameId?: string): string {
  const trimmed = (gameId || "").trim();
  if (!trimmed || trimmed === "custom-container" || trimmed === "custom") {
    return DEFAULT_GAME_COVER;
  }
  return `/landing/games/${trimmed}.jpg`;
}

export function gameCover(gameId: string, customCover?: string): string {
  const trimmed = customCover?.trim() || "";
  return trimmed || defaultGameCover(gameId);
}

/** Compress an uploaded image File into a compact JPEG data URL for cover art. */
export function compressCoverImageFile(file: File, maxWidth = 800, maxHeight = 1000, quality = 0.86): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("读取图片文件失败"));
    reader.onload = () => {
      const dataUrl = String(reader.result || "");
      const img = new Image();
      img.onerror = () => reject(new Error("解析图片格式失败"));
      img.onload = () => {
        let width = img.naturalWidth || img.width || 600;
        let height = img.naturalHeight || img.height || 750;
        const ratio = Math.min(1, maxWidth / width, maxHeight / height);
        width = Math.max(1, Math.round(width * ratio));
        height = Math.max(1, Math.round(height * ratio));
        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          resolve(dataUrl);
          return;
        }
        ctx.drawImage(img, 0, 0, width, height);
        resolve(canvas.toDataURL("image/jpeg", quality));
      };
      img.src = dataUrl;
    };
    reader.readAsDataURL(file);
  });
}

export function appToPreset(app: AppItem): GamePreset {
  const firstPort = app.ports?.[0] || { name: "game", protocol: "TCP", targetPort: 25565 };
  const proto = (firstPort.protocol || "TCP").toUpperCase() === "UDP" ? "UDP" : "TCP";
  const cmdArr = app.command || [];
  let cmd = "";
  if (cmdArr.length === 3 && cmdArr[0] === "sh" && cmdArr[1] === "-c") {
    cmd = cmdArr[2];
  } else if (cmdArr.length > 0) {
    cmd = cmdArr.join(" ");
  }
  const res = app.resources || {};
  return {
    id: app.metadata.name,
    name: app.displayName || app.metadata.name,
    category: app.category || "survival",
    tagline: app.tagline || "",
    summary: app.summary || app.tagline || "",
    defaultName: `${app.metadata.name.split("-")[0] || "srv"}-01`,
    image: app.image,
    command: cmd,
    portName: firstPort.name || "game",
    protocol: proto,
    targetPort: firstPort.targetPort || 25565,
    cpu: res.cpu || "2",
    memory: res.memory || "4Gi",
    storage: res.storage || "20Gi",
    env: Object.entries(app.env || {}).map(([name, value]) => ({ name, value })),
    coverImage: gameCover(app.metadata.name, app.coverImage),
  };
}

export function normalizeConfigFieldValue(field: AppConfigField, raw: string): string {
  const trimmed = String(raw ?? "").trim();
  if (!trimmed) return trimmed;
  if (field.type === "number") {
    if (/^[+-]?\d+(?:\.\d+)?$/.test(trimmed)) return trimmed;
    const m = trimmed.match(/^([+-]?\d+(?:\.\d+)?)\s*([a-zA-Z]+)$/);
    if (m) {
      const base = Number(m[1]);
      const suffix = m[2].toUpperCase();
      const expectsMB =
        field.name.toLowerCase().endsWith("mb") ||
        (field.label || "").toUpperCase().includes("MB") ||
        (field.envKey || "").toUpperCase() === "MEMORY";
      let num = base;
      if (suffix === "G" || suffix === "GI" || suffix === "GB" || suffix === "GIB") {
        num = expectsMB ? base * 1024 : base * 1024 * 1024 * 1024;
      } else if (suffix === "M" || suffix === "MI" || suffix === "MB" || suffix === "MIB") {
        num = expectsMB ? base : base * 1024 * 1024;
      } else if (suffix === "K" || suffix === "KI" || suffix === "KB" || suffix === "KIB") {
        num = expectsMB ? base / 1024 : base * 1024;
      } else {
        return field.default !== undefined ? String(field.default) : trimmed;
      }
      if (field.min !== undefined && num < field.min) return field.default !== undefined ? String(field.default) : String(field.min);
      if (field.max !== undefined && num > field.max) return field.default !== undefined ? String(field.default) : String(field.max);
      return String(Math.round(num * 100) / 100);
    }
    return field.default !== undefined ? String(field.default) : trimmed;
  }
  return trimmed;
}

export function defaultAppConfigValues(app?: AppItem): Record<string, string> {
  const out: Record<string, string> = {};
  for (const field of app?.configSchema || []) {
    if (field.default !== undefined) {
      out[field.name] = String(field.default);
    }
  }
  return out;
}

export function compileAppConfig(
  app: AppItem | undefined,
  version: string,
  configValues: Record<string, string>,
  baseEnv: Record<string, string>,
): {
  image: string;
  env: Record<string, string>;
  files: InstanceFile[];
  ports?: InstancePort[];
} {
  const env: Record<string, string> = { ...(app?.env || {}), ...baseEnv };
  let image = app?.image || "";
  const ver = version.trim() || app?.recommendedVersion || app?.versions?.[0] || "";
  if (ver && app?.imageTemplate) {
    const nextImage = app.imageTemplate
      .replace(/\{\{\s*VERSION\s*\}\}/gi, ver)
      .replace(/\{version\}/gi, ver);
    if (nextImage) image = nextImage;
  }
  if (ver && env.VERSION !== undefined) {
    env.VERSION = ver;
  }
  if (app?.preStartCommand?.trim()) {
    env.PRE_START_HOOK = app.preStartCommand
      .trim()
      .replace(/\{\{\s*Version\s*\}\}/gi, ver)
      .replace(/\{version\}/gi, ver);
  }
  if (app?.startupCommand?.trim()) {
    env.STARTUP_CMD = app.startupCommand
      .trim()
      .replace(/\{\{\s*Version\s*\}\}/gi, ver)
      .replace(/\{version\}/gi, ver);
  }
  if (app?.preStopCommand?.trim()) {
    env.PRE_STOP_HOOK = app.preStopCommand
      .trim()
      .replace(/\{\{\s*Version\s*\}\}/gi, ver)
      .replace(/\{version\}/gi, ver);
  }

  const mergedValues: Record<string, string> = {
    ...defaultAppConfigValues(app),
    ...configValues,
  };

  for (const field of app?.configSchema || []) {
    const rawVal = mergedValues[field.name];
    if (rawVal !== undefined) {
      const normalized = normalizeConfigFieldValue(field, rawVal);
      mergedValues[field.name] = normalized;
      if (field.envKey) {
        if (field.envKey.toUpperCase() === "MEMORY" && field.type === "number" && /^\d+$/.test(normalized)) {
          env[field.envKey] = `${normalized}M`;
        } else {
          env[field.envKey] = normalized;
        }
      }
    }
  }

  const files: InstanceFile[] = (app?.fileTemplates || []).map((tpl) => {
    const content = tpl.content.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (_, key: string) => {
      return mergedValues[key] !== undefined ? String(mergedValues[key]) : "";
    });
    return {
      path: tpl.path,
      content,
    };
  });

  return {
    image,
    env,
    files,
    ports: app?.ports,
  };
}

export function resolveServerArtifact(app: AppItem | undefined, version: string): ArtifactRef | undefined {
  const sb = app?.serverBinding;
  if (!sb?.repository || !sb?.package) return undefined;
  const ver = version.trim() || app?.recommendedVersion || app?.versions?.[0] || "latest";
  if (!ver) return undefined;
  const targetPath = sb.targetPath
    ? sb.targetPath.replace(/\{\{\s*Version\s*\}\}/gi, ver).replace(/\{version\}/gi, ver)
    : undefined;
  return {
    repository: sb.repository,
    package: sb.package,
    version: ver,
    ...(targetPath ? { targetPath } : {}),
  };
}
