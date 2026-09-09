"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LockKeyhole, Plus } from "lucide-react";
import { listTenantInstanceViews } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { ConsolePageHeader } from "@/components/console-page-header";
import { ServerManagementTable } from "@/components/server-management-table";
import { DeployInstanceModal } from "@/components/deploy-instance-modal";
import { useI18n } from "@/lib/i18n";
import { useTenantContext } from "@/lib/tenant-context";

export default function ServersPage() {
  const { canCreateServer } = usePermissions();
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const { currentOrganization } = useTenantContext();
  const organizationId = currentOrganization?.id;
  const canDeployInScope = canCreateServer && Boolean(organizationId);

  const instancesQuery = useQuery({
    queryKey: ["tenant-instances", organizationId],
    queryFn: () => listTenantInstanceViews(organizationId!),
    enabled: Boolean(organizationId),
    retry: false,
    refetchInterval: 5000
  });
  const instances = instancesQuery.data?.items ?? [];

  return (
    <div className="space-y-3">
      <ConsolePageHeader
        title={isZh ? "实例" : "Instances"}
        action={
          canDeployInScope ? (
            <button
              type="button"
              onClick={() => setDeployModalOpen(true)}
              className="h-7 px-2.5 bg-slate-900 hover:bg-slate-800 text-white text-xs font-semibold rounded-lg transition shadow-xs flex items-center gap-1 cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5 stroke-[2.5]" />
              <span>{isZh ? "部署" : "Deploy"}</span>
            </button>
          ) : organizationId ? (
            <div className="flex items-center gap-1.5 rounded-md border bg-slate-50 px-2 py-1 text-[11px] text-slate-500 micro-border">
              <LockKeyhole className="size-3" />
              <span>{isZh ? "只读" : "Read only"}</span>
            </div>
          ) : null
        }
      />

      <div className="bg-white border micro-border rounded-xl subtle-elevation overflow-hidden">
        {instancesQuery.isLoading ? (
          <div className="p-10 text-center text-xs text-slate-400">
            {isZh ? "正在读取实例…" : "Loading instances…"}
          </div>
        ) : instancesQuery.isError ? (
          <div className="p-10 text-center text-xs text-slate-500">
            {isZh ? "实例列表暂时无法读取。" : "Instances are temporarily unavailable."}
          </div>
        ) : (
          <ServerManagementTable instances={instances} />
        )}
      </div>

      <DeployInstanceModal open={deployModalOpen} onClose={() => setDeployModalOpen(false)} organizationId={currentOrganization?.id} />
    </div>
  );
}
