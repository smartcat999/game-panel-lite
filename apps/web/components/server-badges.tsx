"use client";

import { Badge } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { serverProviderDisplay } from "@/lib/server-display";
import type { Server, ServerMode, ServerStatus } from "@/lib/types";

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

export function ServerProviderBadge({ server }: { server: Pick<Server, "mode" | "providerKey"> }) {
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
