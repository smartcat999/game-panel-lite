"use client";

import { useEffect } from "react";
import { Check } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface FloatingSaveDockProps {
  show: boolean;
  saving?: boolean;
  onSave: () => void;
  onDiscard: () => void;
  message?: string;
  className?: string;
}

export function FloatingSaveDock({
  show,
  saving = false,
  onSave,
  onDiscard,
  message,
  className
}: FloatingSaveDockProps) {
  const { locale } = useI18n();
  const isZh = locale === "zh";

  // Cmd+S / Ctrl+S keyboard shortcut
  useEffect(() => {
    if (!show) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        onSave();
      }
      if (e.key === "Escape") {
        e.preventDefault();
        onDiscard();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [show, onSave, onDiscard]);

  return (
    <div
      className={cn(
        "fixed bottom-6 left-1/2 -translate-x-1/2 z-40 flex items-center gap-3 rounded-full bg-slate-900/95 py-1.5 pl-4 pr-2 text-xs text-white shadow-2xl backdrop-blur-md border border-slate-700/80 subtle-elevation transition-all duration-200",
        show
          ? "translate-y-0 opacity-100 pointer-events-auto"
          : "translate-y-12 opacity-0 pointer-events-none",
        className
      )}
    >
      <div className="flex items-center gap-2">
        <span className="size-2 rounded-full bg-amber-400 animate-pulse shrink-0" />
        <span className="text-xs font-medium text-slate-200">
          {message ?? (isZh ? "检测到未保存的参数更改" : "Unsaved parameter changes detected")}
        </span>
      </div>

      <div className="h-3 w-px bg-slate-700 shrink-0" />

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={onDiscard}
          disabled={saving}
          className="rounded-full px-2.5 py-1 text-[11px] font-medium text-slate-300 hover:bg-slate-800 hover:text-white transition disabled:opacity-50"
        >
          {isZh ? "放弃" : "Discard"}
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={saving}
          className="flex items-center gap-1 rounded-full bg-emerald-500 px-3 py-1 text-[11px] font-bold text-slate-950 hover:bg-emerald-400 transition shadow-xs disabled:opacity-50"
        >
          <Check className="size-3.5 stroke-[2.5]" />
          <span>{saving ? (isZh ? "保存中..." : "Saving...") : (isZh ? "保存 (⌘S)" : "Save (⌘S)")}</span>
        </button>
      </div>
    </div>
  );
}
