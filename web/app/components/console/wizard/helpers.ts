import type { QuotaSummary } from "../../../console/ConsoleProvider";
import { parseNumericQuantity } from "../../../console/lib/format";
import { RECOMMENDED_PLAN_ID, RESOURCE_PLANS, type GamePreset, type ResourcePlan } from "../../../console/lib/presets";

export interface EnvRow {
  id: number;
  name: string;
  value: string;
}

export type PublishedPortMode = "auto" | "fixed";

export interface AdvancedSettings {
  region: string; // "" = auto-select
  publishedPortMode: PublishedPortMode;
  publishedPort: string; // 1..65535 when publishedPortMode === "fixed"
  env: EnvRow[];
  // Used only when creating from the "custom-container" BYO-image app:
  image: string;
  command: string;
  protocol: "TCP" | "UDP";
  portName: string;
  targetPort: string;
}

const REGION_LABELS: Record<string, string> = {
  "cn-east": "华东",
  "cn-east-1": "华东 1",
  "cn-south": "华南",
  "cn-south-1": "华南 1",
  "cn-north": "华北",
  "cn-north-1": "华北 1",
  "ap-east-1": "亚太香港",
  "ap-southeast-1": "亚太新加坡",
  "ap-northeast-1": "亚太东京",
  "us-west-1": "美西",
  "us-east-1": "美东",
  "eu-central-1": "欧洲法兰克福",
};

export function formatRegionLabel(region: string): string {
  const trimmed = region.trim();
  return REGION_LABELS[trimmed] || trimmed;
}

let envSeq = 0;
export const nextEnvId = () => ++envSeq;

export function findPlan(id?: string | null): ResourcePlan | undefined {
  return id ? RESOURCE_PLANS.find((p) => p.id === id) : undefined;
}

/** "palworld-prod-01" -> "palworld-prod-37" */
export function suggestName(preset: GamePreset): string {
  const base = preset.defaultName.replace(/-\d+$/, "");
  const suffix = String(10 + Math.floor(Math.random() * 90));
  return `${base}-${suffix}`;
}

export function advancedFromPreset(preset: GamePreset): AdvancedSettings {
  return {
    region: "",
    publishedPortMode: "auto",
    publishedPort: String(preset.targetPort || 25565),
    env: preset.env.map((e) => ({ id: nextEnvId(), name: e.name, value: e.value })),
    image: preset.image,
    command: preset.command,
    protocol: preset.protocol,
    portName: preset.portName,
    targetPort: String(preset.targetPort),
  };
}

/** Returns a reason string when the plan does not fit into the remaining quota. */
export function planQuotaIssue(plan: ResourcePlan, quota: QuotaSummary): string | null {
  const cpuLeft = quota.cpu.max - Math.max(quota.cpu.used, quota.cpu.reserved ?? 0);
  const memLeft = quota.memory.max - Math.max(quota.memory.used, quota.memory.reserved ?? 0);
  const diskLeft = quota.storage.max - Math.max(quota.storage.used, quota.storage.reserved ?? 0);
  const eps = 1e-6;
  if (parseNumericQuantity(plan.cpu) > cpuLeft + eps) return "CPU";
  if (parseNumericQuantity(plan.memory) > memLeft + eps) return "内存";
  if (parseNumericQuantity(plan.storage) > diskLeft + eps) return "存储";
  return null;
}

/** Plan matching the preset's own sizing, else recommended, else the first plan that fits. */
export function defaultPlanId(preset: GamePreset | undefined, quota: QuotaSummary): string | null {
  const fitting = RESOURCE_PLANS.filter((p) => !planQuotaIssue(p, quota));
  if (fitting.length === 0) return null;
  const recId = recommendedPlanId(preset);
  const recIdx = RESOURCE_PLANS.findIndex((p) => p.id === recId);
  if (recIdx < 0) {
    return fitting.find((p) => p.id === RECOMMENDED_PLAN_ID)?.id ?? fitting[0].id;
  }
  // Recommended tier if it fits, otherwise the largest fitting tier below it.
  const atOrBelow = fitting.filter((p) => RESOURCE_PLANS.indexOf(p) <= recIdx);
  return (atOrBelow[atOrBelow.length - 1] ?? fitting[0]).id;
}

/** Smallest tier that meets the game's own recommended CPU and memory. */
export function recommendedPlanId(preset: GamePreset | undefined): string | null {
  if (!preset) return null;
  const cpu = parseNumericQuantity(preset.cpu);
  const mem = parseNumericQuantity(preset.memory);
  const tier = RESOURCE_PLANS.find(
    (p) => parseNumericQuantity(p.cpu) >= cpu && parseNumericQuantity(p.memory) >= mem,
  );
  return tier?.id ?? RESOURCE_PLANS[RESOURCE_PLANS.length - 1].id;
}

/** True when the tier is smaller than the game's recommended CPU or memory. */
export function belowRecommended(plan: ResourcePlan, preset: GamePreset | undefined): boolean {
  if (!preset) return false;
  return (
    parseNumericQuantity(plan.cpu) < parseNumericQuantity(preset.cpu) ||
    parseNumericQuantity(plan.memory) < parseNumericQuantity(preset.memory)
  );
}

const NAME_RE = /^[a-z][a-z0-9-]*[a-z0-9]$/;

export function validateName(name: string, taken: string[]): string | null {
  const n = name.trim();
  if (!n) return "请填写服务器名称";
  if (n.length < 3 || n.length > 40) return "名称长度需为 3–40 个字符";
  if (!/^[a-z]/.test(n)) return "名称需以小写字母开头";
  if (!NAME_RE.test(n)) return "只能使用小写字母、数字和 -，且不能以 - 结尾";
  if (taken.includes(n)) return "当前工作区已有同名服务器，请换一个名称";
  return null;
}

export function validatePort(v: string, allowZero: boolean): string | null {
  if (!/^\d+$/.test(v.trim())) return "请输入数字端口";
  const n = Number(v);
  if (allowZero && n === 0) return null;
  if (n < 1 || n > 65535) return allowZero ? "端口范围 1–65535，或填 0 自动分配" : "端口范围 1–65535";
  return null;
}
