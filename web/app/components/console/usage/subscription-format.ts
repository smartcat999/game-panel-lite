import { formatCpu, formatGB, parseNumericQuantity } from "../../../console/lib/format";
import type { QuotaOrderItem, ResourcePlanPrice, TenantQuota } from "../../../console/lib/types";

export type BadgeTone = "neutral" | "accent" | "success" | "warning" | "danger";

export const SUBSCRIPTION_STATE_META: Record<string, { label: string; tone: BadgeTone }> = {
  Trialing: { label: "试用中", tone: "accent" },
  Active: { label: "已激活", tone: "success" },
  Grace: { label: "宽限期", tone: "warning" },
  Suspended: { label: "已过宽限期", tone: "danger" },
  None: { label: "未开通", tone: "neutral" },
};

export const ORDER_STATE_META: Record<string, { label: string; tone: BadgeTone }> = {
  Paid: { label: "已生效", tone: "success" },
  Pending: { label: "待支付", tone: "warning" },
  Cancelled: { label: "已撤销", tone: "neutral" },
};

export const ORDER_SOURCE_LABEL: Record<string, string> = {
  Trial: "新手试用",
  Redeem: "兑换码",
  AdminGrant: "管理员发放",
  Checkout: "在线购买",
};

export type QuotaDimension = keyof TenantQuota;

export const QUOTA_DIMENSIONS: Array<{ key: QuotaDimension; label: string; unit: string }> = [
  { key: "cpu", label: "CPU", unit: "核" },
  { key: "memory", label: "内存", unit: "GB" },
  { key: "storage", label: "存储", unit: "GB" },
  { key: "instances", label: "服务器", unit: "台" },
  { key: "backups", label: "备份", unit: "份" },
  { key: "backupStorage", label: "备份空间", unit: "GB" },
];

export const DAY_MS = 24 * 60 * 60 * 1000;

export const trimNum = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));

/** Numeric value of a quota dimension in display units (cores, GB, count). */
export function quotaValue(q: TenantQuota | undefined, key: QuotaDimension): number {
  return parseNumericQuantity(q?.[key] || "0");
}

export function formatQuotaValue(q: TenantQuota | undefined, key: QuotaDimension): string {
  const raw = q?.[key] || "0";
  if (key === "cpu") return formatCpu(raw);
  if (key === "memory" || key === "storage" || key === "backupStorage") return formatGB(raw);
  return `${trimNum(parseNumericQuantity(raw))} ${key === "instances" ? "台" : "份"}`;
}

export function formatPrice(price: ResourcePlanPrice | undefined): { amount: string; cycle: string } | null {
  if (!price) return null;
  const cycle = `/ ${price.durationDays} 天`;
  if (!price.amountCents) return { amount: "免费", cycle };
  const symbol = price.currency === "USD" ? "$" : "¥";
  const cents = price.amountCents;
  return { amount: `${symbol}${(cents / 100).toFixed(cents % 100 === 0 ? 0 : 2)}`, cycle };
}

/** "trial:4599dcf8-…-c223459730c0" -> "trial…30c0" */
export function shortOrderId(id: string): string {
  if (id.length <= 12) return id;
  const prefix = /^([a-z]+)[:-]/i.exec(id)?.[1] ?? "";
  return `${prefix}…${id.slice(-4)}`;
}

export function orderSourceText(order: QuotaOrderItem, planName: string): string {
  return `${planName} · ${ORDER_SOURCE_LABEL[order.source] || order.source} · ${order.durationDays} 天`;
}

export function daysUntil(iso: string | undefined, now: number): number | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return null;
  return Math.max(0, Math.ceil((t - now) / DAY_MS));
}
