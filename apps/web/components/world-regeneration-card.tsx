"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Globe2, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button, Card } from "@/components/ui";
import { getWorldRegeneration, regenerateWorld } from "@/lib/api";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { GameServerResource, ServerStatus, WorldRegenerationState } from "@/lib/types";

const stageLabels: Record<string, MessageKey> = {
  queued: "worldRegenerationStageQueued",
  stopping: "worldRegenerationStageStopping",
  backing_up: "worldRegenerationStageBackingUp",
  resetting: "worldRegenerationStageResetting",
  starting: "worldRegenerationStageStarting",
  health_check: "worldRegenerationStageHealthCheck",
  rolling_back: "worldRegenerationStageRollingBack",
  completed: "worldRegenerationStageCompleted"
};

function shouldResume(status: ServerStatus) {
  return status === "running" || status === "starting" || status === "restarting" || status === "creating";
}

export function WorldRegenerationCard({
  playersOnline,
  resource,
  serverStatus,
  onActiveChange
}: {
  playersOnline: number;
  resource: GameServerResource;
  serverStatus: ServerStatus;
  onActiveChange?: (active: boolean) => void;
}) {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const queryKey = ["world-regeneration", resource.id] as const;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [startAfter, setStartAfter] = useState(shouldResume(serverStatus));
  const query = useQuery({
    queryKey,
    queryFn: () => getWorldRegeneration(resource.id),
    retry: false,
    refetchInterval: (current) => isActive(current.state.data) ? 2000 : 30000
  });
  const mutation = useMutation({
    mutationFn: () => regenerateWorld(resource.id, startAfter),
    onSuccess: async (job) => {
      setConfirmOpen(false);
      client.setQueryData<WorldRegenerationState>(queryKey, { supported: true, job });
      await client.invalidateQueries({ queryKey });
    }
  });
  const state = query.data;
  const active = isActive(state);
  const progress = Math.max(0, Math.min(100, Math.round(state?.job?.progress ?? 0)));
  const stageLabel = t(stageLabels[state?.job?.stage ?? ""] ?? "worldRegenerationStageQueued");
  const clusterName = readString(resource.spec.config, "identity.clusterName") || resource.name;
  const worldPreset = readString(resource.spec.config, "world.preset") || "forest_default";
  const cavesEnabled = readBoolean(resource.spec.config, "caves.enabled");
  const blockedByPlayers = playersOnline > 0;

  useEffect(() => {
    onActiveChange?.(active);
    return () => onActiveChange?.(false);
  }, [active, onActiveChange]);

  return (
    <>
      <Card className="p-4 sm:p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/45 text-panel-green">
              <Globe2 aria-hidden="true" className="size-4" />
            </span>
            <div className="min-w-0">
              <h2 className="font-semibold text-white">{t("worldManagementTitle")}</h2>
              <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-400">{t("worldManagementDescription")}</p>
            </div>
          </div>
          {!query.isLoading && !query.isError ? (
            <span className={cn(
              "inline-flex w-fit items-center rounded px-2 py-1 text-xs font-medium",
              active || state?.job?.status === "succeeded" ? "bg-panel-green/10 text-panel-green" : state?.job?.status === "failed" ? "bg-red-400/10 text-red-200" : "bg-slate-800 text-slate-300"
            )}>
              {active ? stageLabel : state?.job?.status === "succeeded" ? t("worldRegenerationStatusSucceeded") : state?.job?.status === "failed" ? t("worldRegenerationStatusFailed") : t("worldRegenerationStatusReady")}
            </span>
          ) : null}
        </div>

        {query.isLoading ? (
          <div className="mt-5 h-24 animate-pulse rounded-md bg-slate-800/60 motion-reduce:animate-none" aria-label={t("loading")} />
        ) : query.isError ? (
          <div className="mt-5 rounded-md border border-panel-gold/25 bg-panel-gold/10 p-3 text-xs leading-5 text-panel-gold">
            <p>{t("worldRegenerationUnavailable")}</p>
            <Button className="mt-2 h-8 px-2 text-xs" variant="secondary" onClick={() => void query.refetch()}>
              <RefreshCw aria-hidden="true" className="size-3.5" />{t("worldRegenerationRetry")}
            </Button>
          </div>
        ) : (
          <>
            <dl className="mt-5 grid gap-x-6 gap-y-3 border-y border-panel-line py-4 sm:grid-cols-3">
              <WorldValue label={t("worldClusterName")} value={clusterName} />
              <WorldValue label={t("worldPresetLabel")} value={worldPreset} />
              <WorldValue label={t("worldCavesLabel")} value={cavesEnabled ? t("worldCavesEnabled") : t("worldCavesDisabled")} />
            </dl>
            <p className="mt-3 text-xs leading-5 text-slate-400">{t("worldGenerationConfigHint")}</p>
            {active ? (
              <div className="mt-4 rounded-md border border-panel-green/25 bg-panel-green/10 p-3">
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="font-medium text-slate-200" aria-live="polite">{stageLabel}</span>
                  <span className="tabular-nums text-slate-400">{progress}%</span>
                </div>
                <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-800" role="progressbar" aria-label={t("worldRegenerationProgress")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}>
                  <div className="h-full rounded-full bg-panel-green transition-[width] duration-200 motion-reduce:transition-none" style={{ width: `${progress}%` }} />
                </div>
              </div>
            ) : null}
            {state?.job?.status === "failed" ? (
              <div className="mt-4 flex items-start gap-2 rounded-md border border-red-400/25 bg-red-400/10 p-3 text-xs leading-5 text-red-200" role="alert">
                <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
                <span>{locale === "zh" ? t("worldRegenerationFailureDetail") : state.job.error || t("worldRegenerationFailureDetail")}</span>
              </div>
            ) : null}
            <div className="mt-4 flex justify-end">
              <Button variant="gold" disabled={active} onClick={() => {
                mutation.reset();
                setStartAfter(shouldResume(serverStatus));
                setConfirmOpen(true);
              }}>{t("worldRegenerateAction")}</Button>
            </div>
          </>
        )}
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        eyebrow={t("worldRegenerationDialogEyebrow")}
        title={t("worldRegenerationDialogTitle")}
        description={t("worldRegenerationDialogDescription")}
        detail={<div className="space-y-3">
          <div className="flex items-start gap-2.5">
            <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded border border-panel-gold/35 bg-panel-gold/10 text-panel-gold"><Check aria-hidden="true" className="size-3.5" /></span>
            <div><p className="font-medium text-slate-200">{t("worldRegenerationBackupRequired")}</p><p className="mt-0.5 text-xs leading-5 text-slate-400">{t("worldRegenerationBackupHint")}</p></div>
          </div>
          <p className="rounded-md bg-slate-900/70 px-3 py-2.5 text-xs leading-5 text-slate-300">{t("worldRegenerationPreserves")}</p>
          <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-panel-line bg-slate-900/45 p-3">
            <input className="mt-0.5 size-4 accent-panel-green" type="checkbox" checked={startAfter} onChange={(event) => setStartAfter(event.target.checked)} />
            <span><span className="block font-medium text-slate-200">{t("worldRegenerationStartAfter")}</span><span className="mt-0.5 block text-xs leading-5 text-slate-400">{shouldResume(serverStatus) ? t("worldRegenerationStartRunningHint") : t("worldRegenerationStartStoppedHint")}</span></span>
          </label>
          {blockedByPlayers ? <div className="flex items-start gap-2 rounded-md border border-panel-gold/30 bg-panel-gold/10 p-3 text-xs leading-5 text-panel-gold" role="alert"><AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" /><span>{t("worldRegenerationPlayersBlocked", { count: playersOnline })}</span></div> : null}
          {mutation.isError ? <div className="flex items-start gap-2 rounded-md border border-red-400/25 bg-red-400/10 p-3 text-xs leading-5 text-red-200" role="alert"><AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" /><span>{t("worldRegenerationStartFailed")}</span></div> : null}
        </div>}
        cancelLabel={t("cancel")}
        confirmLabel={mutation.isPending ? t("actionWorking") : t("worldRegenerationConfirm")}
        confirmVariant="danger"
        busy={mutation.isPending}
        confirmDisabled={blockedByPlayers}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => mutation.mutate()}
      />
    </>
  );
}

function WorldValue({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0"><dt className="text-xs text-slate-500">{label}</dt><dd className="mt-1 truncate text-sm font-medium text-slate-200" title={value}>{value}</dd></div>;
}

function isActive(state?: WorldRegenerationState) {
  return state?.job?.status === "queued" || state?.job?.status === "running";
}

function readString(config: Record<string, unknown> | undefined, path: string) {
  const value = readPath(config, path);
  return typeof value === "string" ? value.trim() : "";
}

function readBoolean(config: Record<string, unknown> | undefined, path: string) {
  return readPath(config, path) === true;
}

function readPath(config: Record<string, unknown> | undefined, path: string): unknown {
  let current: unknown = config;
  for (const key of path.split(".")) {
    if (!current || typeof current !== "object" || Array.isArray(current)) return undefined;
    current = (current as Record<string, unknown>)[key];
  }
  return current;
}
