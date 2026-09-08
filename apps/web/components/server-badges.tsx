"use client";

import { Box } from "lucide-react";
import { Badge } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { serverProviderDisplay } from "@/lib/server-display";
import { cn } from "@/lib/utils";
import type { MessageKey } from "@/lib/i18n";
import type { ProviderKey, ServerMode, ServerStatus } from "@/lib/types";

export function ServerStatusBadge({ status }: { status: ServerStatus }) {
  const { t } = useI18n();
  const color =
    status === "running"
      ? "bg-panel-green/15 text-panel-green"
      : status === "errored"
        ? "bg-red-500/15 text-red-200"
        : status === "starting" || status === "stopping" || status === "restarting" || status === "creating"
          ? "bg-panel-gold/15 text-panel-gold"
          : status === "deleting"
            ? "bg-red-500/15 text-red-200"
            : "bg-slate-700 text-slate-300";
  const label =
    status === "running"
      ? t("statusRunning")
      : status === "errored"
        ? t("statusErrored")
        : status === "starting"
          ? t("statusStarting")
          : status === "stopping"
            ? t("statusStopping")
            : status === "restarting"
              ? t("statusRestarting")
              : status === "creating"
                ? t("statusCreating")
                : status === "deleting"
                  ? t("statusDeleting")
                  : t("statusStopped");
  return <Badge className={color}>{label}</Badge>;
}

export function ServerModeBadge({ mode }: { mode: ServerMode }) {
  const { t } = useI18n();
  return mode === "tmodloader" ? (
    <Badge className="bg-panel-purple/20 text-panel-purple">tModLoader</Badge>
  ) : (
    <Badge className="bg-panel-green/15 text-panel-green">{t("modeVanilla")}</Badge>
  );
}

export function ServerProviderBadge({ server }: { server: { mode?: ServerMode; providerKey?: ProviderKey } }) {
  const display = serverProviderDisplay(server);
  const className =
    display.tone === "purple"
      ? "bg-panel-purple/20 text-panel-purple"
      : display.tone === "sky"
        ? "bg-sky-500/15 text-sky-300"
        : display.tone === "amber"
          ? "bg-panel-gold/15 text-panel-gold"
          : display.tone === "slate"
            ? "bg-slate-700 text-slate-300"
            : "bg-panel-green/15 text-panel-green";
  return <Badge className={className}>{display.label}</Badge>;
}

export function ServerProviderLabel({ server }: { server: { mode?: ServerMode; providerKey?: ProviderKey } }) {
  const { t } = useI18n();
  const display = serverProviderDisplay(server);
  const labelKey = providerLabelKey(server);
  const color =
    display.tone === "purple"
      ? "text-panel-purple"
      : display.tone === "sky"
        ? "text-sky-300"
        : display.tone === "amber"
          ? "text-panel-gold"
          : display.tone === "slate"
            ? "text-slate-400"
            : "text-panel-green";

  return (
    <span className="inline-flex min-w-0 items-center gap-2 whitespace-nowrap" title={labelKey ? t(labelKey) : display.label}>
      <Box aria-hidden="true" className={cn("size-3.5 shrink-0", color)} strokeWidth={1.8} />
      <span className="truncate font-medium text-slate-300">{labelKey ? t(labelKey) : display.label}</span>
    </span>
  );
}

export function ServerStatusIndicator({ status }: { status: ServerStatus }) {
  const { t } = useI18n();
  const isTransitioning = status === "starting" || status === "stopping" || status === "restarting" || status === "creating";
  const label =
    status === "running"
      ? t("statusRunning")
      : status === "errored"
        ? t("statusErrored")
        : status === "starting"
          ? t("statusStarting")
          : status === "stopping"
            ? t("statusStopping")
            : status === "restarting"
              ? t("statusRestarting")
              : status === "creating"
                ? t("statusCreating")
                : status === "deleting"
                  ? t("statusDeleting")
                  : t("statusStopped");
  const dotColor =
    status === "running"
      ? "bg-panel-green"
      : status === "errored" || status === "deleting"
        ? "bg-red-400"
        : isTransitioning
          ? "bg-panel-gold"
          : "bg-slate-500";
  const textColor =
    status === "running"
      ? "text-panel-green"
      : status === "errored" || status === "deleting"
        ? "text-red-300"
        : isTransitioning
          ? "text-panel-gold"
          : "text-slate-400";

  return (
    <span className={cn("inline-flex items-center gap-2 whitespace-nowrap font-medium", textColor)}>
      <span aria-hidden="true" className={cn("size-2 shrink-0 rounded-full", dotColor, isTransitioning && "motion-safe:animate-pulse")} />
      {label}
    </span>
  );
}

