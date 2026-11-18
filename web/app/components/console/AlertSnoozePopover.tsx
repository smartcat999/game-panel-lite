"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Bell, BellOff } from "lucide-react";
import { focusRing } from "./ui";
import type { AlertItem } from "../../console/lib/types";
import { formatSilenceRemaining } from "../../console/lib/observability";

interface PopoverCoords {
  top?: number;
  bottom?: number;
  right: number;
}

function useAnchoredPopover(estimatedHeight: number) {
  const [open, setOpen] = useState(false);
  const [coords, setCoords] = useState<PopoverCoords | null>(null);
  const triggerRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const updatePosition = useCallback(() => {
    const el = triggerRef.current;
    if (!el || typeof window === "undefined") return;
    const rect = el.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const openUpward = spaceBelow < estimatedHeight && rect.top > spaceBelow;
    setCoords({
      ...(openUpward
        ? { bottom: Math.max(8, window.innerHeight - rect.top + 8) }
        : { top: rect.bottom + 8 }),
      right: Math.max(8, window.innerWidth - rect.right),
    });
  }, [estimatedHeight]);

  useEffect(() => {
    if (!open) return;
    updatePosition();
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef.current?.contains(target) && !panelRef.current?.contains(target)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", updatePosition, true);
    window.addEventListener("resize", updatePosition);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", updatePosition, true);
      window.removeEventListener("resize", updatePosition);
    };
  }, [open, updatePosition]);

  return { open, setOpen, coords, triggerRef, panelRef, updatePosition };
}

const VALUE_LABELS: Record<string, string> = {
  "Phase = Failed": "启动或运行异常",
  "phase == Failed": "启动或运行异常",
  "ExitCode 137 (SIGKILL)": "内存超限退出 (OOM)",
};

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function formatPopoverHeadline(primary?: AlertItem): string {
  if (!primary) return "当前资源存在异常告警";
  let title = primary.title.trim();
  const target = primary.targetName?.trim();
  if (target) {
    const escaped = escapeRegExp(target);
    title = title
      .replace(new RegExp(`^(?:服务器|计算节点)\\s*${escaped}\\s*(?:·\\s*)?`), "")
      .replace(new RegExp(`^${escaped}\\s*(?:·\\s*)?`), "")
      .trim();
  }
  const rawVal = primary.value?.trim() || "";
  const cleanVal = VALUE_LABELS[rawVal] || rawVal;
  if (!cleanVal) return title || "当前资源存在异常告警";
  if (!title) return cleanVal;
  if (title.includes(cleanVal)) return title;
  if (cleanVal.includes(title)) return cleanVal;
  return `${title} · ${cleanVal}`;
}

export function ResourceAlertBellButton({
  alerts,
  muted,
  silencedUntil,
  surface = "dark",
  readOnly,
  disabled,
  onSnooze,
  onResume,
}: {
  alerts: AlertItem[];
  muted: boolean;
  silencedUntil?: string;
  surface?: "dark" | "light";
  readOnly?: boolean;
  disabled?: boolean;
  onSnooze: (hours: 1 | 4 | 24) => void;
  onResume: () => void;
}) {
  const { open, setOpen, coords, triggerRef, panelRef, updatePosition } =
    useAnchoredPopover(112);

  if (alerts.length === 0 && !muted) {
    return null;
  }

  const primary = alerts[0];
  const headline = formatPopoverHeadline(primary);

  const triggerStyle =
    surface === "light"
      ? muted
        ? "border border-[#E3E6F2] bg-[#F1F3FB] text-[#5B6075] hover:bg-[#E6E9F5]"
        : "border border-[#E3E6F2] bg-white text-[#14161F] shadow-sm hover:bg-[#F1F3FB]"
      : muted
        ? "border border-white/30 bg-white/20 text-white backdrop-blur hover:bg-white/30"
        : "bg-white text-[#14161F] shadow-sm hover:bg-[#F1F3FB]";

  const dotRing = surface === "light" ? "ring-white" : "ring-[#0F1222]/70";

  return (
    <div
      ref={triggerRef}
      className="relative inline-flex"
      onClick={(e) => e.stopPropagation()}
    >
      <button
        type="button"
        disabled={disabled}
        aria-label={muted ? "告警免打扰设置" : "暂停告警通知"}
        aria-expanded={open}
        onClick={() => {
          if (!open) updatePosition();
          setOpen((prev) => !prev);
        }}
        className={`relative inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${triggerStyle} ${focusRing}`}
      >
        {muted ? (
          <BellOff className="h-4 w-4" aria-hidden="true" />
        ) : (
          <>
            <Bell className="h-4 w-4" aria-hidden="true" />
            <span
              aria-hidden="true"
              className={`absolute right-0.5 top-0.5 h-2.5 w-2.5 rounded-full bg-[#D92D20] ring-2 ${dotRing}`}
            />
          </>
        )}
      </button>

      {open &&
        coords &&
        typeof document !== "undefined" &&
        createPortal(
          <div
            ref={panelRef}
            style={coords}
            onClick={(e) => e.stopPropagation()}
            className="fixed z-50 min-w-[248px] max-w-[340px] rounded-2xl border border-[#E3E6F2] bg-white shadow-[0_20px_40px_-20px_rgba(20,22,31,0.35)]"
          >
            {muted ? (
              <div className="flex items-center justify-between gap-4 px-4 py-3 text-xs">
                <span className="font-medium text-[#14161F]">
                  {silencedUntil ? `免打扰${formatSilenceRemaining(silencedUntil)}` : "免打扰中"}
                </span>
                {readOnly ? (
                  <span className="text-[#5B6075]">平台全局</span>
                ) : (
                  <button
                    type="button"
                    disabled={disabled}
                    onClick={() => {
                      setOpen(false);
                      onResume();
                    }}
                    className={`rounded-lg px-2 py-1 font-semibold text-[#2B3AE8] transition-colors hover:bg-[#EEF1FF] ${focusRing}`}
                  >
                    恢复
                  </button>
                )}
              </div>
            ) : (
              <div>
                <div className="flex items-center gap-2 border-b border-[#EEF0F7] px-3.5 py-2.5 text-xs">
                  <span
                    className="h-2 w-2 shrink-0 rounded-full bg-[#D92D20]"
                    aria-hidden="true"
                  />
                  <span className="truncate font-semibold text-[#B42318]">{headline}</span>
                  {alerts.length > 1 && (
                    <span className="shrink-0 text-[#5B6075]">+{alerts.length - 1}</span>
                  )}
                </div>
                <div className="flex items-center justify-between gap-3 px-3.5 py-2.5">
                  <span className="text-xs text-[#5B6075]">暂停通知</span>
                  <div className="flex items-center gap-1.5">
                    {([1, 4, 24] as const).map((hrs) => (
                      <button
                        key={hrs}
                        type="button"
                        disabled={disabled}
                        onClick={() => {
                          setOpen(false);
                          onSnooze(hrs);
                        }}
                        className={`rounded-full bg-[#F1F3FB] px-2.5 py-1 text-xs font-semibold text-[#14161F] transition-colors hover:bg-[#2B3AE8] hover:text-white ${focusRing}`}
                      >
                        {hrs}h
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </div>,
          document.body,
        )}
    </div>
  );
}

export const ServerAlertBellButton = ResourceAlertBellButton;
