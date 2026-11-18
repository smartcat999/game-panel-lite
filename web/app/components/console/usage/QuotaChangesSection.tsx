"use client";

import { Fragment, useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatDateTime } from "../../../console/lib/format";
import type {
  QuotaOrderItem,
  QuotaPolicyChangeItem,
  QuotaPolicySnapshot,
  ResourcePlanItem,
} from "../../../console/lib/types";
import { Badge, Button, CardHeader, CopyButton, EmptyState, Mono, Skeleton } from "../ui";
import {
  ORDER_SOURCE_LABEL,
  QUOTA_DIMENSIONS,
  formatQuotaValue,
  orderSourceText,
  quotaValue,
  shortOrderId,
  type QuotaDimension,
} from "./subscription-format";

type Tone = "neutral" | "accent" | "success" | "warning" | "danger";

const MAX_DIFF_DIMS = 3;

function dimText(snap: QuotaPolicySnapshot | undefined, key: QuotaDimension): string {
  if (snap?.unlimitedQuota?.includes(key)) return "不限";
  return formatQuotaValue(snap?.quota, key);
}

function isZero(snap: QuotaPolicySnapshot | undefined): boolean {
  return QUOTA_DIMENSIONS.every((d) => !snap?.unlimitedQuota?.includes(d.key) && quotaValue(snap?.quota, d.key) === 0);
}

function compareDims(c: QuotaPolicyChangeItem): { up: boolean; down: boolean } {
  let up = false;
  let down = false;
  for (const d of QUOTA_DIMENSIONS) {
    const a = quotaValue(c.old?.quota, d.key);
    const b = quotaValue(c.target?.quota, d.key);
    up = up || b > a;
    down = down || b < a;
  }
  return { up, down };
}

/** Orders newly applied by this change carry policyVersion === change.toVersion. */
function triggerOrders(c: QuotaPolicyChangeItem, orders: Map<string, QuotaOrderItem>): QuotaOrderItem[] {
  return (c.orderIDs || [])
    .map((id) => orders.get(id))
    .filter((o): o is QuotaOrderItem => !!o && o.policyVersion === c.toVersion);
}

function subscriptionReason(c: QuotaPolicyChangeItem, trigger?: QuotaOrderItem): { label: string; tone: Tone } {
  const { up, down } = compareDims(c);
  if (!trigger) return down && !up ? { label: "配额回落", tone: "neutral" } : { label: "配额同步", tone: "neutral" };
  if (trigger.source === "Trial") return { label: "试用开通", tone: "accent" };
  if (trigger.orderKind === "AddonPack") return { label: "扩容叠加", tone: "accent" };
  if (isZero(c.old)) return { label: "套餐激活", tone: "success" };
  if (up && !down) return { label: "升级", tone: "success" };
  if (down && !up) return { label: "降级", tone: "neutral" };
  if (!up && !down) return { label: "续期", tone: "success" };
  return { label: "套餐变更", tone: "accent" };
}

function changeReason(c: QuotaPolicyChangeItem, triggers: QuotaOrderItem[]): { label: string; tone: Tone } {
  switch (c.changeKind) {
    case "Bootstrap":
      if (triggers.some((o) => o.source === "Trial")) return { label: "试用开通", tone: "accent" };
      return isZero(c.target) ? { label: "工作区开通", tone: "neutral" } : { label: "初始配额", tone: "neutral" };
    case "Expiry":
      return { label: "到期回收", tone: "warning" };
    case "AdminAdjust":
      return { label: "管理员调整", tone: "accent" };
    default:
      return subscriptionReason(c, triggers[0]);
  }
}

function changeSummary(c: QuotaPolicyChangeItem): string {
  if (c.fromVersion === 0) {
    const dims = QUOTA_DIMENSIONS.filter((d) => quotaValue(c.target?.quota, d.key) > 0 || c.target?.unlimitedQuota?.includes(d.key));
    if (dims.length === 0) return "初始配额为 0";
    return dims.slice(0, MAX_DIFF_DIMS).map((d) => `${d.label} ${dimText(c.target, d.key)}`).join(" · ");
  }
  if (c.changeKind === "Expiry" && isZero(c.target)) return "宽限期结束，配额回收为 0";
  const changed = QUOTA_DIMENSIONS.filter((d) => dimText(c.old, d.key) !== dimText(c.target, d.key));
  if (changed.length === 0) {
    const from = c.old?.expiresAt;
    const to = c.target?.expiresAt;
    return from !== to && to ? `配额不变 · 到期 ${formatDateTime(from)} → ${formatDateTime(to)}` : "配额不变";
  }
  const head = changed
    .slice(0, MAX_DIFF_DIMS)
    .map((d) => `${d.label} ${dimText(c.old, d.key)} → ${dimText(c.target, d.key)}`)
    .join(" · ");
  return changed.length > MAX_DIFF_DIMS ? `${head} 等 ${changed.length} 项` : head;
}

