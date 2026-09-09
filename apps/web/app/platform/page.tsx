"use client";

import Link from "next/link";
import { ArrowRight, Building2, MapPinned, Server } from "lucide-react";
import { PlatformAccessGuard } from "@/components/platform-access-guard";
import { ConsolePageHeader } from "@/components/console-page-header";
import { useI18n } from "@/lib/i18n";

export default function PlatformOverviewPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  return (
    <PlatformAccessGuard>
      <div className="space-y-3">
        <ConsolePageHeader title={isZh ? "平台概览" : "Platform overview"} />
        <div className="rounded-xl border bg-white p-5 micro-border subtle-elevation">
          <h2 className="text-base font-semibold text-slate-900">{isZh ? "平台管理中心" : "Platform administration"}</h2>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-500">
            {isZh
              ? "业务控制面管理租户关系和资源交付；基础设施运维管理区域承载和实际执行。两者通过逻辑实例与区域部署关联。"
              : "Business control manages tenant relationships and service delivery. Infrastructure operations manage regional capacity and execution. Logical instances connect the two."}
          </p>
        </div>
        <section className="space-y-2">
          <div>
            <h2 className="text-xs font-semibold text-slate-800">{isZh ? "业务控制面" : "Business control plane"}</h2>
            <p className="mt-0.5 text-[11px] text-slate-500">{isZh ? "处理全局业务对象，不建立当前租户上下文。" : "Work with global business records without adopting a current tenant context."}</p>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <OverviewLink href="/platform/organizations" icon={<Building2 className="size-4" />} title={isZh ? "租户管理" : "Tenant management"} description={isZh ? "组织、成员关系与资源归属" : "Organizations, memberships, and ownership"} />
            <OverviewLink href="/platform/instances" icon={<Server className="size-4" />} title={isZh ? "交付追踪" : "Delivery tracking"} description={isZh ? "逻辑实例、租户归属与部署位置" : "Logical instances, tenant ownership, and placement"} />
          </div>
        </section>
        <section className="space-y-2">
          <div>
            <h2 className="text-xs font-semibold text-slate-800">{isZh ? "基础设施运维" : "Infrastructure operations"}</h2>
            <p className="mt-0.5 text-[11px] text-slate-500">{isZh ? "进入具体区域后管理节点、容量、部署与任务。" : "Enter a Region to operate nodes, capacity, deployments, and tasks."}</p>
          </div>
          <OverviewLink href="/platform/regions" icon={<MapPinned className="size-4" />} title={isZh ? "区域运维" : "Region operations"} description={isZh ? "区域目录、创建准入与状态摘要" : "Region directory, create admission, and status summaries"} />
        </section>
      </div>
    </PlatformAccessGuard>
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
