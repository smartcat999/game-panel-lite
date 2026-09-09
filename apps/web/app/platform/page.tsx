"use client";

import Link from "next/link";
import { ArrowRight, Building2, MapPinned, Server } from "lucide-react";
import { PlatformScopeGuard } from "@/components/platform-scope-guard";
import { ConsolePageHeader } from "@/components/console-page-header";
import { useI18n } from "@/lib/i18n";

export default function PlatformOverviewPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  return (
    <PlatformScopeGuard>
      <div className="space-y-3">
        <ConsolePageHeader title={isZh ? "平台概览" : "Platform overview"} />
        <div className="rounded-xl border bg-white p-5 micro-border subtle-elevation">
          <h2 className="text-base font-semibold text-slate-900">{isZh ? "全局控制面" : "Global control plane"}</h2>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-500">
            {isZh
              ? "管理租户、全局实例与区域目录。节点、容量和执行状态属于区域运维范围。"
              : "Manage tenants, global instances, and the Region directory. Nodes, capacity, and execution state remain within Region operations."}
          </p>
        </div>
        <div className="grid gap-3 md:grid-cols-3">
          <OverviewLink href="/platform/organizations" icon={<Building2 className="size-4" />} title={isZh ? "租户管理" : "Tenant management"} description={isZh ? "组织、成员关系与资源归属" : "Organizations, memberships, and ownership"} />
          <OverviewLink href="/servers" icon={<Server className="size-4" />} title={isZh ? "全局实例" : "Global instances"} description={isZh ? "逻辑实例与租户归属" : "Logical instances and tenant ownership"} />
          <OverviewLink href="/platform/regions" icon={<MapPinned className="size-4" />} title={isZh ? "区域运维" : "Region operations"} description={isZh ? "区域目录、创建准入与状态摘要" : "Region directory, create admission, and status summaries"} />
        </div>
      </div>
    </PlatformScopeGuard>
  );
}

function OverviewLink({ href, icon, title, description }: { href: string; icon: React.ReactNode; title: string; description: string }) {
  return (
    <Link href={href} className="group rounded-xl border bg-white p-4 micro-border subtle-elevation transition hover:border-slate-300">
      <div className="flex items-center justify-between text-slate-500">
        {icon}
        <ArrowRight className="size-3.5 transition group-hover:translate-x-0.5" />
      </div>
      <p className="mt-4 text-sm font-semibold text-slate-900">{title}</p>
      <p className="mt-1 text-[11px] leading-4 text-slate-400">{description}</p>
    </Link>
  );
}
