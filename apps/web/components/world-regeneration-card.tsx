"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check } from "lucide-react";
import { useEffect, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { getWorldRegeneration, regenerateWorld } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { ServerStatus, WorldRegenerationState } from "@/lib/types";

function shouldResume(status: ServerStatus) {
  return status === "running" || status === "starting" || status === "restarting" || status === "creating";
}

export function WorldRegenerationAction({
  open,
  playersOnline,
  serverId,
  serverStatus,
  onActiveChange,
  onOpenChange
}: {
  open: boolean;
  playersOnline: number;
  serverId: string;
  serverStatus: ServerStatus;
  onActiveChange?: (active: boolean) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useI18n();
  const client = useQueryClient();
  const queryKey = ["world-regeneration", serverId] as const;
  const [startAfter, setStartAfter] = useState(shouldResume(serverStatus));
  const query = useQuery({
    queryKey,
    queryFn: () => getWorldRegeneration(serverId),
    retry: false,
    refetchInterval: (current) => isActive(current.state.data) ? 2000 : 30000
  });
  const mutation = useMutation({
    mutationFn: () => regenerateWorld(serverId, startAfter),
    onSuccess: async (job) => {
      onOpenChange(false);
      client.setQueryData<WorldRegenerationState>(queryKey, { supported: true, job });
      await client.invalidateQueries({ queryKey });
    }
  });
  const active = isActive(query.data);
  const blockedByPlayers = playersOnline > 0;

  useEffect(() => {
    onActiveChange?.(active);
    return () => onActiveChange?.(false);
  }, [active, onActiveChange]);

  useEffect(() => {
    if (!open) return;
    setStartAfter(shouldResume(serverStatus));
  }, [open, serverStatus]);

  return (
    <ConfirmDialog
      open={open}
      eyebrow={t("worldRegenerationDialogEyebrow")}
      title={t("worldRegenerationDialogTitle")}
      description={t("worldRegenerationDialogDescription")}
      detail={<div className="space-y-3">
        <div className="flex items-start gap-2.5">
          <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded border border-panel-gold/35 bg-panel-gold/10 text-panel-gold">
            <Check aria-hidden="true" className="size-3.5" />
          </span>
          <div>
            <p className="font-medium text-slate-200">{t("worldRegenerationBackupRequired")}</p>
            <p className="mt-0.5 text-xs leading-5 text-slate-400">{t("worldRegenerationPreserves")}</p>
          </div>
        </div>
        <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-panel-line bg-slate-900/45 p-3">
          <input className="mt-0.5 size-4 accent-panel-green" type="checkbox" checked={startAfter} onChange={(event) => setStartAfter(event.target.checked)} />
          <span>
            <span className="block font-medium text-slate-200">{t("worldRegenerationStartAfter")}</span>
            <span className="mt-0.5 block text-xs leading-5 text-slate-400">
              {shouldResume(serverStatus) ? t("worldRegenerationStartRunningHint") : t("worldRegenerationStartStoppedHint")}
            </span>
          </span>
        </label>
        {blockedByPlayers ? (
          <div className="flex items-start gap-2 rounded-md border border-panel-gold/30 bg-panel-gold/10 p-3 text-xs leading-5 text-panel-gold" role="alert">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
            <span>{t("worldRegenerationPlayersBlocked", { count: playersOnline })}</span>
          </div>
        ) : null}
        {mutation.isError || query.isError ? (
          <div className="flex items-start gap-2 rounded-md border border-red-400/25 bg-red-400/10 p-3 text-xs leading-5 text-red-200" role="alert">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
            <span>{t("worldRegenerationStartFailed")}</span>
          </div>
        ) : null}
      </div>}
      cancelLabel={t("cancel")}
      confirmLabel={mutation.isPending ? t("actionWorking") : t("worldRegenerationConfirm")}
      confirmVariant="danger"
      busy={mutation.isPending}
      confirmDisabled={blockedByPlayers || active || query.isError}
      onCancel={() => onOpenChange(false)}
      onConfirm={() => mutation.mutate()}
    />
  );
}

function isActive(state?: WorldRegenerationState) {
  return state?.job?.status === "queued" || state?.job?.status === "running";
}
