"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button, Card } from "@/components/ui";
import { applyGameUpdate, checkGameUpdate, getGameUpdate } from "@/lib/api";
import { isGameUpdateStateActive, normalizeGameUpdateProgress } from "@/lib/game-update";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { GameUpdateState, ServerStatus } from "@/lib/types";

const updateStatusLabelKeys: Record<string, MessageKey> = {
  unknown: "gameUpdateStatusUnknown",
  checking: "gameUpdateStatusChecking",
  up_to_date: "gameUpdateStatusUpToDate",
  available: "gameUpdateStatusAvailable",
  updating: "gameUpdateStatusUpdating",
  failed: "gameUpdateStatusFailed"
};

const updateStageLabelKeys: Record<string, MessageKey> = {
  queued: "gameUpdateStageQueued",
  preflight: "gameUpdateStagePreflight",
  backing_up: "gameUpdateStageBackingUp",
  backup: "gameUpdateStageBackingUp",
  stopping: "gameUpdateStageStopping",
  refreshing_metadata: "gameUpdateStageRefreshingMetadata",
  validating: "gameUpdateStageValidating",
  downloading: "gameUpdateStageDownloading",
  installing: "gameUpdateStageInstalling",
  starting: "gameUpdateStageStarting",
  health_check: "gameUpdateStageHealthCheck",
  succeeded: "gameUpdateStageSucceeded",
  completed: "gameUpdateStageSucceeded",
  failed: "gameUpdateStageFailed"
};

function shouldResumeServer(status: ServerStatus) {
  return status === "running" || status === "starting" || status === "restarting" || status === "creating";
}

