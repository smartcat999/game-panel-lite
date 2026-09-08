"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowUpRight,
  CheckCircle2,
  Clock,
  CreditCard,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Zap
} from "lucide-react";
import { Button } from "@/components/ui";
import {
  listCommercePlans,
  listCommerceSubscriptions,
  createCommerceOrder,
  simulatePaymentWebhook
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { CommercePlanVersion, CommerceSubscription, GameServerResource } from "@/lib/types";

interface ServerSubscriptionCardProps {
  server: GameServerResource;
  isViewer?: boolean;
}

export function ServerSubscriptionCard({ server, isViewer }: ServerSubscriptionCardProps) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();

  const [renewModalOpen, setRenewModalOpen] = useState(false);
  const [upgradeModalOpen, setUpgradeModalOpen] = useState(false);
  const [selectedPeriods, setSelectedPeriods] = useState<number>(1);
  const [selectedUpgradePlan, setSelectedUpgradePlan] = useState<CommercePlanVersion | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);

  const { data: subscriptions = [] } = useQuery({
    queryKey: ["commerce-subscriptions", server.organizationId],
    queryFn: () => listCommerceSubscriptions(server.organizationId),
    enabled: Boolean(server.organizationId),
    staleTime: 30 * 1000
  });

  const { data: plans = [] } = useQuery({
    queryKey: ["commerce-plans"],
    queryFn: listCommercePlans,
    staleTime: 60 * 1000
  });

  const currentSubscription = subscriptions.find(
    (s: CommerceSubscription) => s.serverId === server.id && s.status === "active"
  );

  // Compute expiry time and remaining days
  const expiryInfo = (() => {
    if (!currentSubscription) return null;
    const durationMs = currentSubscription.quote.durationMs || (currentSubscription.quote.plan.periodSeconds * 1000 * currentSubscription.quote.periods);
    const expiresAt = currentSubscription.createdAtMs + durationMs;
    const now = Date.now();
    const remainingMs = expiresAt - now;
    const remainingDays = Math.max(0, Math.ceil(remainingMs / (1000 * 60 * 60 * 24)));
    const isExpired = remainingMs <= 0;
    const isExpiringSoon = !isExpired && remainingDays <= 5;
    return {
      expiresAt: new Date(expiresAt).toLocaleDateString(locale, {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit"
      }),
      remainingDays,
      isExpired,
      isExpiringSoon
    };
  })();

  // Mutation to handle Renew / Upgrade
  const checkoutMutation = useMutation({
    mutationFn: async ({ plan, periods }: { plan: CommercePlanVersion; periods: number }) => {
      setActionError(null);
      setActionSuccess(null);

      const order = await createCommerceOrder({
        organizationId: server.organizationId,
        serverId: server.id,
        planId: plan.planId,
        planVersion: plan.version,
        periods,
        idempotencyKey: `sub-${server.id}-${Date.now()}`
      });

      await simulatePaymentWebhook({
        provider: "wechat",
        merchantId: "mch_default",
        transactionId: `tx-${Date.now()}`,
        eventId: `evt-${Date.now()}`,
        orderId: order.id,
        amountMinor: order.quote.amountMinor,
        currency: order.quote.plan.currency
      });

      return order;
    },
    onSuccess: () => {
      setActionSuccess(isZh ? "预付费订阅已成功激活/续期！" : "Subscription successfully renewed / activated!");
      queryClient.invalidateQueries({ queryKey: ["commerce-subscriptions"] });
      setTimeout(() => {
        setRenewModalOpen(false);
        setUpgradeModalOpen(false);
        setActionSuccess(null);
      }, 1500);
    },
    onError: (err: Error) => {
      setActionError(err.message || (isZh ? "结算开通失败，请稍后重试" : "Checkout failed, please retry"));
    }
  });

  if (!server.organizationId) {
    return null;
  }

  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="flex size-7 items-center justify-center rounded-lg border border-panel-gold/30 bg-panel-gold/10 text-panel-gold">
            <Sparkles className="size-3.5" />
          </span>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-xs font-bold uppercase tracking-wider text-slate-200">
                {isZh ? "SaaS 预付费订阅与商业保障" : "SaaS Prepaid Subscription & SLA"}
              </h2>
              {currentSubscription ? (
                <span className="inline-flex items-center gap-1 rounded-full border border-panel-green/30 bg-panel-green/10 px-2 py-0.5 text-[10px] font-semibold text-panel-green">
                  <ShieldCheck className="size-3" />
                  {isZh ? "商业订阅生效中" : "Active Subscription"}
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 rounded-full border border-slate-700 bg-slate-800/60 px-2 py-0.5 text-[10px] font-medium text-slate-400">
                  {isZh ? "基础点券模式" : "Credits Mode"}
                </span>
              )}
            </div>
            <p className="text-[11px] text-slate-500">
              {currentSubscription
                ? (isZh ? "独立 CPU 与内存资源预留保障，到期前可自主续费" : "Dedicated resource allocation with guaranteed availability")
                : (isZh ? "可随时升级为商业套餐，享受独享高保真性能与自动续期服务" : "Upgrade to commercial prepaid plan for dedicated resources")}
            </p>
          </div>
        </div>

        {!isViewer && (
          <div>
            {currentSubscription ? (
              <Button
                variant="gold"
                onClick={() => {
                  setActionError(null);
                  setActionSuccess(null);
                  setRenewModalOpen(true);
                }}
                className="text-xs px-2.5 py-1.5 flex items-center gap-1.5 font-semibold"
              >
                <RefreshCw className="size-3" />
                <span>{isZh ? "续费订阅" : "Renew Subscription"}</span>
              </Button>
            ) : (
              <Button
                variant="primary"
                onClick={() => {
                  setActionError(null);
                  setActionSuccess(null);
                  setUpgradeModalOpen(true);
                }}
                className="text-xs px-2.5 py-1.5 flex items-center gap-1.5 font-semibold"
              >
                <Zap className="size-3" />
                <span>{isZh ? "升级商业套餐" : "Upgrade to Plan"}</span>
              </Button>
            )}
          </div>
        )}
      </div>

      {/* Subscription Metrics Overview */}
      {currentSubscription && expiryInfo ? (
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">{isZh ? "已绑商业套餐" : "Active Plan"}</span>
            <div className="mt-1 flex items-center justify-between">
              <span className="text-sm font-bold text-slate-200 truncate">
                {currentSubscription.quote.plan.planId}
              </span>
              <span className="text-xs font-mono font-bold text-panel-gold">
                ¥{(currentSubscription.quote.plan.unitAmountMinor / 100).toFixed(0)}/{isZh ? "月" : "mo"}
              </span>
            </div>
            <span className="text-[10px] text-slate-500 font-mono mt-0.5 block">
              {currentSubscription.quote.plan.cpu}C / {(currentSubscription.quote.plan.memoryMb / 1024).toFixed(0)}GB
            </span>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">{isZh ? "服务到期时间" : "Expires At"}</span>
            <div className="mt-1 flex items-center gap-1.5">
              <Clock className="size-3.5 text-slate-400" />
              <span className="text-xs font-mono font-semibold text-slate-200">
                {expiryInfo.expiresAt}
              </span>
            </div>
            <span className="text-[10px] text-slate-500 mt-0.5 block">
              {isZh ? "周期结束后保留存档 7 天" : "World files retained 7 days"}
            </span>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">{isZh ? "剩余有效期" : "Remaining Time"}</span>
            <div className="mt-1 flex items-center gap-1.5">
              {expiryInfo.isExpiringSoon ? (
                <span className="flex items-center gap-1 text-panel-gold font-bold text-sm">
                  <AlertTriangle className="size-3.5" />
                  {isZh ? `仅剩 ${expiryInfo.remainingDays} 天` : `${expiryInfo.remainingDays} days left`}
                </span>
              ) : expiryInfo.isExpired ? (
                <span className="text-rose-400 font-bold text-sm">
                  {isZh ? "已过期" : "Expired"}
                </span>
              ) : (
                <span className="text-panel-green font-bold text-sm">
                  {isZh ? `${expiryInfo.remainingDays} 天` : `${expiryInfo.remainingDays} days`}
                </span>
              )}
            </div>
            <span className="text-[10px] text-slate-500 mt-0.5 block">
              {isZh ? "已自动发放实例运算权益" : "Entitlements granted"}
            </span>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">{isZh ? "商户履约与凭证" : "Fulfillment Receipt"}</span>
            <div className="mt-1 flex items-center gap-1.5">
              <CheckCircle2 className="size-3.5 text-panel-green" />
              <span className="text-xs font-mono text-slate-300 truncate">
                {currentSubscription.paymentId.slice(0, 14)}...
              </span>
            </div>
            <span className="text-[10px] text-slate-500 mt-0.5 block font-mono">
              Order: {currentSubscription.orderId.slice(0, 10)}
            </span>
          </div>
        </div>
      ) : (
        <div className="mt-4 rounded-lg border border-dashed border-slate-800 bg-slate-900/30 p-4 flex flex-col sm:flex-row items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="size-9 rounded-full bg-slate-800 flex items-center justify-center text-slate-400">
              <CreditCard className="size-4" />
            </div>
            <div>
              <p className="text-xs font-semibold text-slate-300">
                {isZh ? "未绑定预付费商业套餐" : "No Prepaid Commercial Plan Bound"}
              </p>
              <p className="text-[11px] text-slate-500 mt-0.5">
                {isZh ? "当前实例通过开服点券启动。升级为套餐后享有独立节点计算保障与到期提醒支持。" : "Server is running on credits. Upgrade to enjoy dedicated resources."}
              </p>
            </div>
          </div>
          {!isViewer && (
            <button
              type="button"
              onClick={() => setUpgradeModalOpen(true)}
              className="text-xs text-panel-green hover:underline flex items-center gap-1 font-semibold"
            >
              <span>{isZh ? "查看可用套餐" : "Browse Plans"}</span>
              <ArrowUpRight className="size-3" />
            </button>
          )}
        </div>
      )}

      {/* Renew Modal */}
      {renewModalOpen && currentSubscription && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-xl border border-slate-800 bg-slate-950 p-5 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <RefreshCw className="size-4 text-panel-gold" />
                <h3 className="text-sm font-bold text-white">
                  {isZh ? "续费当前商业订阅" : "Renew Subscription"}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setRenewModalOpen(false)}
                className="text-slate-400 hover:text-white text-xs"
              >
                ✕
              </button>
            </div>

            <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3 space-y-1 text-xs">
              <div className="flex justify-between">
                <span className="text-slate-400">{isZh ? "套餐名称" : "Plan"}</span>
                <span className="font-bold text-white">{currentSubscription.quote.plan.planId}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-400">{isZh ? "单月资费" : "Monthly Rate"}</span>
                <span className="font-mono text-panel-gold font-semibold">
                  ¥{(currentSubscription.quote.plan.unitAmountMinor / 100).toFixed(2)}
                </span>
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-300">
                {isZh ? "选择续费时长" : "Select Duration"}
              </label>
              <div className="grid grid-cols-4 gap-2">
                {[
                  { periods: 1, label: isZh ? "1 个月" : "1 mo", badge: "" },
                  { periods: 3, label: isZh ? "3 个月" : "3 mos", badge: isZh ? "特惠" : "Save" },
                  { periods: 6, label: isZh ? "半年" : "6 mos", badge: isZh ? "推荐" : "Best" },
                  { periods: 12, label: isZh ? "年付" : "1 yr", badge: isZh ? "超值" : "Value" }
                ].map((p) => {
                  const isSelected = selectedPeriods === p.periods;
                  return (
                    <button
                      key={p.periods}
                      type="button"
                      onClick={() => setSelectedPeriods(p.periods)}
                      className={cn(
                        "flex flex-col items-center justify-center p-2 rounded-lg border text-center transition relative",
                        isSelected
                          ? "border-panel-gold bg-panel-gold/10 text-white font-bold"
                          : "border-slate-800 bg-slate-900/60 text-slate-400 hover:border-slate-700"
                      )}
                    >
                      {p.badge && (
                        <span className="absolute -top-1.5 -right-1 px-1 py-0.2 rounded bg-panel-gold text-slate-950 font-black text-[9px]">
                          {p.badge}
                        </span>
                      )}
                      <span className="text-xs">{p.label}</span>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Total and actions */}
            <div className="flex items-center justify-between border-t border-slate-800 pt-3">
              <div>
                <span className="text-[11px] text-slate-500 block">{isZh ? "应付金额" : "Total Amount"}</span>
                <span className="text-lg font-mono font-black text-panel-gold">
                  ¥{((currentSubscription.quote.plan.unitAmountMinor * selectedPeriods) / 100).toFixed(2)}
                </span>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  className="text-xs px-3 py-1.5"
                  onClick={() => setRenewModalOpen(false)}
                >
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button
                  variant="gold"
                  disabled={checkoutMutation.isPending}
                  onClick={() => {
                    checkoutMutation.mutate({
                      plan: currentSubscription.quote.plan,
                      periods: selectedPeriods
                    });
                  }}
                  className="text-xs px-3 py-1.5 font-bold text-slate-950"
                >
                  {checkoutMutation.isPending ? (
                    <span className="flex items-center gap-1">
                      <RefreshCw className="size-3 animate-spin" />
                      {isZh ? "支付履约中..." : "Confirming..."}
                    </span>
                  ) : (
                    isZh ? "确认续费并支付" : "Confirm & Pay"
                  )}
                </Button>
              </div>
            </div>

            {actionError && (
              <p className="text-xs text-rose-400 bg-rose-500/10 border border-rose-500/20 p-2 rounded">
                {actionError}
              </p>
            )}
            {actionSuccess && (
              <p className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 p-2 rounded">
                {actionSuccess}
              </p>
            )}
          </div>
        </div>
      )}

      {/* Upgrade Modal */}
      {upgradeModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-lg rounded-xl border border-slate-800 bg-slate-950 p-5 shadow-2xl space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <Zap className="size-4 text-panel-green" />
                <h3 className="text-sm font-bold text-white">
                  {isZh ? "升级为商业预付费套餐" : "Upgrade to Prepaid Plan"}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setUpgradeModalOpen(false)}
                className="text-slate-400 hover:text-white text-xs"
              >
                ✕
              </button>
            </div>

            <p className="text-xs text-slate-400">
              {isZh
                ? "选择适合你好友小队的硬件规格套餐，完成后自动履约并为实例绑定独占资源配额："
                : "Select the desired plan specs to grant dedicated quota for this server:"}
            </p>

            <div className="grid gap-2 sm:grid-cols-2">
              {plans.map((p) => {
                const isSelected = selectedUpgradePlan?.planId === p.planId;
                return (
                  <button
                    key={p.planId}
                    type="button"
                    onClick={() => setSelectedUpgradePlan(p)}
                    className={cn(
                      "flex flex-col items-start p-3 rounded-lg border text-left transition",
                      isSelected
                        ? "border-panel-green bg-panel-green/10 text-white"
                        : "border-slate-800 bg-slate-900/60 text-slate-300 hover:border-slate-700"
                    )}
                  >
                    <div className="flex items-center justify-between w-full">
                      <span className="text-xs font-bold truncate">{p.planId}</span>
                      <span className="text-xs font-mono font-bold text-panel-gold">
                        ¥{(p.unitAmountMinor / 100).toFixed(0)}/{isZh ? "月" : "mo"}
                      </span>
                    </div>
                    <span className="text-[10px] text-slate-400 font-mono mt-1">
                      {p.cpu}C / {(p.memoryMb / 1024).toFixed(0)}GB · {p.regionId}
                    </span>
                    <span className="text-[10px] text-slate-500 mt-0.5">
                      {p.providerKey}
                    </span>
                  </button>
                );
              })}
            </div>

            {/* Total and actions */}
            <div className="flex items-center justify-between border-t border-slate-800 pt-3">
              <div>
                <span className="text-[11px] text-slate-500 block">{isZh ? "首期应付金额" : "First Month Total"}</span>
                <span className="text-lg font-mono font-black text-panel-green">
                  ¥{selectedUpgradePlan ? (selectedUpgradePlan.unitAmountMinor / 100).toFixed(2) : "0.00"}
                </span>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  className="text-xs px-3 py-1.5"
                  onClick={() => setUpgradeModalOpen(false)}
                >
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button
                  variant="primary"
                  disabled={!selectedUpgradePlan || checkoutMutation.isPending}
                  onClick={() => {
                    if (selectedUpgradePlan) {
                      checkoutMutation.mutate({
                        plan: selectedUpgradePlan,
                        periods: 1
                      });
                    }
                  }}
                  className="text-xs px-3 py-1.5 font-bold"
                >
                  {checkoutMutation.isPending ? (
                    <span className="flex items-center gap-1">
                      <RefreshCw className="size-3 animate-spin" />
                      {isZh ? "开通履约中..." : "Activating..."}
                    </span>
                  ) : (
                    isZh ? "立即支付并开通" : "Pay & Activate"
                  )}
                </Button>
              </div>
            </div>

            {actionError && (
              <p className="text-xs text-rose-400 bg-rose-500/10 border border-rose-500/20 p-2 rounded">
                {actionError}
              </p>
            )}
            {actionSuccess && (
              <p className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 p-2 rounded">
                {actionSuccess}
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
