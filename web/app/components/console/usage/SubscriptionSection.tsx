"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatDateTime } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import type { ResourcePlanItem, Tenant, TenantQuota } from "../../../console/lib/types";
import { Badge, Button, Input, Skeleton } from "../ui";
import {
  SUBSCRIPTION_STATE_META,
  daysUntil,
  formatPrice,
  formatQuotaValue,
  quotaValue,
  trimNum,
  type QuotaDimension,
} from "./subscription-format";

const COMPARE_ROWS: Array<{ key: QuotaDimension; label: string }> = [
  { key: "cpu", label: "CPU" },
  { key: "memory", label: "内存" },
  { key: "storage", label: "存储" },
  { key: "instances", label: "服务器" },
];

function sortPlans(items: ResourcePlanItem[]): ResourcePlanItem[] {
  return [...items].sort(
    (a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0) || quotaValue(a.quota, "cpu") - quotaValue(b.quota, "cpu"),
  );
}

function AddonPlus({ addon, dim }: { addon?: TenantQuota; dim: QuotaDimension }) {
  const v = quotaValue(addon, dim);
  if (v <= 0) return null;
  return <span className="ml-1 text-[12px] font-medium text-[#2B3AE8]">+{trimNum(v)}</span>;
}

export function SubscriptionSection({
  tenant,
  tenantObj,
  canManageTenant,
}: {
  tenant: string;
  tenantObj: Tenant | undefined;
  canManageTenant: boolean;
}) {
  const { apiFetch, toast, reload } = useConsole();
  const [plans, setPlans] = useState<ResourcePlanItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [code, setCode] = useState("");
  const [target, setTarget] = useState("");
  const [redeeming, setRedeeming] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [now] = useState(() => Date.now());

  const loadPlans = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch("/apis/core.gamepanel.io/v1/resourceplans?fieldSelector=published=true");
      if (res.ok) {
        const data = await res.json();
        setPlans(Array.isArray(data?.items) ? data.items : []);
      }
    } catch {
      /* transient fetch failure keeps previous catalog */
    } finally {
      setLoading(false);
    }
  }, [apiFetch]);

  useEffect(() => {
    void loadPlans();
  }, [loadPlans]);

  async function submitRedeem(e: FormEvent) {
    e.preventDefault();
    const trimmed = code.trim();
    if (!trimmed) {
      inputRef.current?.focus();
      return;
    }
    setRedeeming(true);
    try {
      const res = await apiFetch(`/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/redeem`, {
        method: "POST",
        body: JSON.stringify({ code: trimmed }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "兑换失败，请检查兑换码是否有效"));
      toast("success", "兑换成功，工作区配额已更新");
      setCode("");
      setTarget("");
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "兑换失败");
    } finally {
      setRedeeming(false);
    }
  }

  function pickTarget(name: string) {
    setTarget(name);
    inputRef.current?.focus();
  }

  const isSubscription = tenantObj?.spec?.billingMode === "Subscription";
  const sub = tenantObj?.status?.subscription;
  const state = sub?.state && sub.planName ? sub.state : "None";
  const basePlans = useMemo(() => sortPlans(plans.filter((p) => p.planKind !== "Addon")), [plans]);
  const addonPlans = useMemo(() => sortPlans(plans.filter((p) => p.planKind === "Addon")), [plans]);

  if (!isSubscription) {
    return (
      <div className="p-6">
        <div className="text-[16px] font-semibold text-[#14161F]">手动配额模式</div>
        <p className="mt-1 text-[13px] text-[#5B6075]">当前工作区的配额由平台管理员统一设置，不支持兑换套餐。</p>
      </div>
    );
  }

  return (
    <>
      <StatusStrip state={state} expiresAt={sub?.expiresAt} graceUntil={sub?.graceUntil} />
      <div className="flex flex-wrap items-end justify-between gap-6 border-b border-[#EEF0F7] p-6">
        <StatusHero
          state={state}
          planName={sub?.planDisplayName || sub?.planName || ""}
          expiresAt={sub?.expiresAt}
          graceUntil={sub?.graceUntil}
          hasAddon={quotaValue(sub?.addonQuota, "cpu") + quotaValue(sub?.addonQuota, "storage") > 0}
          now={now}
        />
        {canManageTenant && (
          <form onSubmit={(e) => void submitRedeem(e)} className="flex w-full flex-col gap-2 sm:w-auto">
            <label htmlFor="redeem-code" className="text-[13px] font-medium text-[#3D4258]">
              兑换码
            </label>
            <div className="flex gap-2">
              <Input
                id="redeem-code"
                ref={inputRef}
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder={target ? `输入「${target}」兑换码` : "GP-XXXX-XXXX-XXXX"}
                autoComplete="off"
                className="w-full font-mono text-[13px] uppercase sm:w-[260px]"
              />
              <Button type="submit" variant="primary" loading={redeeming} disabled={redeeming}>
                兑换
              </Button>
            </div>
          </form>
        )}
      </div>

      {loading && plans.length === 0 ? (
        <div className="grid gap-3 p-6 sm:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-48" />
          ))}
        </div>
      ) : basePlans.length === 0 ? (
        <p className="px-6 py-8 text-[13px] text-[#8A90A6]">暂无已发布的套餐。</p>
      ) : (
        <PlanCompareTable
          plans={basePlans}
          currentPlan={state === "None" ? "" : sub?.planName || ""}
          expired={state === "Suspended" || state === "Grace"}
          addonQuota={sub?.addonQuota}
          onPick={canManageTenant ? pickTarget : undefined}
        />
      )}

      {addonPlans.length > 0 && <AddonList plans={addonPlans} onPick={canManageTenant ? pickTarget : undefined} />}
    </>
  );
}

