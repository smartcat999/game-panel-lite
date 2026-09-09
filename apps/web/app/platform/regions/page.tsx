"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { PlatformScopeGuard } from "@/components/platform-scope-guard";
import { ConsolePageHeader } from "@/components/console-page-header";
import { listRegions } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { regionDisplayName } from "@/lib/region-display";

export default function PlatformRegionsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const regions = useQuery({ queryKey: ["platform", "regions"], queryFn: listRegions, retry: false });

  return (
    <PlatformScopeGuard>
      <div className="space-y-3">
        <ConsolePageHeader title={isZh ? "区域与节点" : "Regions and nodes"} />
        <div className="overflow-x-auto rounded-xl border bg-white micro-border subtle-elevation">
          <table className="min-w-[620px] w-full text-left text-xs">
            <thead className="border-b bg-slate-50/70 text-[10px] uppercase tracking-wider text-slate-400">
              <tr><th className="px-4 py-2.5">{isZh ? "区域" : "Region"}</th><th className="px-4 py-2.5">{isZh ? "区域 ID" : "Region ID"}</th><th className="px-4 py-2.5">{isZh ? "创建准入" : "Create admission"}</th><th className="px-4 py-2.5 text-right">{isZh ? "查看" : "View"}</th></tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {(regions.data ?? []).map((region) => (
                <tr key={region.id}>
                  <td className="px-4 py-3 font-semibold text-slate-800">{regionDisplayName(region, locale)}</td>
                  <td className="px-4 py-3 font-mono text-[11px] text-slate-500">{region.id}</td>
                  <td className="px-4 py-3 text-slate-600">{region.acceptingCreates ? (isZh ? "开放" : "Open") : (isZh ? "关闭" : "Closed")}</td>
                  <td className="px-4 py-3 text-right"><Link href={`/platform/regions/${encodeURIComponent(region.id)}`} className="inline-flex items-center gap-1 font-medium text-emerald-700 hover:text-emerald-800">{isZh ? "区域运维" : "Region operations"}<ArrowRight className="size-3" /></Link></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </PlatformScopeGuard>
  );
}
