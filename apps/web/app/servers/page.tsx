"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LockKeyhole, Plus } from "lucide-react";
import { listGameServers, getSettings } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { ConsolePageHeader } from "@/components/console-page-header";
import { ServerManagementTable } from "@/components/server-management-table";
import { DeployInstanceModal } from "@/components/deploy-instance-modal";
import { useI18n } from "@/lib/i18n";
import { useConsoleContext } from "@/lib/console-context";

export default function ServersPage() {
  const { canCreateServer } = usePermissions();
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const { scope, currentOrganization } = useConsoleContext();
  const organizationId = scope.kind === "organization" ? scope.organizationId : undefined;
  const canDeployInScope = canCreateServer && Boolean(organizationId);

  const serversQuery = useQuery({
    queryKey: ["game-servers", scope.kind, organizationId ?? "all"],
    queryFn: () => listGameServers(organizationId),
    retry: false,
    refetchInterval: 5000
  });

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
    retry: false,
    staleTime: 5 * 60 * 1000
  });

  const servers = serversQuery.data ?? [];

  return (
    <div className="space-y-3">
      <ConsolePageHeader
        title={scope.kind === "platform" ? (isZh ? "全局实例" : "Global instances") : (isZh ? "实例" : "Instances")}
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
          ) : scope.kind === "organization" ? (
            <div className="flex items-center gap-1.5 rounded-md border bg-slate-50 px-2 py-1 text-[11px] text-slate-500 micro-border">
              <LockKeyhole className="size-3" />
              <span>{isZh ? "只读" : "Read only"}</span>
            </div>
          ) : null
        }
      />

      {/* Public Cloud High-Density Table */}
      <div className="bg-white border micro-border rounded-xl subtle-elevation overflow-hidden">
        <ServerManagementTable servers={servers} publicHost={settingsQuery.data?.publicHost} />
      </div>

      <DeployInstanceModal open={deployModalOpen} onClose={() => setDeployModalOpen(false)} organizationId={currentOrganization?.id} />
    </div>
  );
}
