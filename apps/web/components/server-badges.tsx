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
