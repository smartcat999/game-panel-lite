"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Cpu, HardDrive, MapPin, MemoryStick, Server, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { createCommerceOrder, createTenantInstance, listCommercePlans, listRegions } from "@/lib/api";
import type { CommercePlanVersion } from "@/lib/types";
import { Button, Input } from "@/components/ui";
import { cn } from "@/lib/utils";
import { regionDisplayName } from "@/lib/region-display";

interface DeployInstanceModalProps {
  open: boolean;
  onClose: () => void;
  organizationId?: string;
}

type Engine = "vanilla" | "tmodloader";

export function DeployInstanceModal({ open, onClose, organizationId }: DeployInstanceModalProps) {
  const queryClient = useQueryClient();
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  const [engine, setEngine] = useState<Engine>("vanilla");
  const [name, setName] = useState("terraria-survival");
  const [regionId, setRegionId] = useState("");
  const [planId, setPlanId] = useState("");
  const [error, setError] = useState("");
  const [createdOrderId, setCreatedOrderId] = useState("");
  const requestIdentity = useRef("");

  const plansQuery = useQuery({
    queryKey: ["commerce-plans"],
    queryFn: listCommercePlans,
    enabled: open,
    retry: false
  });
  const regionsQuery = useQuery({
    queryKey: ["regions"],
    queryFn: listRegions,
    enabled: open,
    retry: false,
    staleTime: 5 * 60 * 1000
  });

  const providerKey = engine === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
  const providerPlans = useMemo(
    () => (plansQuery.data ?? []).filter((plan) => plan.providerKey === providerKey),
    [plansQuery.data, providerKey]
  );
  const regionIds = useMemo(
    () => {
      const accepting = new Set((regionsQuery.data ?? []).filter((region) => region.acceptingCreates).map((region) => region.id));
      return Array.from(new Set(providerPlans.map((plan) => plan.regionId).filter((id) => accepting.has(id))));
    },
    [providerPlans, regionsQuery.data]
  );
  const visiblePlans = useMemo(
    () => providerPlans.filter((plan) => plan.regionId === regionId),
    [providerPlans, regionId]
  );
  const selectedPlan = visiblePlans.find((plan) => planKey(plan) === planId);

  useEffect(() => {
    if (!open) return;
    const randomSuffix = Math.floor(10 + Math.random() * 90);
    setName(`terraria-${randomSuffix}`);
    setError("");
    setCreatedOrderId("");
    requestIdentity.current = "";
  }, [open]);

  useEffect(() => {
    if (!open || regionIds.length === 0) return;
    const firstRegionId = regionIds[0];
    if (firstRegionId && !regionIds.includes(regionId)) setRegionId(firstRegionId);
  }, [open, regionId, regionIds]);

  useEffect(() => {
    if (!open || visiblePlans.length === 0) return;
    const firstPlan = visiblePlans[0];
    if (firstPlan && !visiblePlans.some((plan) => planKey(plan) === planId)) setPlanId(planKey(firstPlan));
  }, [open, planId, visiblePlans]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  const deployMutation = useMutation({
    mutationFn: async () => {
      if (!organizationId) throw new Error(isZh ? "请先选择空间" : "Select a workspace first");
      if (!selectedPlan) throw new Error(isZh ? "请选择可用套餐" : "Select an available plan");
      if (!requestIdentity.current) requestIdentity.current = crypto.randomUUID();
      const instance = await createTenantInstance({
        organizationId,
        name: name.trim(),
        planId: selectedPlan.planId,
        planVersion: selectedPlan.version,
        idempotencyKey: `instance-${requestIdentity.current}`,
        configuration: {}
      });
      const order = await createCommerceOrder({
        organizationId,
        serverId: instance.instanceId,
        planId: selectedPlan.planId,
        planVersion: selectedPlan.version,
        periods: 1,
        idempotencyKey: `order-${requestIdentity.current}`
      });
      return { instance, order };
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["tenant-instances", organizationId] });
      setCreatedOrderId(result.order.id);
      setError("");
    },
    onError: (err: unknown) => {
      setError(err instanceof Error ? err.message : isZh ? "创建实例失败" : "Failed to deploy instance");
    }
  });

  if (!open) return null;

  const regionName = (id: string) => {
    const region = regionsQuery.data?.find((item) => item.id === id);
    if (region) return regionDisplayName(region, locale);
    if (id === "default") return isZh ? "默认区域" : "Default region";
    return id.replaceAll("-", " ");
  };

  return (
    <div
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/35 p-4 animate-in fade-in duration-150"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="deploy-title"
        onClick={(event) => event.stopPropagation()}
        className="max-h-[calc(100vh-2rem)] w-full max-w-2xl overflow-y-auto rounded-xl border bg-white shadow-xl micro-border animate-in zoom-in-95 duration-150"
      >
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-slate-100 bg-white px-5 py-3">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-lg border border-emerald-100 bg-emerald-50 text-emerald-600">
              <Server className="size-4" />
            </div>
            <div>
              <h2 id="deploy-title" className="text-sm font-bold text-slate-900">
                {isZh ? "创建 Terraria 实例" : "Create Terraria instance"}
              </h2>
              <p className="text-[11px] text-slate-400">
                {isZh ? "服务激活后，由所选区域自动调度节点" : "The selected Region schedules a node after service activation"}
              </p>
            </div>
          </div>
          <button
            type="button"
            aria-label={isZh ? "关闭" : "Close"}
            onClick={onClose}
            className="flex size-7 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 hover:text-slate-700"
          >
            <X className="size-4" />
          </button>
        </div>

        <div className="space-y-5 p-5 text-xs">
          <section className="space-y-2">
            <h3 className="font-semibold text-slate-800">{isZh ? "服务器类型" : "Server type"}</h3>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <EngineOption
                active={engine === "vanilla"}
                description={isZh ? "官方原版服务端" : "Official server"}
                label="Vanilla"
                onClick={() => setEngine("vanilla")}
              />
              <EngineOption
                active={engine === "tmodloader"}
                description={isZh ? "支持模组的服务端" : "Mod-enabled server"}
                label="tModLoader"
                tone="purple"
                onClick={() => setEngine("tmodloader")}
              />
            </div>
          </section>

          <section className="space-y-2">
            <h3 className="font-semibold text-slate-800">{isZh ? "区域" : "Region"}</h3>
            {plansQuery.isLoading ? (
              <div className="h-9 animate-pulse rounded-lg bg-slate-100" />
            ) : regionIds.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {regionIds.map((id) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setRegionId(id)}
                    className={cn(
                      "flex h-8 items-center gap-1.5 rounded-lg border px-2.5 font-medium transition",
                      regionId === id
                        ? "border-emerald-500 bg-emerald-50 text-emerald-700"
                        : "border-slate-200 bg-white text-slate-600 hover:bg-slate-50"
                    )}
                  >
                    <MapPin className="size-3.5" />
                    <span className="capitalize">{regionName(id)}</span>
                  </button>
                ))}
              </div>
            ) : (
              <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-amber-700">
                {isZh ? "当前类型暂无可售区域" : "No regions are available for this server type"}
              </p>
            )}
          </section>

          <section className="space-y-2">
            <h3 className="font-semibold text-slate-800">{isZh ? "套餐" : "Plan"}</h3>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {visiblePlans.map((plan) => (
                <PlanOption
                  key={`${plan.planId}-${plan.version}`}
                  active={planKey(plan) === planId}
                  locale={locale}
                  plan={plan}
                  onClick={() => setPlanId(planKey(plan))}
                />
              ))}
            </div>
          </section>

          <section className="space-y-1.5">
            <label htmlFor="instance-name" className="font-semibold text-slate-800">
              {isZh ? "实例名称" : "Instance name"}
            </label>
            <Input
              id="instance-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              className="font-mono"
            />
          </section>

          {error ? (
            <p role="alert" className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-rose-700">
              {error}
            </p>
          ) : null}
          {createdOrderId ? (
            <div className="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-emerald-800">
              <p className="font-semibold">{isZh ? "实例配置和待支付订单已创建" : "Instance configuration and pending order created"}</p>
              <p className="mt-1 font-mono text-[10px] text-emerald-700">{createdOrderId}</p>
              <p className="mt-1 text-[11px] leading-4 text-emerald-700">
                {isZh ? "当前本地环境尚未配置真实支付渠道，订单不会自动授予运行权益。" : "No real payment provider is configured locally, so this order does not grant runtime entitlement automatically."}
              </p>
            </div>
          ) : null}
        </div>

        <div className="sticky bottom-0 flex items-center justify-between gap-3 border-t border-slate-100 bg-white px-5 py-3">
          <p className="text-[11px] text-slate-400">
            {selectedPlan ? `${formatPeriod(selectedPlan.periodSeconds, isZh)} · ${isZh ? "激活后开始交付" : "delivery starts after activation"}` : ""}
          </p>
          <div className="flex items-center gap-2">
            <Button type="button" variant="secondary" onClick={onClose} disabled={deployMutation.isPending}>
              {isZh ? "取消" : "Cancel"}
            </Button>
            <Button
              type="button"
              onClick={() => createdOrderId ? onClose() : deployMutation.mutate()}
              disabled={deployMutation.isPending || (!createdOrderId && (!name.trim() || !selectedPlan))}
              className="bg-slate-900 text-white hover:bg-slate-800"
            >
              {createdOrderId ? (isZh ? "完成" : "Done") : deployMutation.isPending ? (isZh ? "正在创建" : "Creating") : (isZh ? "创建订单" : "Create order")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function EngineOption({
  active,
  description,
  label,
  onClick,
  tone = "green"
}: {
  active: boolean;
  description: string;
  label: string;
  onClick: () => void;
  tone?: "green" | "purple";
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center justify-between rounded-xl border p-3 text-left transition",
        active && tone === "green" && "border-emerald-500 bg-emerald-50/60",
        active && tone === "purple" && "border-purple-500 bg-purple-50/60",
        !active && "border-slate-200 bg-white hover:bg-slate-50"
      )}
    >
      <span>
        <span className="block font-bold text-slate-900">{label}</span>
        <span className="mt-0.5 block text-[11px] text-slate-400">{description}</span>
      </span>
      {active ? <Check className={cn("size-4", tone === "purple" ? "text-purple-600" : "text-emerald-600")} /> : null}
    </button>
  );
}

