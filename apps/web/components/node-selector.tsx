"use client";

import { useQuery } from "@tanstack/react-query";
import { Check, Cpu, Globe, HardDrive, Radio, Server } from "lucide-react";
import { listComputeNodes } from "@/lib/api";
import { cn } from "@/lib/utils";

export function NodeSelector({
  value,
  onChange
}: {
  value?: string;
  onChange: (nodeId: string) => void;
}) {
  const nodesQuery = useQuery({
    queryKey: ["compute-nodes"],
    queryFn: listComputeNodes,
    retry: false,
    staleTime: 60000
  });

  const nodes = nodesQuery.data ?? [];
  const selectedNodeId = value || (nodes[0]?.id ?? "node-local");

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium text-slate-200">
          Target Compute Node / Region
        </label>
        <span className="text-xs text-slate-500 font-mono">
          {nodes.length} Nodes Available
        </span>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {nodes.map((node) => {
          const isSelected = selectedNodeId === node.id;
          const isOnline = node.status === "online";

          return (
            <button
              key={node.id}
              type="button"
              onClick={() => onChange(node.id)}
              className={cn(
                "group relative flex flex-col justify-between rounded-xl border p-4 text-left transition-all duration-200 focus:outline-none",
                isSelected
                  ? "border-panel-green bg-gradient-to-b from-panel-green/10 to-slate-950/90 shadow-lg shadow-panel-green/10"
                  : "border-panel-line bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900/60"
              )}
            >
              <div>
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={cn(
                        "flex size-8 items-center justify-center rounded-lg border",
                        isSelected
                          ? "border-panel-green/40 bg-panel-green/20 text-panel-green"
                          : "border-panel-line bg-slate-900 text-slate-400"
                      )}
                    >
                      {node.isLocal ? <Server className="size-4" /> : <Globe className="size-4" />}
                    </div>
                    <div>
                      <h4 className="truncate font-semibold text-slate-100 text-xs sm:text-sm">
                        {node.name}
                      </h4>
                      <span className="text-[11px] text-slate-500 font-mono">
                        {node.region} · {node.publicIp}
                      </span>
                    </div>
                  </div>

                  <span
                    className={cn(
                      "size-2 rounded-full ring-2 ring-opacity-20",
                      isOnline ? "bg-panel-green ring-panel-green" : "bg-panel-gold ring-panel-gold"
                    )}
                  />
                </div>

                <div className="mt-3.5 flex items-center justify-between border-t border-panel-line/40 pt-2.5 text-[11px] text-slate-400">
                  <span className="flex items-center gap-1">
                    <Cpu className="size-3 text-slate-500" />
                    {node.cpuCores} Cores
                  </span>
                  <span className="flex items-center gap-1">
                    <HardDrive className="size-3 text-slate-500" />
                    {Math.round(node.memoryTotalMb / 1024)} GB RAM
                  </span>
                  <span className="flex items-center gap-1">
                    <Radio className="size-3 text-slate-500" />
                    {node.runningCount} Running
                  </span>
                </div>
              </div>

              {isSelected ? (
                <div className="absolute right-2.5 top-2.5 flex size-4 items-center justify-center rounded-full bg-panel-green text-slate-950">
                  <Check className="size-3 stroke-[3]" />
                </div>
              ) : null}
            </button>
          );
        })}
      </div>
    </div>
  );
}