function chunk<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) out.push(items.slice(i, i + size));
  return out;
}

/** Resolve referenced orders and plan display names with batch calls on their own resource endpoints. */
function useLinkedResources(tenant: string, changes: QuotaPolicyChangeItem[]) {
  const { apiFetch } = useConsole();
  const [orders, setOrders] = useState<Map<string, QuotaOrderItem>>(new Map());
  const [planNames, setPlanNames] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    const ids = [...new Set(changes.flatMap((c) => c.orderIDs || []))];
    if (!tenant || ids.length === 0) return;
    let cancelled = false;
    void (async () => {
      const found = new Map<string, QuotaOrderItem>();
      for (const part of chunk(ids, 100)) {
        const selector = encodeURIComponent(`metadata.name.in=${part.join(",")}`);
        const res = await apiFetch(
          `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/orders?limit=${part.length}&fieldSelector=${selector}`,
        ).catch(() => null);
        if (!res?.ok) continue;
        const data = await res.json();
        for (const o of (data?.items || []) as QuotaOrderItem[]) found.set(o.id, o);
      }
      if (cancelled) return;
      setOrders(found);
      const plans = [...new Set([...found.values()].map((o) => o.planName))];
      if (plans.length === 0) return;
      const selector = encodeURIComponent(`metadata.name.in=${plans.join(",")}`);
      const res = await apiFetch(`/apis/core.gamepanel.io/v1/resourceplans?fieldSelector=${selector}`).catch(() => null);
      if (!res?.ok || cancelled) return;
      const data = await res.json();
      setPlanNames(new Map(((data?.items || []) as ResourcePlanItem[]).map((p) => [p.metadata.name, p.displayName])));
    })();
    return () => {
      cancelled = true;
    };
  }, [apiFetch, tenant, changes]);

  return { orders, planNames };
}

export function QuotaChangesSection({
  tenant,
  changes,
  total,
  loading,
  page,
  pageSize,
  onPageChange,
}: {
  tenant: string;
  changes: QuotaPolicyChangeItem[];
  total: number;
  loading: boolean;
  page: number;
  pageSize: number;
  onPageChange: (next: number) => void;
}) {
  const { orders, planNames } = useLinkedResources(tenant, changes);
  const [expanded, setExpanded] = useState("");
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, totalPages);
  const planLabel = (name: string) => planNames.get(name) || name;

  return (
    <>
      <CardHeader title="变更记录" description={total > 0 ? `共 ${total} 条` : undefined} />
      {loading && changes.length === 0 ? (
        <div className="space-y-3 p-6">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-12" />
          ))}
        </div>
      ) : changes.length === 0 ? (
        <EmptyState
          image="/landing/features/console.jpg"
          title="暂无变更记录"
          body="开通、兑换、续期或到期后，工作区配额的每次变化都会记录在这里。"
        />
      ) : (
        <>
          <div tabIndex={0} role="region" aria-label="变更记录" className="overflow-x-auto">
            <table className="w-full min-w-[820px] text-left">
              <thead>
                <tr className="border-b border-[#EEF0F7] text-[13px] text-[#5B6075]">
                  <th scope="col" className="px-6 py-3 font-medium">时间</th>
                  <th scope="col" className="px-4 py-3 font-medium">原因</th>
                  <th scope="col" className="px-4 py-3 font-medium">配额变化</th>
                  <th scope="col" className="px-4 py-3 font-medium">来源</th>
                  <th scope="col" className="px-6 py-3 font-medium">有效期</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#EEF0F7]">
                {changes.map((c) => (
                  <ChangeRow
                    key={c.metadata.uid || c.metadata.name}
                    change={c}
                    orders={orders}
                    planLabel={planLabel}
                    open={expanded === c.metadata.name}
                    onToggle={() => setExpanded((cur) => (cur === c.metadata.name ? "" : c.metadata.name))}
                  />
                ))}
              </tbody>
            </table>
          </div>
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-[#EEF0F7] px-6 py-3 text-[13px] text-[#5B6075]">
              <span className="tabular-nums">
                第 {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, total)} 条，共 {total} 条
              </span>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={currentPage <= 1 || loading}
                  icon={<ChevronLeft className="h-4 w-4" />}
                  onClick={() => onPageChange(Math.max(1, currentPage - 1))}
                >
                  上一页
                </Button>
                <span className="px-2 font-medium tabular-nums text-[#14161F]">
                  {currentPage} / {totalPages}
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={currentPage >= totalPages || loading}
                  onClick={() => onPageChange(Math.min(totalPages, currentPage + 1))}
                >
                  下一页
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </>
  );
}

