"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ConsolePageHeader } from "@/components/console-page-header";
import { PlatformScopeGuard } from "@/components/platform-scope-guard";
import { getRegionNodes, getRegionStatus, listRegions } from "@/lib/api";
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
  const nodes = useQuery({ queryKey: ["platform", "regions", regionId, "nodes"], queryFn: () => getRegionNodes(regionId), retry: false, refetchInterval: 30_000 });
  const region = regions.data?.find((item) => item.id === regionId);
  const snapshot = status.data;
  const nodePage = nodes.data;
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

        <section className="overflow-hidden rounded-xl border bg-white micro-border subtle-elevation">
          <div className="flex flex-wrap items-end justify-between gap-2 border-b border-slate-100 px-4 py-3">
            <div>
              <h2 className="text-xs font-semibold text-slate-800">{isZh ? "节点运维" : "Node operations"}</h2>
              <p className="mt-1 text-[11px] text-slate-500">{isZh ? "明细直接来自该区域的运维接口，不写入全局数据库。" : "Details come directly from this Region's operations API and are not stored in the global database."}</p>
            </div>
            {nodePage ? <p className="text-[10px] text-slate-400">{isZh ? `读取于 ${new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(nodePage.observedAtMs))}` : `Read ${new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(nodePage.observedAtMs))}`}</p> : null}
          </div>
          {nodes.isLoading ? (
            <div className="p-8 text-center text-xs text-slate-400">{isZh ? "正在连接区域运维服务…" : "Connecting to Region operations…"}</div>
          ) : nodes.isError ? (
            <div className="p-8 text-center">
              <p className="text-sm font-medium text-slate-700">{isZh ? "区域运维通道暂不可用" : "Region operations are unavailable"}</p>
              <p className="mx-auto mt-2 max-w-xl text-xs leading-5 text-slate-500">{isZh ? "全局状态摘要仍可独立工作；节点明细需要配置到该区域的双向认证连接。" : "The global status summary remains independent. Node details require a configured mutual-authentication connection to this Region."}</p>
            </div>
          ) : !nodePage || nodePage.nodes.length === 0 ? (
            <div className="p-8 text-center text-xs text-slate-500">{isZh ? "该区域尚未配置节点。" : "No nodes are configured in this Region."}</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-xs">
                <thead className="bg-slate-50 text-[10px] uppercase tracking-wider text-slate-400">
                  <tr><th className="px-4 py-2.5 font-medium">{isZh ? "节点" : "Node"}</th><th className="px-3 py-2.5 font-medium">{isZh ? "运行状态" : "Runtime"}</th><th className="px-3 py-2.5 font-medium">{isZh ? "调度" : "Scheduling"}</th><th className="px-3 py-2.5 font-medium">CPU</th><th className="px-3 py-2.5 font-medium">{isZh ? "内存" : "Memory"}</th><th className="px-4 py-2.5 text-right font-medium">{isZh ? "待授权任务" : "Pending tasks"}</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {nodePage.nodes.map((node) => (
                    <tr key={node.id} className="text-slate-600">
                      <td className="px-4 py-3"><p className="font-medium text-slate-800">{node.name}</p><p className="mt-0.5 font-mono text-[10px] text-slate-400">{node.id} · {node.architecture}</p></td>
                      <td className="px-3 py-3"><span className="inline-flex items-center gap-1.5"><span className={`size-1.5 rounded-full ${node.online ? "bg-emerald-500" : "bg-slate-300"}`} />{node.online ? (isZh ? "在线" : "Online") : (isZh ? "离线" : "Offline")}</span></td>
                      <td className="px-3 py-3">{node.schedulable ? (isZh ? "允许" : "Enabled") : (isZh ? "已暂停" : "Paused")}</td>
                      <td className="px-3 py-3 font-mono">{node.reservedCpu} / {node.cpu}</td>
                      <td className="px-3 py-3 font-mono">{node.reservedMemoryMb} / {node.memoryMb} MB</td>
                      <td className="px-4 py-3 text-right font-mono">{node.pendingTasks}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
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
