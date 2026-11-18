"use client";

import { useQuery } from "@tanstack/react-query";
import { Cpu, HardDrive } from "lucide-react";
import { getObservabilityMetrics } from "@/lib/api";
import { cn } from "@/lib/utils";

export function ClusterStatusPill() {
  const metricsQuery = useQuery({
    queryKey: ["observability-metrics"],
    queryFn: getObservabilityMetrics,
    retry: false,
    refetchInterval: 10000,
    staleTime: 5000
  });

  const host = metricsQuery.data?.host;
  const cpuPercent = host ? Math.round(host.totalCpuPercent) : 0;
  const memUsedMB = host ? host.totalMemoryMb : 0;
  const memTotalMB = host?.memoryLimitMb || 16384;
  const memUsedGB = (memUsedMB / 1024).toFixed(1);
  const memTotalGB = Math.round(memTotalMB / 1024);

  const isCpuHigh = cpuPercent > 80;

  return (
    <div className="hidden xl:flex items-center gap-2.5 rounded-lg border border-slate-800 bg-slate-950/70 px-2.5 py-1 text-[11px] text-slate-300 backdrop-blur-sm">
      <div className="flex items-center gap-1.5 font-mono">
        <span className={cn("size-1.5 rounded-full", isCpuHigh ? "bg-rose-500 animate-ping" : "bg-panel-green animate-pulse")} />
        <Cpu className="size-3 text-sky-400" />
        <span className={cn(isCpuHigh ? "text-rose-400 font-semibold" : "text-slate-200")}>
          {cpuPercent}%
        </span>
      </div>

      <div className="h-3 w-px bg-slate-800" />

      <div className="flex items-center gap-1.5 font-mono text-slate-300">
        <HardDrive className="size-3 text-purple-400" />
        <span>
          {memUsedGB}G <span className="text-slate-500">/ {memTotalGB}G</span>
        </span>
      </div>
    </div>
  );
}
