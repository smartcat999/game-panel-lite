"use client";

import { useQuery } from "@tanstack/react-query";
import { PlatformAccessGuard } from "@/components/platform-access-guard";
import { ConsolePageHeader } from "@/components/console-page-header";
import { listOrganizations } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function PlatformOrganizationsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const organizations = useQuery({ queryKey: ["platform", "organizations"], queryFn: listOrganizations, retry: false });

  return (
    <PlatformAccessGuard>
      <div className="space-y-3">
        <ConsolePageHeader title={isZh ? "租户管理" : "Tenant management"} />
        <div className="overflow-x-auto rounded-xl border bg-white micro-border subtle-elevation">
          <table className="min-w-[620px] w-full text-left text-xs">
            <thead className="border-b bg-slate-50/70 text-[10px] uppercase tracking-wider text-slate-400">
              <tr><th className="px-4 py-2.5">{isZh ? "租户" : "Tenant"}</th><th className="px-4 py-2.5">{isZh ? "标识" : "Identifier"}</th><th className="px-4 py-2.5">{isZh ? "套餐" : "Plan"}</th><th className="px-4 py-2.5 text-right">{isZh ? "余额" : "Balance"}</th></tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {(organizations.data ?? []).map((organization) => (
                <tr key={organization.id}>
                  <td className="px-4 py-3 font-semibold text-slate-800">{organization.name}</td>
                  <td className="px-4 py-3 font-mono text-[11px] text-slate-500">{organization.slug}</td>
                  <td className="px-4 py-3 text-slate-600">{organization.plan}</td>
                  <td className="px-4 py-3 text-right font-mono text-slate-600">{organization.credits ?? 0}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {!organizations.isLoading && organizations.data?.length === 0 ? <p className="p-6 text-center text-xs text-slate-400">{isZh ? "暂无租户" : "No tenants"}</p> : null}
        </div>
      </div>
    </PlatformAccessGuard>
  );
}
