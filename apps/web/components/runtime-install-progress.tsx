"use client";

import { Loader2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { formatTransferBytes, runtimeInstallProgress } from "@/lib/runtime-install-progress";
import type { RuntimeImageStatus } from "@/lib/types";

export function RuntimeInstallProgress({ status }: { status?: RuntimeImageStatus }) {
  const { t } = useI18n();
  const p = runtimeInstallProgress(status);
  let amount = "";
  if (p.total > 0 && p.percent !== undefined) {
    amount = `${formatTransferBytes(p.current)} / ${formatTransferBytes(p.total)}`;
  } else if (p.phase === "extracting" && p.completedLayers > 0) {
    amount = t("runtimeInstallLayers", { count: p.completedLayers });
  } else if (p.current > 0) {
    const key = p.phase === "saving" ? "runtimeInstallWritten" : p.phase === "extracting" ? "runtimeInstallExtracted" : "runtimeInstallDownloaded";
    amount = t(key, { size: formatTransferBytes(p.current) });
  }
  return (
    <div className="min-w-0 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs">
        <span className="inline-flex items-center gap-1.5 text-sky-300">
          <Loader2 aria-hidden="true" className="size-3.5 animate-spin motion-reduce:animate-none" />
          {t(p.label)}
        </span>
        <span className="flex flex-wrap items-center gap-x-3 text-slate-400 tabular-nums">
          {amount ? <span>{amount}</span> : null}
          {p.speed > 0 ? <span>↓ {formatTransferBytes(p.speed)}/s</span> : null}
          {p.percent !== undefined ? <span className="text-sky-300">{p.percent}%</span> : null}
        </span>
      </div>
      <div role="progressbar" aria-label={t(p.label)} aria-valuemin={0} aria-valuemax={100} aria-valuenow={p.percent} className="relative h-1 overflow-hidden rounded-full bg-slate-800">
        {p.percent !== undefined ? (
          <div className="h-full rounded-full bg-sky-300 transition-[width] duration-300 motion-reduce:transition-none" style={{ width: `${p.percent}%` }} />
        ) : (
          <div className="runtime-install-indeterminate absolute inset-y-0 left-0 w-20 rounded-full bg-sky-300/90 motion-reduce:animate-none" />
        )}
      </div>
    </div>
  );
}