export function GameUpdateCard({
  playersOnline,
  serverId,
  serverStatus,
  onActiveChange
}: {
  playersOnline: number;
  serverId: string;
  serverStatus: ServerStatus;
  onActiveChange?: (active: boolean) => void;
}) {
  const { locale, t } = useI18n();
  const queryClient = useQueryClient();
  const queryKey = ["game-update", serverId] as const;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [startAfterUpdate, setStartAfterUpdate] = useState(shouldResumeServer(serverStatus));

  const updateQuery = useQuery({
    queryKey,
    queryFn: () => getGameUpdate(serverId),
    retry: false,
    refetchInterval: (query) => isGameUpdateStateActive(query.state.data) ? 2000 : 30000
  });

  const checkMutation = useMutation({
    mutationFn: () => checkGameUpdate(serverId),
    onSuccess: async (job) => {
      queryClient.setQueryData<GameUpdateState>(queryKey, (current) => ({
        supported: current?.supported ?? true,
        status: "checking",
        installedBuildId: current?.installedBuildId,
        latestBuildId: current?.latestBuildId,
        checkedAt: current?.checkedAt,
        job
      }));
      await queryClient.invalidateQueries({ queryKey });
    }
  });

  const applyMutation = useMutation({
    mutationFn: () => applyGameUpdate(serverId, startAfterUpdate),
    onSuccess: async (job) => {
      setConfirmOpen(false);
      queryClient.setQueryData<GameUpdateState>(queryKey, (current) => ({
        supported: current?.supported ?? true,
        status: "updating",
        installedBuildId: current?.installedBuildId,
        latestBuildId: current?.latestBuildId,
        checkedAt: current?.checkedAt,
        job
      }));
      await queryClient.invalidateQueries({ queryKey });
    }
  });

  const state = updateQuery.data;
  const active = isGameUpdateStateActive(state);
  const progress = normalizeGameUpdateProgress(state?.job?.progress);
  const status = state?.status ?? "unknown";
  const stageKey = updateStageLabelKeys[state?.job?.stage ?? ""];
  const statusLabel = t(updateStatusLabelKeys[status] ?? "gameUpdateStatusUnknown");
  const stageLabel = stageKey ? t(stageKey) : status === "checking" ? t("gameUpdateStatusChecking") : t("gameUpdateStatusUpdating");
  const blockedByPlayers = playersOnline > 0;
  const loadError = updateQuery.isError ? t("gameUpdateUnavailable") : "";
  const actionError = checkMutation.isError
    ? t("gameUpdateCheckFailed")
    : applyMutation.isError
      ? t("gameUpdateApplyFailed")
      : state?.job?.status === "failed"
        ? state.job.operation === "check" ? t("gameUpdateCheckFailed") : gameUpdateFailureMessage(state.job.error, locale, t)
        : "";

  const openUpdate = () => {
    applyMutation.reset();
    setStartAfterUpdate(shouldResumeServer(serverStatus));
    setConfirmOpen(true);
  };

  useEffect(() => {
    onActiveChange?.(active);
    return () => onActiveChange?.(false);
  }, [active, onActiveChange]);

  return (
    <>
      <Card className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/45 text-panel-green">
              <RefreshCw aria-hidden="true" className={cn("size-4", active && "animate-spin motion-reduce:animate-none")} />
            </span>
            <div className="min-w-0">
              <h2 className="font-semibold text-white">{t("gameVersionTitle")}</h2>
              <p className="mt-1 text-xs leading-5 text-slate-400">{t("gameVersionDescription")}</p>
            </div>
          </div>
          {!updateQuery.isLoading && !updateQuery.isError && state?.supported !== false ? (
            <StatusPill status={status} label={statusLabel} />
          ) : null}
        </div>

        {updateQuery.isLoading ? (
          <div className="mt-4 space-y-2" aria-label={t("loading")}>
            <div className="h-10 animate-pulse rounded-md bg-slate-800/70 motion-reduce:animate-none" />
            <div className="h-10 animate-pulse rounded-md bg-slate-800/50 motion-reduce:animate-none" />
          </div>
        ) : updateQuery.isError ? (
          <div className="mt-4 rounded-md border border-panel-gold/25 bg-panel-gold/10 p-3">
            <p className="text-xs leading-5 text-panel-gold">{loadError}</p>
            <Button className="mt-2 h-8 px-2 text-xs" variant="secondary" onClick={() => void updateQuery.refetch()}>
              <RefreshCw aria-hidden="true" className="size-3.5" />
              {t("gameUpdateRetryLoad")}
            </Button>
          </div>
        ) : state?.supported === false ? (
          <p className="mt-4 rounded-md border border-panel-line bg-slate-950/35 p-3 text-xs leading-5 text-slate-400">{t("gameUpdateUnsupported")}</p>
        ) : (
          <>
            <dl className="mt-4 divide-y divide-panel-line rounded-md border border-panel-line bg-slate-950/35 px-3">
              <VersionRow label={t("gameUpdateCurrentBuild")} value={state?.installedBuildId || "—"} />
              <VersionRow label={t("gameUpdateLatestBuild")} value={state?.latestBuildId || "—"} />
              <VersionRow label={t("gameUpdateLastChecked")} value={formatCheckedAt(state?.checkedAt, locale, t("gameUpdateNeverChecked"))} />
            </dl>

            {active ? (
              <div className="mt-4 rounded-md border border-panel-green/25 bg-panel-green/10 p-3">
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="font-medium text-slate-200" aria-live="polite">{stageLabel}</span>
                  <span className="tabular-nums text-slate-400">{progress}%</span>
                </div>
                <div
                  aria-label={t("gameUpdateProgress")}
                  aria-valuemax={100}
                  aria-valuemin={0}
                  aria-valuenow={progress}
                  className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-800"
                  role="progressbar"
                >
                  <div className="h-full rounded-full bg-panel-green transition-[width] duration-200 motion-reduce:transition-none" style={{ width: `${progress}%` }} />
                </div>
              </div>
            ) : null}

            {actionError ? (
              <div className="mt-3 flex items-start gap-2 rounded-md border border-red-400/25 bg-red-400/10 p-3 text-xs leading-5 text-red-200" role="alert">
                <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
                <span>{actionError}</span>
              </div>
            ) : null}

            <div className="mt-4">
              {status === "available" && !active ? (
                <Button className="w-full" variant="gold" onClick={openUpdate}>
                  {t("gameUpdateView")}
                </Button>
              ) : (
                <Button
                  className="w-full"
                  variant="secondary"
                  disabled={active || checkMutation.isPending}
                  onClick={() => {
                    checkMutation.reset();
                    checkMutation.mutate();
                  }}
                >
                  <RefreshCw aria-hidden="true" className={cn("size-4", (active || checkMutation.isPending) && "animate-spin motion-reduce:animate-none")} />
                  {active || checkMutation.isPending
                    ? status === "updating" ? t("gameUpdateStatusUpdating") : t("gameUpdateStatusChecking")
                    : status === "up_to_date" || status === "failed" ? t("gameUpdateCheckAgain") : t("gameUpdateCheck")}
                </Button>
              )}
            </div>
          </>
        )}
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        eyebrow={t("gameUpdateDialogEyebrow")}
        title={t("gameUpdateDialogTitle")}
        description={t("gameUpdateDialogDescription")}
        detail={(
          <div className="space-y-3">
            <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2 rounded-md bg-slate-900/70 px-3 py-2.5">
              <BuildTarget label={t("gameUpdateCurrentBuild")} value={state?.installedBuildId || "—"} />
              <span aria-hidden="true" className="text-slate-600">→</span>
              <BuildTarget align="right" label={t("gameUpdateLatestBuild")} value={state?.latestBuildId || "—"} />
            </div>
            <div className="flex items-start gap-2.5">
              <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded border border-panel-green/35 bg-panel-green/10 text-panel-green">
                <Check aria-hidden="true" className="size-3.5" />
              </span>
              <div>
                <p className="font-medium text-slate-200">{t("gameUpdateBackupRequired")}</p>
                <p className="mt-0.5 text-xs leading-5 text-slate-400">{t("gameUpdateBackupRequiredHint")}</p>
              </div>
            </div>
            <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-panel-line bg-slate-900/45 p-3">
              <input
                className="mt-0.5 size-4 accent-panel-green"
                type="checkbox"
                checked={startAfterUpdate}
                onChange={(event) => setStartAfterUpdate(event.target.checked)}
              />
              <span>
                <span className="block font-medium text-slate-200">{t("gameUpdateStartAfter")}</span>
                <span className="mt-0.5 block text-xs leading-5 text-slate-400">
                  {shouldResumeServer(serverStatus) ? t("gameUpdateStartAfterRunningHint") : t("gameUpdateStartAfterStoppedHint")}
                </span>
              </span>
            </label>
            {blockedByPlayers ? (
              <div className="flex items-start gap-2 rounded-md border border-panel-gold/30 bg-panel-gold/10 p-3 text-xs leading-5 text-panel-gold" role="alert">
                <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
                <span>{t("gameUpdatePlayersBlocked", { count: playersOnline })}</span>
              </div>
            ) : null}
            {applyMutation.isError ? (
              <div className="flex items-start gap-2 rounded-md border border-red-400/25 bg-red-400/10 p-3 text-xs leading-5 text-red-200" role="alert">
                <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
                <span>{t("gameUpdateApplyFailed")}</span>
              </div>
            ) : null}
          </div>
        )}
        cancelLabel={t("cancel")}
        confirmLabel={applyMutation.isPending ? t("actionWorking") : t("gameUpdateApply")}
        confirmVariant="gold"
        busy={applyMutation.isPending}
        confirmDisabled={blockedByPlayers}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => applyMutation.mutate()}
      />
    </>
  );
}

function VersionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 py-2.5">
      <dt className="text-xs text-slate-400">{label}</dt>
      <dd className="truncate text-right font-mono text-xs font-medium text-slate-200">{value}</dd>
    </div>
  );
}

function BuildTarget({ align = "left", label, value }: { align?: "left" | "right"; label: string; value: string }) {
  return (
    <div className={cn("min-w-0", align === "right" && "text-right")}>
      <p className="text-[11px] text-slate-400">{label}</p>
      <p className="mt-0.5 truncate font-mono text-xs font-semibold text-slate-100">{value}</p>
    </div>
  );
}

function StatusPill({ label, status }: { label: string; status: string }) {
  const healthy = status === "up_to_date";
  const warning = status === "available";
  const failed = status === "failed";
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] font-medium",
        healthy && "border-panel-green/30 bg-panel-green/10 text-panel-green",
        warning && "border-panel-gold/30 bg-panel-gold/10 text-panel-gold",
        failed && "border-red-400/30 bg-red-400/10 text-red-200",
        !healthy && !warning && !failed && "border-panel-line bg-slate-950/45 text-slate-400"
      )}
    >
      <span className={cn("size-1.5 rounded-full bg-current", (status === "checking" || status === "updating") && "animate-pulse motion-reduce:animate-none")} />
      {label}
    </span>
  );
}

function formatCheckedAt(value: string | undefined, locale: "zh" | "en", fallback: string) {
  if (!value) return fallback;
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return fallback;
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(timestamp);
}

function gameUpdateFailureMessage(error: string | undefined, locale: "zh" | "en", t: (key: MessageKey) => string) {
  if (!error) return t("gameUpdateFailureDetail");
  if (locale === "en") return error;
  const normalized = error.toLowerCase();
  if (normalized.includes("disk space")) return t("gameUpdateFailureDisk");
  if (normalized.includes("memory")) return t("gameUpdateFailureMemory");
  if (normalized.includes("health")) return t("gameUpdateFailureHealth");
  if (normalized.includes("interrupted")) return t("gameUpdateFailureInterrupted");
  if (normalized.includes("steam") || normalized.includes("manifest")) return t("gameUpdateFailureValidation");
  return t("gameUpdateFailureDetail");
}