function StatusStrip({ state, expiresAt, graceUntil }: { state: string; expiresAt?: string; graceUntil?: string }) {
  if (state === "Grace") {
    return (
      <div role="status" className="border-b border-[#FEDF89] bg-[#FFFAEB] px-6 py-3 text-[13px] text-[#B54708]">
        套餐已于 {formatDateTime(expiresAt)} 到期，服务器已停服保号。{formatDateTime(graceUntil)}{" "}
        前兑换任意基础套餐即可自动恢复运行，逾期资源将被回收。
      </div>
    );
  }
  if (state === "Suspended") {
    return (
      <div role="status" className="border-b border-[#FECDCA] bg-[#FEF3F2] px-6 py-3 text-[13px] text-[#B42318]">
        宽限期已于 {formatDateTime(graceUntil)} 结束，服务器资源等待回收。兑换基础套餐可恢复配额。
      </div>
    );
  }
  return null;
}

function StatusHero({
  state,
  planName,
  expiresAt,
  graceUntil,
  hasAddon,
  now,
}: {
  state: string;
  planName: string;
  expiresAt?: string;
  graceUntil?: string;
  hasAddon: boolean;
  now: number;
}) {
  const meta = SUBSCRIPTION_STATE_META[state] || { label: state, tone: "neutral" as const };
  let headline: ReactNode;
  if (state === "None") {
    headline = <b className="text-[28px] font-bold leading-none tracking-tight text-[#14161F]">未开通</b>;
  } else if (state === "Suspended") {
    headline = <b className="text-[28px] font-bold leading-none tracking-tight text-[#B42318]">已过期</b>;
  } else {
    const grace = state === "Grace";
    const days = daysUntil(grace ? graceUntil : expiresAt, now) ?? 0;
    headline = (
      <>
        <b className={`text-[28px] font-bold leading-none tracking-tight ${grace ? "text-[#B54708]" : "text-[#14161F]"}`}>
          {days}
        </b>
        <span className="text-[14px] text-[#5B6075]">{grace ? "天宽限期剩余" : "天后到期"}</span>
      </>
    );
  }
  return (
    <div>
      <div className="flex items-baseline gap-2 tabular-nums">{headline}</div>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {state === "None" ? (
          <span className="text-[13px] text-[#5B6075]">兑换基础套餐后即可创建服务器</span>
        ) : (
          <>
            <span className="text-[16px] font-semibold text-[#14161F]">{planName}</span>
            <Badge tone={meta.tone}>{meta.label}</Badge>
            <span className="ml-2 text-[13px] tabular-nums text-[#8A90A6]">
              {hasAddon ? "含扩容包 · " : ""}到期时间 {formatDateTime(expiresAt)}
            </span>
          </>
        )}
      </div>
    </div>
  );
}