function providerLabelKey(server: { mode?: ServerMode; providerKey?: ProviderKey }): MessageKey | undefined {
  const providerKey = server.providerKey || (server.mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla");
  if (providerKey === "terraria-vanilla") return "providerNameTerrariaVanilla";
  if (providerKey === "terraria-tmodloader") return "providerNameTerrariaTmodloader";
  if (providerKey === "palworld") return "providerNamePalworld";
  if (providerKey === "dont-starve-together") return "providerNameDST";
  if (providerKey === "minecraft") return "providerNameMinecraft";
  return undefined;
}

export function ServerConvergenceBadge({
  desiredGeneration = 1,
  appliedGeneration = 0,
  phase
}: {
  desiredGeneration?: number;
  appliedGeneration?: number;
  phase?: string;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  if (phase === "failed" || phase === "deleting" || phase === "deleted") {
    return null;
  }

  const isConverging = desiredGeneration > appliedGeneration;

  if (isConverging) {
    return (
      <span
        title={
          isZh
            ? `控制面最新配置 (Gen ${desiredGeneration}) 正在同步至节点守护进程 (已应用 Gen ${appliedGeneration})`
            : `Desired Gen ${desiredGeneration} is reconciling to node (Applied Gen ${appliedGeneration})`
        }
        className="inline-flex items-center gap-1.5 rounded-full border border-sky-500/30 bg-sky-500/10 px-2.5 py-0.5 text-xs font-semibold text-sky-400 motion-safe:animate-pulse"
      >
        <span className="size-1.5 rounded-full bg-sky-400" />
        <span>{isZh ? `⚡ 配置收敛中 (v${appliedGeneration} → v${desiredGeneration})` : `⚡ Reconciling (v${appliedGeneration} → v${desiredGeneration})`}</span>
      </span>
    );
  }

  return (
    <span
      title={
        isZh
          ? `节点运行时与控制面规格已达成最终一致 (Gen ${appliedGeneration})`
          : `Node runtime has converged to desired generation (Gen ${appliedGeneration})`
      }
      className="inline-flex items-center gap-1 rounded-full border border-slate-800 bg-slate-900/60 px-2 py-0.5 text-[11px] font-mono text-slate-400"
    >
      <span className="size-1.5 rounded-full bg-emerald-400/80" />
      <span>v{appliedGeneration || desiredGeneration}</span>
    </span>
  );
}

export function ServerRegionBadge({ region }: { region?: string }) {
  if (!region) return null;
  return (
    <span
      title={`Deployment Region: ${region}`}
      className="inline-flex items-center gap-1 rounded-full border border-slate-800 bg-slate-900/70 px-2.5 py-0.5 text-xs font-mono text-slate-300"
    >
      <span>🌐</span>
      <span>{region}</span>
    </span>
  );
}

export function ServerSubscriptionBadge({
  status,
  expiresAtMs
}: {
  status?: string;
  expiresAtMs?: number;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  if (!status) return null;

  const isExpired = expiresAtMs ? expiresAtMs <= Date.now() : false;
  const expirationDate = expiresAtMs ? new Date(expiresAtMs).toLocaleDateString() : null;

  if (status === "active" && !isExpired) {
    return (
      <span
        title={expirationDate ? (isZh ? `订阅有效期至: ${expirationDate}` : `Valid until: ${expirationDate}`) : undefined}
        className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-xs font-semibold text-emerald-400"
      >
        <span className="size-1.5 rounded-full bg-emerald-400" />
        <span>{isZh ? (expirationDate ? `包月生效中 (至 ${expirationDate})` : "包月生效中") : (expirationDate ? `Prepaid (until ${expirationDate})` : "Prepaid Active")}</span>
      </span>
    );
  }

  if (isExpired || status === "expired") {
    return (
      <span
        className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-0.5 text-xs font-semibold text-amber-400"
      >
        <span className="size-1.5 rounded-full bg-amber-400" />
        <span>{isZh ? "订阅已到期" : "Subscription Expired"}</span>
      </span>
    );
  }

  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full border border-slate-700 bg-slate-800/80 px-2.5 py-0.5 text-xs font-medium text-slate-300"
    >
      <span className="size-1.5 rounded-full bg-slate-400" />
      <span>{status}</span>
    </span>
  );
}