function PlanOption({ active, locale, plan, onClick }: {
  active: boolean;
  locale: string;
  plan: CommercePlanVersion;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-xl border p-3 text-left transition",
        active ? "border-emerald-500 bg-emerald-50/50" : "border-slate-200 bg-white hover:bg-slate-50"
      )}
    >
      <span className="flex items-start justify-between gap-3">
        <span className="font-bold capitalize text-slate-900">{planName(plan.planId)}</span>
        <span className="font-mono font-semibold text-slate-900">{formatPrice(plan, locale)}</span>
      </span>
      <span className="mt-3 grid grid-cols-3 gap-2 text-[11px] text-slate-500">
        <span className="flex items-center gap-1"><Cpu className="size-3" />{plan.cpu} vCPU</span>
        <span className="flex items-center gap-1"><MemoryStick className="size-3" />{formatMemory(plan.memoryMb)}</span>
        <span className="flex items-center gap-1"><HardDrive className="size-3" />{formatStorage(plan.storageBytes)}</span>
      </span>
    </button>
  );
}

function planName(planId: string) {
  const part = planId.split("-").at(-1) ?? planId;
  return part.replaceAll("_", " ");
}

function planKey(plan: CommercePlanVersion) {
  return `${plan.planId}@${plan.version}`;
}

function formatPrice(plan: CommercePlanVersion, locale: string) {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: plan.currency,
    maximumFractionDigits: 0
  }).format(plan.unitAmountMinor / 100);
}

function formatMemory(memoryMb: number) {
  return memoryMb >= 1024 ? `${memoryMb / 1024} GB` : `${memoryMb} MB`;
}

function formatStorage(bytes: number) {
  return `${Math.round(bytes / 1024 / 1024 / 1024)} GB`;
}

function formatPeriod(seconds: number, isZh: boolean) {
  const days = Math.max(1, Math.round(seconds / 86400));
  return isZh ? `套餐周期 ${days} 天` : `${days}-day billing period`;
}
