"use client";

import { useEffect, useState, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Globe, ChevronDown, Check } from "lucide-react";
import { listRegions } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export const REGION_STORAGE_KEY = "gamepanel.selected_region";

export function useCurrentRegion() {
  const [region, setRegionState] = useState<string>("all");

  useEffect(() => {
    const stored = window.localStorage.getItem(REGION_STORAGE_KEY);
    if (stored) {
      setRegionState(stored);
    }

    const handler = (e: Event) => {
      const custom = e as CustomEvent<string>;
      setRegionState(custom.detail);
    };
    window.addEventListener("regionChange", handler);
    return () => window.removeEventListener("regionChange", handler);
  }, []);

  const setRegion = (next: string) => {
    setRegionState(next);
    window.localStorage.setItem(REGION_STORAGE_KEY, next);
    window.dispatchEvent(new CustomEvent("regionChange", { detail: next }));
  };

  return { region, setRegion };
}

export function RegionSwitcher() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { region, setRegion } = useCurrentRegion();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const { data: regions = [] } = useQuery({
    queryKey: ["cloud-regions"],
    queryFn: listRegions,
    staleTime: 60000,
  });

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const activeRegionObj = regions.find((r) => r.id === region);

  const displayLabel = region === "all"
    ? (isZh ? "全部可用区 (All Regions)" : "All Regions")
    : activeRegionObj
    ? `${activeRegionObj.flag} ${isZh ? activeRegionObj.name : activeRegionObj.nameEn}`
    : region;

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 text-xs font-medium text-slate-200 hover:border-slate-700 hover:bg-slate-900 transition focus:outline-none focus:ring-1 focus:ring-panel-green/50"
      >
        <Globe className="size-3.5 text-sky-400 shrink-0" />
        <span className="truncate max-w-[130px]">{displayLabel}</span>
        <ChevronDown className={cn("size-3 text-slate-400 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="absolute left-0 mt-1.5 z-50 w-64 rounded-xl border border-slate-700 bg-slate-900 p-2 shadow-2xl ring-1 ring-white/10 backdrop-blur-xl">
          <div className="px-2 py-1.5 text-[11px] font-semibold text-slate-400 border-b border-slate-800 mb-1">
            {isZh ? "公有云多区域 (Multi-Region / Cell)" : "Global Cloud Regions"}
          </div>

          <button
            type="button"
            onClick={() => {
              setRegion("all");
              setOpen(false);
            }}
            className={cn(
              "flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-xs transition",
              region === "all" ? "bg-slate-800 font-semibold text-white" : "text-slate-300 hover:bg-slate-800/60"
            )}
          >
            <div className="flex items-center gap-2">
              <span className="text-base">🌐</span>
              <span>{isZh ? "全部可用区" : "All Regions"}</span>
            </div>
            {region === "all" && <Check className="size-3.5 text-panel-green" />}
          </button>

          {regions.map((reg) => {
            const isSelected = region === reg.id;
            return (
              <button
                key={reg.id}
                type="button"
                onClick={() => {
                  setRegion(reg.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-xs transition mt-0.5",
                  isSelected ? "bg-slate-800 font-semibold text-white" : "text-slate-300 hover:bg-slate-800/60"
                )}
              >
                <div className="flex items-center gap-2">
                  <span className="text-base">{reg.flag}</span>
                  <div className="text-left">
                    <div className="text-slate-100">{isZh ? reg.name : reg.nameEn}</div>
                    <div className="text-[10px] text-slate-400 font-mono">
                      {reg.nodeCount > 0 ? (isZh ? `${reg.nodeCount} 台计算节点` : `${reg.nodeCount} node(s)`) : (isZh ? "就绪可调度" : "Ready")}
                    </div>
                  </div>
                </div>
                {isSelected && <Check className="size-3.5 text-panel-green" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
