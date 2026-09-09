"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { PlatformScopeGuard } from "@/components/platform-scope-guard";
import { ConsolePageHeader } from "@/components/console-page-header";
import { listComputeNodes, listRegions } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { regionDisplayName } from "@/lib/region-display";

export default function RegionOperationsPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const regionId = decodeURIComponent(params.id);
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const regions = useQuery({ queryKey: ["platform", "regions"], queryFn: listRegions, retry: false });
  const nodes = useQuery({ queryKey: ["platform", "regions", regionId, "nodes"], queryFn: () => listComputeNodes(regionId), retry: false });
  const region = regions.data?.find((item) => item.id === regionId);

  return (
    <PlatformScopeGuard>
      <div className="space-y-3">
        <ConsolePageHeader
          title={region ? regionDisplayName(region, locale) : regionId}
          action={
            <label className="flex items-center gap-2 text-[11px] text-slate-500">
              <span>{isZh ? "区域" : "Region"}</span>
              <select
                aria-label={isZh ? "切换区域" : "Switch Region"}
                value={regionId}
                onChange={(event) => router.push(`/platform/regions/${encodeURIComponent(event.target.value)}`)}
                className="h-7 rounded-md border border-slate-200 bg-white px-2 text-xs font-medium text-slate-700 outline-none focus:border-emerald-500"
              >
                {(regions.data ?? []).map((entry) => <option key={entry.id} value={entry.id}>{regionDisplayName(entry, locale)}</option>)}
              </select>
            </label>
          }
        />
        <div className="rounded-xl border bg-white px-4 py-3 micro-border subtle-elevation">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div><p className="text-[10px] uppercase tracking-wider text-slate-400">{isZh ? "区域运维" : "Region operations"}</p><p className="mt-1 text-xs text-slate-600">{isZh ? "节点、容量与执行状态由该区域负责。" : "This Region owns nodes, capacity, and execution state."}</p></div>
            <span className="text-xs font-medium text-slate-600">{region?.acceptingCreates ? (isZh ? "允许新建实例" : "Accepting new instances") : (isZh ? "已停止新建准入" : "Create admission closed")}</span>
          </div>
        </div>
        <div className="overflow-x-auto rounded-xl border bg-white micro-border subtle-elevation">
          <div className="border-b px-4 py-3"><h2 className="text-xs font-semibold text-slate-800">{isZh ? "节点" : "Nodes"}</h2></div>
          <table className="min-w-[620px] w-full text-left text-xs">
            <thead className="border-b bg-slate-50/70 text-[10px] uppercase tracking-wider text-slate-400"><tr><th className="px-4 py-2.5">{isZh ? "节点" : "Node"}</th><th className="px-4 py-2.5">{isZh ? "状态" : "Status"}</th><th className="px-4 py-2.5">{isZh ? "调度" : "Scheduling"}</th><th className="px-4 py-2.5 text-right">{isZh ? "资源" : "Resources"}</th></tr></thead>
            <tbody className="divide-y divide-slate-100">
              {(nodes.data ?? []).map((node) => <tr key={node.id}><td className="px-4 py-3"><span className="block font-semibold text-slate-800">{node.name}</span><span className="font-mono text-[10px] text-slate-400">{node.id}</span></td><td className="px-4 py-3 text-slate-600">{node.status}</td><td className="px-4 py-3 text-slate-600">{node.unschedulable ? (isZh ? "禁止调度" : "Cordoned") : (isZh ? "可调度" : "Schedulable")}</td><td className="px-4 py-3 text-right font-mono text-[11px] text-slate-500">{node.cpuCores} CPU · {node.memoryTotalMb} MB</td></tr>)}
            </tbody>
          </table>
          {!nodes.isLoading && nodes.data?.length === 0 ? <p className="p-6 text-center text-xs text-slate-400">{isZh ? "该区域暂无节点" : "No nodes in this Region"}</p> : null}
        </div>
      </div>
    </PlatformScopeGuard>
  );
}
