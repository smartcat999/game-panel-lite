"use client";

import { useEffect } from "react";
import { AlertTriangle, Bell, Clock, ExternalLink, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface IncidentDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function IncidentDrawer({ open, onClose }: IncidentDrawerProps) {
  const { locale } = useI18n();
  const isZh = locale === "zh";

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    if (open) {
      window.addEventListener("keydown", handleKeyDown);
    }
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const incidents = [
    {
      id: "inc-1",
      level: "P1",
      title: isZh ? "实例停服宽限期提醒" : "Server Grace Period Warning",
      server: "terraria-hardcore-01",
      message: isZh
        ? "实例处于 7 天停服保护宽限期（剩余 3 天），到期后将自动释放计算资源并归档。"
        : "Instance is in 7-day grace period (3 days remaining). Compute will be released upon expiry.",
      time: isZh ? "10 分钟前" : "10m ago",
      tone: "danger" as const
    },
    {
      id: "inc-2",
      level: "P2",
      title: isZh ? "自动快照备份完成" : "Scheduled Snapshot Completed",
      server: "calamity-infernum-03",
      message: isZh
        ? "每日凌晨全量世界快照 (HardcoreWorld01.wld) 备份完毕，用时 1.2s，大小 14.8 MB。"
        : "Daily world snapshot completed in 1.2s (14.8 MB).",
      time: isZh ? "2 小时前" : "2h ago",
      tone: "info" as const
    }
  ];

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="fixed inset-0 bg-slate-900/20 backdrop-blur-xs transition-opacity" onClick={onClose} />
      <aside className="relative z-10 flex h-full w-full max-w-sm flex-col border-l micro-border bg-white shadow-2xl animate-in slide-in-from-right duration-200">
        <div className="flex h-12 items-center justify-between border-b micro-border px-4 shrink-0">
          <div className="flex items-center gap-2">
            <div className="flex size-2 rounded-full bg-amber-500 animate-pulse" />
            <span className="text-xs font-bold text-slate-900">
              {isZh ? "告警与通知中心" : "Incidents & Alerts"}
            </span>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex size-6 items-center justify-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-slate-600 transition"
          >
            <X className="size-3.5" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-3 space-y-2.5 text-xs">
          {incidents.map((inc) => (
            <div
              key={inc.id}
              className={cn(
                "rounded-xl border p-3 space-y-1.5 transition",
                inc.tone === "danger"
                  ? "border-rose-200 bg-rose-50/40 text-rose-950"
                  : "border-slate-200 bg-slate-50/60 text-slate-900"
              )}
            >
              <div className="flex items-center justify-between">
                <span className="flex items-center gap-1.5 font-bold text-xs">
                  {inc.tone === "danger" ? (
                    <AlertTriangle className="size-3.5 text-rose-500 shrink-0" />
                  ) : (
                    <Bell className="size-3.5 text-slate-500 shrink-0" />
                  )}
                  <span>{inc.title}</span>
                </span>
                <span
                  className={cn(
                    "rounded px-1.5 py-0.2 font-mono text-[10px] font-bold",
                    inc.tone === "danger" ? "bg-rose-100 text-rose-700" : "bg-slate-200 text-slate-700"
                  )}
                >
                  {inc.level}
                </span>
              </div>
              <p className="text-[11px] text-slate-600 leading-relaxed">
                {isZh ? "实例 " : "Instance "}
                <code className="font-mono font-semibold text-slate-800">{inc.server}</code>
                {": "}
                {inc.message}
              </p>
              <div className="flex items-center justify-between pt-1 text-[10px] text-slate-400">
                <span className="flex items-center gap-1">
                  <Clock className="size-3" />
                  <span>{inc.time}</span>
                </span>
                <button
                  type="button"
                  onClick={onClose}
                  className="inline-flex items-center gap-1 font-medium text-slate-700 hover:text-slate-950 transition"
                >
                  <span>{isZh ? "查看详情" : "Details"}</span>
                  <ExternalLink className="size-2.5" />
                </button>
              </div>
            </div>
          ))}
        </div>
      </aside>
    </div>
  );
}