function PlanCompareTable({
  plans,
  currentPlan,
  expired,
  addonQuota,
  onPick,
}: {
  plans: ResourcePlanItem[];
  currentPlan: string;
  expired: boolean;
  addonQuota?: TenantQuota;
  onPick?: (name: string) => void;
}) {
  const cur = (p: ResourcePlanItem) => p.metadata.name === currentPlan;
  const tint = (p: ResourcePlanItem) => (cur(p) ? "bg-[#F6F8FF]" : "");
  const showAddon = (p: ResourcePlanItem) => cur(p) && !expired;
  return (
    <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="套餐对比">
      <table className="w-full min-w-[760px] table-fixed border-collapse text-left">
        <colgroup>
          <col className="w-32" />
          {plans.map((p) => (
            <col key={p.metadata.name} />
          ))}
        </colgroup>
        <thead>
          <tr className="border-b border-[#EEF0F7]">
            <th scope="col" className="px-6 pb-4 pt-6" />
            {plans.map((p) => {
              const price = formatPrice(p.prices?.[0]);
              return (
                <th
                  key={p.metadata.name}
                  scope="col"
                  className={`px-4 pb-4 pt-6 align-top font-normal ${tint(p)} ${cur(p) ? "shadow-[inset_0_2px_0_#2B3AE8]" : ""}`}
                >
                  <div className="flex items-center gap-2 text-[16px] font-semibold text-[#14161F]">
                    {p.displayName}
                    {cur(p) && (
                      <span className={`text-[12px] font-medium ${expired ? "text-[#B54708]" : "text-[#2B3AE8]"}`}>
                        {expired ? "已到期" : "当前"}
                      </span>
                    )}
                  </div>
                  <div className="mt-1 truncate text-[12px] text-[#8A90A6]" title={p.description}>
                    {p.description || p.metadata.name}
                  </div>
                  {price && (
                    <div className="mt-3 flex items-baseline gap-1 tabular-nums">
                      <span className="text-[20px] font-bold text-[#14161F]">{price.amount}</span>
                      <span className="text-[12px] text-[#8A90A6]">{price.cycle}</span>
                    </div>
                  )}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody className="divide-y divide-[#EEF0F7]">
          {COMPARE_ROWS.map((row) => (
            <tr key={row.key}>
              <th scope="row" className="px-6 py-3 text-[13px] font-normal text-[#5B6075]">
                {row.label}
              </th>
              {plans.map((p) => (
                <td key={p.metadata.name} className={`px-4 py-3 text-[14px] font-medium tabular-nums text-[#14161F] ${tint(p)}`}>
                  {formatQuotaValue(p.quota, row.key)}
                  {showAddon(p) && <AddonPlus addon={addonQuota} dim={row.key} />}
                </td>
              ))}
            </tr>
          ))}
          <tr>
            <th scope="row" className="px-6 py-3 text-[13px] font-normal text-[#5B6075]">
              备份
            </th>
            {plans.map((p) => (
              <td key={p.metadata.name} className={`px-4 py-3 text-[14px] font-medium tabular-nums text-[#14161F] ${tint(p)}`}>
                {formatQuotaValue(p.quota, "backups")} · {formatQuotaValue(p.quota, "backupStorage")}
                {showAddon(p) && <AddonPlus addon={addonQuota} dim="backups" />}
              </td>
            ))}
          </tr>
        </tbody>
        {onPick && (
          <tfoot>
            <tr>
              <td />
              {plans.map((p) => (
                <td key={p.metadata.name} className={`px-4 pb-6 pt-4 ${tint(p)}`}>
                  <Button
                    size="sm"
                    variant={cur(p) ? "primary" : "secondary"}
                    className="w-full"
                    onClick={() => onPick(p.displayName)}
                  >
                    兑换此档
                  </Button>
                </td>
              ))}
            </tr>
          </tfoot>
        )}
      </table>
    </div>
  );
}

function AddonList({ plans, onPick }: { plans: ResourcePlanItem[]; onPick?: (name: string) => void }) {
  const spec = (q: TenantQuota) =>
    (["cpu", "memory", "storage", "instances", "backups", "backupStorage"] as QuotaDimension[])
      .filter((k) => quotaValue(q, k) > 0)
      .map((k) => `+${formatQuotaValue(q, k)}`)
      .join(" · ");
  return (
    <div className="border-t border-[#EEF0F7] pb-3">
      <div className="px-6 pb-2 pt-4 text-[14px] font-semibold text-[#14161F]">扩容包</div>
      <ul className="divide-y divide-[#EEF0F7]">
        {plans.map((p) => {
          const price = formatPrice(p.prices?.[0]);
          return (
            <li
              key={p.metadata.name}
              className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1 px-6 py-3 md:grid-cols-[160px_1fr_120px_120px]"
            >
              <span className="text-[14px] font-medium text-[#14161F]">{p.displayName}</span>
              <span className="order-3 col-span-2 text-[13px] tabular-nums text-[#5B6075] md:order-none md:col-span-1">
                {spec(p.quota)}
              </span>
              <span className="text-right text-[14px] font-semibold tabular-nums text-[#14161F]">
                {price ? (
                  <>
                    {price.amount} <span className="text-[12px] font-normal text-[#8A90A6]">{price.cycle}</span>
                  </>
                ) : (
                  "—"
                )}
              </span>
              {onPick && (
                <Button
                  size="sm"
                  variant="secondary"
                  className="order-4 col-span-2 w-full md:order-none md:col-span-1"
                  onClick={() => onPick(p.displayName)}
                >
                  兑换此包
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
