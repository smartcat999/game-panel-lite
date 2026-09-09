"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Plus,
  RefreshCw,
  Search
} from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listGameServers, getSettings } from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { usePermissions } from "@/lib/permissions";
import { ServerManagementTable } from "@/components/server-management-table";
import { DeployInstanceModal } from "@/components/deploy-instance-modal";
import { cn } from "@/lib/utils";

export default function ServersPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const { canCreateServer } = usePermissions();

  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("all");

  const serversQuery = useQuery({
    queryKey: ["game-servers"],
    queryFn: listGameServers,
    retry: false,
    refetchInterval: 10000
  });

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
    retry: false,
    staleTime: 5 * 60 * 1000
  });

  const servers = serversQuery.data ?? [];
  const running = servers.filter((s) => gameServerStatus(s) === "running");
  const stopped = servers.filter((s) => gameServerStatus(s) === "stopped");

  const filteredServers = servers.filter((server) => {
    const status = gameServerStatus(server);
    if (statusFilter === "running" && status !== "running") return false;
    if (statusFilter === "stopped" && status !== "stopped") return false;

    if (!search.trim()) return true;
    const term = search.toLowerCase();
    return server.name.toLowerCase().includes(term) || server.id.toLowerCase().includes(term);
  });

  return (
    <div className="space-y-3.5">
      {/* Direct, Streamlined Top Bar (Zero redundant cards) */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
        {/* Title + Status Filter Tabs */}
        <div className="flex items-center gap-3">
          <h1 className="text-base font-bold text-slate-900 tracking-tight">
            {isZh ? "服务器" : "Servers"}
          </h1>

          <div className="flex items-center gap-1 bg-white border border-slate-200/80 rounded-lg p-0.5 shadow-2xs">
            <button
              onClick={() => setStatusFilter("all")}
              className={cn(
                "px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer",
                statusFilter === "all"
                  ? "bg-slate-900 text-white font-semibold shadow-xs"
                  : "text-slate-600 hover:text-slate-900 hover:bg-slate-50"
              )}
            >
              {isZh ? "全部" : "All"} <span className="opacity-70 font-mono text-[11px] ml-0.5">{servers.length}</span>
            </button>
            <button
              onClick={() => setStatusFilter("running")}
              className={cn(
                "px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer",
                statusFilter === "running"
                  ? "bg-emerald-600 text-white font-semibold shadow-xs"
                  : "text-slate-600 hover:text-slate-900 hover:bg-slate-50"
              )}
            >
              {isZh ? "运行中" : "Running"} <span className="opacity-70 font-mono text-[11px] ml-0.5">{running.length}</span>
            </button>
            <button
              onClick={() => setStatusFilter("stopped")}
              className={cn(
                "px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer",
                statusFilter === "stopped"
                  ? "bg-slate-700 text-white font-semibold shadow-xs"
                  : "text-slate-600 hover:text-slate-900 hover:bg-slate-50"
              )}
            >
              {isZh ? "已停止" : "Stopped"} <span className="opacity-70 font-mono text-[11px] ml-0.5">{stopped.length}</span>
            </button>
          </div>
        </div>

        {/* Search + Refresh + Deploy Action */}
        <div className="flex items-center gap-2">
          <div className="relative min-w-[180px] sm:min-w-[220px]">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-slate-400" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={isZh ? "搜索服务器..." : "Search servers..."}
              className="h-8 w-full rounded-lg border border-slate-200/80 bg-white pl-8 pr-3 text-xs text-slate-900 placeholder:text-slate-400 focus:border-emerald-500 focus:outline-none shadow-2xs"
            />
          </div>

          <button
            type="button"
            onClick={() => queryClient.invalidateQueries({ queryKey: ["game-servers"] })}
            title={isZh ? "刷新列表" : "Refresh"}
            className="flex size-8 items-center justify-center rounded-lg border border-slate-200/80 bg-white text-slate-500 hover:bg-slate-50 hover:text-slate-800 shadow-2xs transition cursor-pointer shrink-0"
          >
            <RefreshCw className={cn("size-3.5", serversQuery.isFetching && "animate-spin text-emerald-600")} />
          </button>

          {canCreateServer && (
            <button
              type="button"
              onClick={() => setDeployModalOpen(true)}
              className="flex h-8 items-center gap-1.5 rounded-lg bg-emerald-600 px-3 text-xs font-semibold text-white shadow-xs hover:bg-emerald-700 transition cursor-pointer shrink-0"
            >
              <Plus className="size-3.5" />
              <span>{isZh ? "新建实例" : "New Server"}</span>
            </button>
          )}
        </div>
      </div>

      {/* Clean Single Server Table Container */}
      <div className="rounded-xl border border-slate-200/80 bg-white shadow-2xs overflow-hidden">
        <ServerManagementTable
          servers={filteredServers}
          publicHost={settingsQuery.data?.publicHost}
        />
      </div>

      {/* Deploy Modal */}
      <DeployInstanceModal
        open={deployModalOpen}
        onClose={() => setDeployModalOpen(false)}
      />
    </div>
  );
}
