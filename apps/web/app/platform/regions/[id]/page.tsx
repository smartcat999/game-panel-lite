"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ConsolePageHeader } from "@/components/console-page-header";
import { PlatformScopeGuard } from "@/components/platform-scope-guard";
import { getRegionStatus, listRegions } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { regionDisplayName } from "@/lib/region-display";

function percent(used: number, total: number) {
  return total <= 0 ? 0 : Math.min(100, Math.round((used / total) * 100));
}

export default function RegionOperationsPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const regionId = decodeURIComponent(params.id);
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const regions = useQuery({ queryKey: ["platform", "regions"], queryFn: listRegions, retry: false });
  const status = useQuery({ queryKey: ["platform", "regions", regionId, "status"], queryFn: () => getRegionStatus(regionId), retry: false, refetchInterval: 30_000 });
  const region = regions.data?.find((item) => item.id === regionId);
  const snapshot = status.data;
  const observedAt = snapshot ? new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(snapshot.observedAtMs)) : "";
  const isStale = snapshot ? Date.now() - snapshot.observedAtMs > 90_000 : false;

  return (
    <PlatformScopeGuard>
      <div className="space-y-3">
        <ConsolePageHeader
          title={region ? regionDisplayName(region, locale) : regionId}
          action={
            <label className="flex items-center gap-2 text-[11px] text-slate-500">
              <span>{isZh ? "区域" : "Region"}</span>
              <select aria-label={isZh ? "切换区域" : "Switch Region"} value={regionId} onChange={(event) => router.push(`/platform/regions/${encodeURIComponent(event.target.value)}`)} className="h-7 rounded-md border border-slate-200 bg-white px-2 text-xs font-medium text-slate-700 outline-none focus:border-emerald-500">
                {(regions.data ?? []).map((entry) => <option key={entry.id} value={entry.id}>{regionDisplayName(entry, locale)}</option>)}
              </select>
            </label>
          }
        />

        <div className="rounded-xl border bg-white px-4 py-3 micro-border subtle-elevation">
          <div className="grid gap-3 md:grid-cols-[1fr_auto] md:items-center">
            <div>
              <p className="text-[10px] uppercase tracking-wider text-slate-400">{isZh ? "职责边界" : "Responsibility boundary"}</p>
              <p className="mt-1 text-xs leading-5 text-slate-600">{isZh ? "全局控制面负责区域目录和新建准入；节点、容量、部署与执行任务由该区域独立负责。此处只读取区域异步上报的运维摘要。" : "The global control plane owns the Region directory and create admission. This Region independently owns nodes, capacity, deployments, and execution tasks. This page reads only its asynchronously reported operations summary."}</p>
            </div>
            <div className="text-left md:text-right">
              <p className="text-[10px] uppercase tracking-wider text-slate-400">{isZh ? "全局准入" : "Global admission"}</p>
              <p className="mt-1 text-xs font-semibold text-slate-700">{region?.acceptingCreates ? (isZh ? "允许新建实例" : "Accepting new instances") : (isZh ? "已停止新建准入" : "Create admission closed")}</p>
            </div>
          </div>
        </div>

        {status.isLoading ? (
          <div className="rounded-xl border bg-white p-8 text-center text-xs text-slate-400 micro-border">{isZh ? "正在读取区域状态…" : "Loading Region status…"}</div>
        ) : status.isError ? (
          <div className="rounded-xl border border-amber-200 bg-amber-50 p-6 text-center text-xs text-amber-800">{isZh ? "区域状态暂时无法读取，请稍后重试。" : "Region status is temporarily unavailable. Try again shortly."}</div>
        ) : snapshot ? (
          <>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {[
                [isZh ? "节点在线" : "Nodes online", `${snapshot.nodes.online} / ${snapshot.nodes.total}`, isZh ? `${snapshot.nodes.schedulable} 个允许调度` : `${snapshot.nodes.schedulable} schedulable`],
                [isZh ? "区域部署" : "Regional deployments", snapshot.deployments.total, isZh ? `${snapshot.deployments.pending} 待调度 · ${snapshot.deployments.rejected} 已拒绝` : `${snapshot.deployments.pending} pending · ${snapshot.deployments.rejected} rejected`],
                [isZh ? "待授权任务" : "Tasks awaiting authority", snapshot.tasks.awaitingAuthority, isZh ? "尚不可下发到节点执行" : "Not yet executable by nodes"],
                [isZh ? "状态序列" : "Status sequence", snapshot.sequence, isZh ? "用于拒绝乱序与冲突上报" : "Rejects stale or conflicting reports"]
              ].map(([label, value, detail]) => (
                <div key={String(label)} className="rounded-xl border bg-white p-4 micro-border subtle-elevation">
                  <p className="text-[10px] uppercase tracking-wider text-slate-400">{label}</p>
                  <p className="mt-2 text-xl font-semibold tracking-tight text-slate-900">{value}</p>
                  <p className="mt-1 text-[11px] text-slate-500">{detail}</p>
                </div>
              ))}
            </div>

            <div className="rounded-xl border bg-white p-4 micro-border subtle-elevation">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div><h2 className="text-xs font-semibold text-slate-800">{isZh ? "区域容量" : "Regional capacity"}</h2><p className="mt-1 text-[11px] text-slate-500">{isZh ? "预留量来自区域调度数据库，不代表实时进程利用率。" : "Reservations come from the regional scheduler database and are not live process utilization."}</p></div>
                <p className={isStale ? "text-[10px] font-medium text-amber-700" : "text-[10px] text-slate-400"}>{isStale ? (isZh ? `状态可能已过期 · ${observedAt}` : `Status may be stale · ${observedAt}`) : (isZh ? `观测于 ${observedAt}` : `Observed ${observedAt}`)}</p>
              </div>
              <div className="mt-4 grid gap-4 md:grid-cols-2">
                <CapacityLine label="CPU" used={snapshot.capacity.cpuReserved} total={snapshot.capacity.cpuTotal} percentage={percent(snapshot.capacity.cpuReserved, snapshot.capacity.cpuTotal)} />
                <CapacityLine label={isZh ? "内存" : "Memory"} used={snapshot.capacity.memoryReservedMb} total={snapshot.capacity.memoryTotalMb} percentage={percent(snapshot.capacity.memoryReservedMb, snapshot.capacity.memoryTotalMb)} unit="MB" />
              </div>
            </div>
          </>
        ) : (
          <div className="rounded-xl border border-dashed bg-white p-8 text-center micro-border">
            <p className="text-sm font-semibold text-slate-700">{isZh ? "尚未收到区域状态" : "No Region status received"}</p>
            <p className="mx-auto mt-2 max-w-xl text-xs leading-5 text-slate-500">{isZh ? "区域仍可存在于全局目录，但在区域状态发布器完成首次异步上报前，全局控制面不会推断节点或容量。" : "A Region can exist in the global directory, but the control plane will not infer nodes or capacity before its status publisher sends the first asynchronous report."}</p>
          </div>
        )}
      </div>
    </PlatformScopeGuard>
  );
}

function CapacityLine({ label, used, total, percentage, unit = "CPU" }: { label: string; used: number; total: number; percentage: number; unit?: string }) {
  return (
    <div>
      <div className="flex items-center justify-between text-[11px]"><span className="font-medium text-slate-600">{label}</span><span className="font-mono text-slate-500">{used} / {total} {unit}</span></div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100"><div className="h-full rounded-full bg-emerald-500" style={{ width: `${percentage}%` }} /></div>
    </div>
  );
}
