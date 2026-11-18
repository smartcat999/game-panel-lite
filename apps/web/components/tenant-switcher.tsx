"use client";

import { useQuery } from "@tanstack/react-query";
import { Building2, Check, ChevronsUpDown, Cpu, HardDrive } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { listOrganizations, getOrganizationUsage } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function TenantSwitcher() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const [open, setOpen] = useState(false);
  const [selectedOrgId, setSelectedOrgId] = useState<string>("default-org");
  const containerRef = useRef<HTMLDivElement>(null);

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: listOrganizations,
    retry: false,
    staleTime: 60000
  });

  const orgs = orgsQuery.data ?? [];
  const currentOrg = orgs.find((o) => o.id === selectedOrgId) || orgs[0];

  useEffect(() => {
    if (orgs.length > 0 && orgs[0] && !orgs.some((o) => o.id === selectedOrgId)) {
      setSelectedOrgId(orgs[0].id);
    }
  }, [orgs, selectedOrgId]);

  useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    if (open) {
      window.addEventListener("pointerdown", handlePointerDown);
    }
    return () => window.removeEventListener("pointerdown", handlePointerDown);
  }, [open]);

  const usageQuery = useQuery({
    queryKey: ["tenant-usage", currentOrg?.id],
    queryFn: () => getOrganizationUsage(currentOrg?.id || "default-org"),
    enabled: Boolean(currentOrg?.id),
    refetchInterval: 30000
  });

  const usage = usageQuery.data;

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2.5 rounded-lg border border-slate-800 bg-slate-950 px-3 py-1.5 text-xs font-medium text-slate-200 transition hover:border-slate-600 hover:bg-slate-900 focus:outline-none focus:ring-1 focus:ring-panel-green/50"
      >
        <div className="flex size-5 items-center justify-center rounded bg-panel-green/15 text-panel-green">
          <Building2 className="size-3.5" />
        </div>
        <div className="flex flex-col text-left">
          <span className="max-w-[130px] truncate font-semibold leading-tight text-white">
            {currentOrg?.name || (isZh ? "默认工作区" : "Default Workspace")}
          </span>
          <span className="text-[10px] text-panel-green font-mono">
            {currentOrg?.plan?.toUpperCase() || "PRO"} {isZh ? "套餐" : "PLAN"}
          </span>
        </div>
        <ChevronsUpDown className="size-3.5 text-slate-400" />
      </button>

      {open && (
        <div className="absolute left-0 top-full z-50 mt-2 w-72 rounded-xl border border-slate-700 bg-[#0d131f] p-3 shadow-2xl ring-1 ring-white/10">
          <div className="px-1 py-1 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
            {isZh ? "工作区与租户" : "Workspaces / Tenants"}
          </div>
          <div className="mt-1.5 space-y-1">
            {orgs.map((org) => {
              const isSelected = (currentOrg?.id || "default-org") === org.id;
              return (
                <button
                  key={org.id}
                  type="button"
                  onClick={() => {
                    setSelectedOrgId(org.id);
                    setOpen(false);
                  }}
                  className={cn(
                    "flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-xs transition",
                    isSelected
                      ? "border border-panel-green/40 bg-panel-green/10 font-semibold text-panel-green"
                      : "border border-transparent bg-slate-900/60 text-slate-300 hover:border-slate-700 hover:bg-slate-800"
                  )}
                >
                  <div className="flex items-center gap-2">
                    <Building2 className="size-3.5 text-slate-400" />
                    <span className="truncate">{org.name}</span>
                  </div>
                  {isSelected && <Check className="size-3.5 text-panel-green" />}
                </button>
              );
            })}
          </div>

          {/* Quota overview strip */}
          {usage && usage.quota && (
            <div className="mt-3 rounded-lg border border-slate-800 bg-[#060911] p-3 text-[11px]">
              <div className="flex items-center justify-between text-slate-300">
                <span className="font-medium">{isZh ? "配额使用率" : "Quota Usage"}</span>
                <span className="font-mono text-white font-bold">
                  {usage.totalServers} / {usage.quota.maxServers} {isZh ? "台实例" : "Servers"}
                </span>
              </div>
              <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
                <div
                  className="h-full rounded-full bg-panel-green transition-all"
                  style={{ width: `${Math.min(100, Math.round((usage.totalServers / (usage.quota.maxServers || 1)) * 100))}%` }}
                />
              </div>
              <div className="mt-2.5 flex items-center justify-between text-[10px] text-slate-400">
                <span className="flex items-center gap-1">
                  <Cpu className="size-3 text-sky-400" /> {usage.usedCpuCores.toFixed(1)} / {usage.quota.maxCpuCores} {isZh ? "核" : "Cores"}
                </span>
                <span className="flex items-center gap-1">
                  <HardDrive className="size-3 text-purple-400" /> {Math.round(usage.usedMemoryMb / 1024)}G / {Math.round(usage.quota.maxMemoryMb / 1024)}G RAM
                </span>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
