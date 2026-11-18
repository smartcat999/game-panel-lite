"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Ban, Gift, Ticket } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatDateTime } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import type {
  QuotaOrderItem,
  RedeemCodeBatchResponse,
  RedeemCodeItem,
  ResourcePlanItem,
  Tenant,
} from "../../../console/lib/types";
import {
  Badge,
  Button,
  CopyButton,
  Field,
  Input,
  Modal,
  Mono,
  Select,
  Skeleton,
} from "../ui";
import { ORDER_SOURCE_LABEL, ORDER_STATE_META } from "../usage/subscription-format";

function usePlansList(open: boolean) {
  const { apiFetch } = useConsole();
  const [plans, setPlans] = useState<ResourcePlanItem[]>([]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    void apiFetch("/apis/core.gamepanel.io/v1/resourceplans")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (!active || !data) return;
        setPlans(Array.isArray(data.items) ? data.items : []);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [apiFetch, open]);

  return plans;
}

export function RedeemCodeBatchModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { apiFetch, toast } = useConsole();
  const plans = usePlansList(open);
  const [planName, setPlanName] = useState("pro");
  const [durationDays, setDurationDays] = useState("30");
  const [count, setCount] = useState("5");
  const [batchName, setBatchName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [createdBatch, setCreatedBatch] = useState<RedeemCodeBatchResponse | null>(null);
  const [codes, setCodes] = useState<RedeemCodeItem[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (plans.length > 0 && !plans.some((p) => p.metadata.name === planName)) {
      setPlanName(plans[0].metadata.name);
    }
  }, [plans, planName]);

  const loadCodes = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch("/apis/core.gamepanel.io/v1/redeemcodes?limit=30");
      if (res.ok) {
        const data = await res.json();
        setCodes(Array.isArray(data?.items) ? data.items : []);
      }
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, [apiFetch]);

  useEffect(() => {
    if (open) {
      setCreatedBatch(null);
      void loadCodes();
    }
  }, [open, loadCodes]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const days = Number(durationDays);
    const cnt = Number(count);
    if (!Number.isInteger(days) || days < 1 || !Number.isInteger(cnt) || cnt < 1 || cnt > 100) {
      toast("error", "请输入有效的天数 (>=1) 和生成数量 (1-100)");
      return;
    }
    setSubmitting(true);
    try {
      const res = await apiFetch("/apis/core.gamepanel.io/v1/redeemcodes", {
        method: "POST",
        body: JSON.stringify({
          planName,
          durationDays: days,
          count: cnt,
          batchName: batchName.trim() || undefined,
        }),
      });
      if (!res.ok) {
        throw new Error(await errorMessage(res, "生成兑换码失败"));
      }
      const batch: RedeemCodeBatchResponse = await res.json();
      setCreatedBatch(batch);
      toast("success", `已生成 ${batch.count} 个 ${batch.planName} 兑换码`);
      await loadCodes();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "生成兑换码失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="订阅兑换码管理"
      description="明文兑换码仅在本次生成时显示一次，系统后台仅保存不可逆哈希。"
      width="max-w-2xl"
      footer={
        <Button variant="ghost" onClick={onClose}>
          关闭
        </Button>
      }
    >
      <div className="space-y-5">
        <form onSubmit={(e) => void submit(e)} className="grid gap-3 rounded-2xl bg-[#F7F8FC] p-4 sm:grid-cols-4">
          <Field label="套餐规格" htmlFor="rc-plan">
            <Select id="rc-plan" name="planName" value={planName} onChange={(e) => setPlanName(e.target.value)}>
              {(plans.length > 0
                ? plans
                : [
                    { metadata: { name: "starter" }, displayName: "体验版 Starter" },
                    { metadata: { name: "standard" }, displayName: "标准版 Standard" },
                    { metadata: { name: "pro" }, displayName: "专业版 Pro" },
                    { metadata: { name: "team" }, displayName: "团队版 Team" },
                  ]
              ).map((p) => (
                <option key={p.metadata.name} value={p.metadata.name}>
                  {p.displayName} ({p.metadata.name})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="有效天数" htmlFor="rc-days">
            <Input
              id="rc-days"
              name="durationDays"
              type="text"
              inputMode="numeric"
              value={durationDays}
              onChange={(e) => setDurationDays(e.target.value)}
            />
          </Field>
          <Field label="数量" htmlFor="rc-count">
            <Input
              id="rc-count"
              name="count"
              type="text"
              inputMode="numeric"
              value={count}
              onChange={(e) => setCount(e.target.value)}
            />
          </Field>
          <Field label="批次备注" htmlFor="rc-batch">
            <Input
              id="rc-batch"
              name="batchName"
              value={batchName}
              onChange={(e) => setBatchName(e.target.value)}
              placeholder="promo-nov"
              autoCorrect="off"
              autoCapitalize="none"
              spellCheck={false}
            />
          </Field>
          <div className="sm:col-span-4 flex justify-end">
            <Button type="submit" variant="primary" icon={<Ticket className="h-4 w-4" />} loading={submitting}>
              生成一次性兑换码
            </Button>
          </div>
        </form>

        {createdBatch && createdBatch.codes.length > 0 && (
          <div className="rounded-2xl border border-[#2B3AE8]/25 bg-[#F4F6FF] p-4">
            <div className="mb-2 flex items-center justify-between text-xs font-semibold text-[#2B3AE8]">
              <span>新生成的明文兑换码（请立即复制保存）</span>
              <CopyButton text={createdBatch.codes.map((c) => c.code).join("\n")} label="复制全部兑换码" />
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {createdBatch.codes.map((c) => (
                <div key={c.id} className="flex items-center justify-between rounded-xl bg-white px-3 py-2 text-xs">
                  <Mono>{c.code}</Mono>
                  <CopyButton text={c.code} label={`复制 ${c.codeHint}`} />
                </div>
              ))}
            </div>
          </div>
        )}

        <div>
          <h4 id="redeem-history-heading" className="mb-2 text-xs font-semibold text-[#3D4258]">最近发放记录</h4>
          {loading && codes.length === 0 ? (
            <Skeleton className="h-24" />
          ) : codes.length === 0 ? (
            <p className="text-xs text-[#8A90A6]">暂无兑换码记录。</p>
          ) : (
            <div
              tabIndex={0}
              role="region"
              aria-labelledby="redeem-history-heading"
              className="max-h-60 overflow-y-auto rounded-xl border border-[#EEF0F7]"
            >
              <table className="w-full text-xs">
                <thead className="sticky top-0 z-10 bg-[#F9FAFE]">
                  <tr className="border-b border-[#EEF0F7] text-left text-[#5B6075]">
                    <th scope="col" className="px-3 py-2 font-medium">脱敏码</th>
                    <th scope="col" className="px-3 py-2 font-medium">套餐 / 时长</th>
                    <th scope="col" className="px-3 py-2 font-medium">批次</th>
                    <th scope="col" className="px-3 py-2 font-medium">使用状态</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#EEF0F7]">
                  {codes.map((c) => (
                    <tr key={c.id}>
                      <td className="px-3 py-2"><Mono>{c.codeHint}</Mono></td>
                      <td className="px-3 py-2">{c.planName} · {c.durationDays}天</td>
                      <td className="px-3 py-2 text-[#6B7188]">{c.batchName || "—"}</td>
                      <td className="px-3 py-2">
                        {c.redeemedCount >= c.maxRedemptions ? (
                          <Badge tone="neutral">已使用</Badge>
                        ) : c.disabled ? (
                          <Badge tone="danger">已停用</Badge>
                        ) : (
                          <Badge tone="success">可用</Badge>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}

export function TenantOrdersModal({
  tenant,
  onClose,
  onUpdated,
}: {
  tenant: Tenant | null;
  onClose: () => void;
  onUpdated: () => Promise<void>;
}) {
  const { apiFetch, toast } = useConsole();
  const plans = usePlansList(Boolean(tenant));
  const [planName, setPlanName] = useState("pro");
  const [durationDays, setDurationDays] = useState("30");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [orders, setOrders] = useState<QuotaOrderItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  useEffect(() => {
    if (plans.length > 0 && !plans.some((p) => p.metadata.name === planName)) {
      setPlanName(plans[0].metadata.name);
    }
  }, [plans, planName]);

  const loadOrders = useCallback(async () => {
    if (!tenant) return;
    setLoading(true);
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant.metadata.name)}/orders?limit=30`,
      );
      if (res.ok) {
        const data = await res.json();
        setOrders(Array.isArray(data?.items) ? data.items : []);
      }
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, [apiFetch, tenant]);

  useEffect(() => {
    if (tenant) {
      setReason("");
      void loadOrders();
    }
  }, [tenant, loadOrders]);

  async function submitGrant(e: FormEvent) {
    e.preventDefault();
    if (!tenant) return;
    if (!reason.trim()) {
      toast("error", "管理员发放权益订单必须填写原因（reason）");
      return;
    }
    const days = Number(durationDays);
    if (!Number.isInteger(days) || days < 1) {
      toast("error", "请输入大于 0 的有效天数");
      return;
    }
    setSubmitting(true);
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant.metadata.name)}/orders`,
        {
          method: "POST",
          body: JSON.stringify({
            planName,
            durationDays: days,
            reason: reason.trim(),
          }),
        },
      );
      if (!res.ok) {
        throw new Error(await errorMessage(res, "发放权益订单失败"));
      }
      toast("success", `已为 ${tenant.metadata.name} 发放 ${planName} (${days}天)`);
      setReason("");
      await Promise.all([loadOrders(), onUpdated()]);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "发放权益订单失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function cancelOrder(orderId: string) {
    if (!tenant) return;
    setCancellingId(orderId);
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant.metadata.name)}/orders/${encodeURIComponent(orderId)}`,
        {
          method: "PATCH",
          body: JSON.stringify({ state: "Cancelled", reason: "管理员在工作区管理面板撤销" }),
        },
      );
      if (!res.ok) {
        throw new Error(await errorMessage(res, "撤销订单失败"));
      }
      toast("success", `已撤销订单 ${orderId}`);
      await Promise.all([loadOrders(), onUpdated()]);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "撤销订单失败");
    } finally {
      setCancellingId(null);
    }
  }

  const display = tenant?.spec?.displayName || tenant?.metadata.name || "";

  return (
    <Modal
      open={Boolean(tenant)}
      onClose={onClose}
      title={`权益订单管理 · ${display}`}
      description="为订阅制工作区发放管理员补偿/赠送套餐订单，或撤销误发订单。"
      width="max-w-2xl"
      footer={
        <Button variant="ghost" onClick={onClose}>
          关闭
        </Button>
      }
    >
      <div className="space-y-5">
        <form onSubmit={(e) => void submitGrant(e)} className="grid gap-3 rounded-2xl bg-[#F7F8FC] p-4 sm:grid-cols-3">
          <Field label="套餐规格" htmlFor="ord-plan">
            <Select id="ord-plan" name="planName" value={planName} onChange={(e) => setPlanName(e.target.value)}>
              {(plans.length > 0
                ? plans
                : [
                    { metadata: { name: "starter" }, displayName: "体验版 Starter" },
                    { metadata: { name: "standard" }, displayName: "标准版 Standard" },
                    { metadata: { name: "pro" }, displayName: "专业版 Pro" },
                    { metadata: { name: "team" }, displayName: "团队版 Team" },
                  ]
              ).map((p) => (
                <option key={p.metadata.name} value={p.metadata.name}>
                  {p.displayName} ({p.metadata.name})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="有效天数" htmlFor="ord-days">
            <Input
              id="ord-days"
              name="durationDays"
              type="text"
              inputMode="numeric"
              value={durationDays}
              onChange={(e) => setDurationDays(e.target.value)}
            />
          </Field>
          <Field label="发放原因（必填）" htmlFor="ord-reason">
            <Input
              id="ord-reason"
              name="reason"
              required
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="内测赠送 / 故障补偿"
            />
          </Field>
          <div className="sm:col-span-3 flex justify-end">
            <Button type="submit" variant="primary" icon={<Gift className="h-4 w-4" />} loading={submitting}>
              发放权益订单
            </Button>
          </div>
        </form>

        <div>
          <h4 id="tenant-orders-heading" className="mb-2 text-xs font-semibold text-[#3D4258]">订单历史</h4>
          {loading && orders.length === 0 ? (
            <Skeleton className="h-24" />
          ) : orders.length === 0 ? (
            <p className="text-xs text-[#8A90A6]">该工作区暂无订单记录。</p>
          ) : (
            <div
              tabIndex={0}
              role="region"
              aria-labelledby="tenant-orders-heading"
              className="max-h-60 overflow-y-auto rounded-xl border border-[#EEF0F7]"
            >
              <table className="w-full text-xs">
                <thead className="sticky top-0 z-10 bg-[#F9FAFE]">
                  <tr className="border-b border-[#EEF0F7] text-left text-[#5B6075]">
                    <th scope="col" className="px-3 py-2 font-medium">订单</th>
                    <th scope="col" className="px-3 py-2 font-medium">套餐 / 来源</th>
                    <th scope="col" className="px-3 py-2 font-medium">有效期至</th>
                    <th scope="col" className="px-3 py-2 font-medium">状态</th>
                    <th scope="col" className="px-3 py-2 text-right font-medium">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#EEF0F7]">
                  {orders.map((ord) => {
                    const st = ORDER_STATE_META[ord.state] || { label: ord.state, tone: "neutral" as const };
                    return (
                      <tr key={ord.id}>
                        <td className="px-3 py-2"><Mono>{ord.id}</Mono></td>
                        <td className="px-3 py-2">
                          <div className="font-medium text-[#14161F]">{ord.planName} · {ord.durationDays}天</div>
                          <div className="text-[#8A90A6]">{ORDER_SOURCE_LABEL[ord.source] || ord.source}{ord.reason ? ` · ${ord.reason}` : ""}</div>
                        </td>
                        <td className="px-3 py-2 tabular-nums text-[#5B6075]">
                          {ord.expiresAt ? formatDateTime(ord.expiresAt) : "—"}
                        </td>
                        <td className="px-3 py-2">
                          <Badge tone={st.tone}>{st.label}</Badge>
                        </td>
                        <td className="px-3 py-2 text-right">
                          {ord.state === "Paid" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              icon={<Ban className="h-3.5 w-3.5" />}
                              loading={cancellingId === ord.id}
                              onClick={() => void cancelOrder(ord.id)}
                            >
                              撤销
                            </Button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}