function ChangeRow({
  change: c,
  orders,
  planLabel,
  open,
  onToggle,
}: {
  change: QuotaPolicyChangeItem;
  orders: Map<string, QuotaOrderItem>;
  planLabel: (name: string) => string;
  open: boolean;
  onToggle: () => void;
}) {
  const triggers = useMemo(() => triggerOrders(c, orders), [c, orders]);
  const reason = changeReason(c, triggers);
  const linked = (c.orderIDs || []).map((id) => orders.get(id)).filter((o): o is QuotaOrderItem => !!o);
  const expiredBase = c.changeKind === "Expiry" ? linked.find((o) => o.orderKind === "BasePlan") : undefined;
  const sourceOrder = triggers[0] || expiredBase;

  return (
    <Fragment>
      <tr
        className="cursor-pointer hover:bg-[#FAFBFE]"
        onClick={onToggle}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onToggle();
          }
        }}
        tabIndex={0}
        aria-expanded={open}
      >
        <td className="whitespace-nowrap px-6 py-4 text-[13px] tabular-nums text-[#5B6075]">
          <span className="inline-flex items-center gap-2">
            <ChevronDown className={`h-4 w-4 text-[#8A90A6] transition-transform ${open ? "rotate-180" : ""}`} aria-hidden />
            {formatDateTime(c.metadata.creationTimestamp)}
          </span>
        </td>
        <td className="px-4 py-4">
          <Badge tone={reason.tone}>{reason.label}</Badge>
        </td>
        <td className="px-4 py-4 text-[14px] tabular-nums text-[#14161F]">{changeSummary(c)}</td>
        <td className="px-4 py-4 text-[13px] text-[#3D4258]">
          {sourceOrder ? (
            <>
              <div>
                {expiredBase && !triggers[0] ? "到期：" : ""}
                {orderSourceText(sourceOrder, planLabel(sourceOrder.planName))}
              </div>
              {triggers.length > 1 && <div className="mt-1 text-[12px] text-[#8A90A6]">另含 {triggers.length - 1} 笔订单</div>}
            </>
          ) : (
            <span className="text-[#8A90A6]">—</span>
          )}
        </td>
        <td className="whitespace-nowrap px-6 py-4 text-[13px] tabular-nums text-[#5B6075]">
          {c.target?.expiresAt ? `至 ${formatDateTime(c.target.expiresAt)}` : "—"}
        </td>
      </tr>
      {open && (
        <tr className="bg-[#F7F8FC]">
          <td colSpan={5} className="px-6 py-4">
            <ChangeDetail change={c} linked={linked} planLabel={planLabel} />
          </td>
        </tr>
      )}
    </Fragment>
  );
}

function ChangeDetail({
  change: c,
  linked,
  planLabel,
}: {
  change: QuotaPolicyChangeItem;
  linked: QuotaOrderItem[];
  planLabel: (name: string) => string;
}) {
  return (
    <dl className="grid gap-x-8 gap-y-3 text-[13px] sm:grid-cols-[96px_1fr]">
      <dt className="text-[#8A90A6]">变更后配额</dt>
      <dd className="tabular-nums text-[#14161F]">
        {QUOTA_DIMENSIONS.map((d) => `${d.label} ${dimText(c.target, d.key)}`).join(" · ")}
      </dd>
      <dt className="text-[#8A90A6]">账本版本</dt>
      <dd>
        <Mono>
          v{c.fromVersion} → v{c.toVersion}
        </Mono>
      </dd>
      <dt className="text-[#8A90A6]">关联订单</dt>
      <dd className="space-y-2">
        {linked.length === 0 ? (
          <span className="text-[#8A90A6]">无</span>
        ) : (
          linked.map((o) => (
            <div key={o.id} className="flex flex-wrap items-center gap-2">
              <span className="text-[#14161F]">
                {planLabel(o.planName)} · {ORDER_SOURCE_LABEL[o.source] || o.source} · {o.durationDays} 天
                {o.policyVersion === c.toVersion ? "（本次生效）" : ""}
              </span>
              <Mono>{shortOrderId(o.id)}</Mono>
              <CopyButton text={o.id} label="复制订单号" />
            </div>
          ))
        )}
      </dd>
    </dl>
  );
}
