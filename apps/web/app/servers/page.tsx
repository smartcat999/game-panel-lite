"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { listGameServers, getSettings } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { ServerManagementTable } from "@/components/server-management-table";
import { DeployInstanceModal } from "@/components/deploy-instance-modal";

export default function ServersPage() {
  const { canCreateServer } = usePermissions();
  const [deployModalOpen, setDeployModalOpen] = useState(false);

  const serversQuery = useQuery({
    queryKey: ["game-servers"],
    queryFn: listGameServers,
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
      {/* Compact Header matching prototype */}
      <div className="h-11 bg-white border micro-border rounded-xl px-3.5 flex items-center justify-between subtle-elevation">
        <h1 className="text-xs font-bold text-slate-900 leading-none">Instances</h1>

        <div className="flex items-center gap-1.5">
          {canCreateServer ? (
            <button
              type="button"
              onClick={() => setDeployModalOpen(true)}
              className="h-7 px-2.5 bg-slate-900 hover:bg-slate-800 text-white text-xs font-semibold rounded-lg transition shadow-xs flex items-center gap-1 cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5 stroke-[2.5]" />
              <span>Deploy</span>
            </button>
          ) : (
            <div className="text-[11px] text-slate-400 font-mono bg-slate-50 border micro-border px-2 py-0.5 rounded-md">
              🔒 Read-Only
            </div>
          )}
        </div>
      </div>

      {/* Public Cloud High-Density Table */}
      <div className="bg-white border micro-border rounded-xl subtle-elevation overflow-hidden">
        <ServerManagementTable servers={servers} publicHost={settingsQuery.data?.publicHost} />
      </div>

      <DeployInstanceModal open={deployModalOpen} onClose={() => setDeployModalOpen(false)} />
    </div>
  );
}
