"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, RefreshCw, ShieldCheck, UserMinus, UserPlus, Users, UserX } from "lucide-react";
import Image from "next/image";
import { useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  addServerWhitelistPlayer,
  banServerPlayer,
  getServerWhitelist,
  kickServerPlayer,
  listServerPlayers,
  removeServerWhitelistPlayer,
} from "@/lib/api";
import { getDSTCharacterDisplayName, getDSTCharacterImage } from "@/lib/dst-characters";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type PendingAction = { player: string; playerName?: string; kind: "kick" | "ban" };

export function PlayersPanel({ serverId }: { serverId: string }) {
  const { t, locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [whitelistPlayer, setWhitelistPlayer] = useState("");
  const [whitelistMessage, setWhitelistMessage] = useState("");
  const [isRefreshing, setIsRefreshing] = useState(false);

  const playersQuery = useQuery({
    queryKey: ["server-players", serverId],
    queryFn: () => listServerPlayers(serverId),
    retry: false,
    refetchInterval: 8000,
  });

  const whitelistQuery = useQuery({
    queryKey: ["server-whitelist", serverId],
    queryFn: () => getServerWhitelist(serverId),
    retry: false,
    refetchInterval: 10000,
  });

  const handleManualRefresh = async () => {
    setIsRefreshing(true);
    try {
      const res = await listServerPlayers(serverId, true);
      queryClient.setQueryData(["server-players", serverId], res);
    } catch {
      // ignore
    } finally {
      setIsRefreshing(false);
    }
  };

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["server-players", serverId] });
  const kickMutation = useMutation({
    mutationFn: (player: string) => kickServerPlayer(serverId, player),
    onSuccess: () => {
      invalidate();
      setPending(null);
    },
  });
  const banMutation = useMutation({
    mutationFn: (player: string) => banServerPlayer(serverId, player),
    onSuccess: () => {
      invalidate();
      setPending(null);
    },
  });
  const whitelistInvalidate = () => queryClient.invalidateQueries({ queryKey: ["server-whitelist", serverId] });
  const addWhitelistMutation = useMutation({
    mutationFn: (player: string) => addServerWhitelistPlayer(serverId, player),
    onSuccess: async () => {
      setWhitelistMessage(t("whitelistPlayerAdded", { name: whitelistPlayer.trim() }));
      setWhitelistPlayer("");
      await whitelistInvalidate();
    },
    onError: (error) => setWhitelistMessage(error instanceof Error ? error.message : t("whitelistActionFailed")),
  });
  const removeWhitelistMutation = useMutation({
    mutationFn: (player: string) => removeServerWhitelistPlayer(serverId, player),
    onSuccess: async () => {
      setWhitelistMessage(t("whitelistPlayerRemoved", { name: whitelistPlayer.trim() }));
      setWhitelistPlayer("");
      await whitelistInvalidate();
    },
    onError: (error) => setWhitelistMessage(error instanceof Error ? error.message : t("whitelistActionFailed")),
  });

  const confirmAction = () => {
    if (!pending) return;
    if (pending.kind === "kick") {
      kickMutation.mutate(pending.player);
    } else {
      banMutation.mutate(pending.player);
    }
  };
  const busy = kickMutation.isPending || banMutation.isPending;
  const whitelistBusy = addWhitelistMutation.isPending || removeWhitelistMutation.isPending;
  const whitelistSupported = Boolean(whitelistQuery.data?.supported);
  const whitelistRunning = Boolean(whitelistQuery.data?.running);
  const whitelistDisabled = whitelistBusy || !whitelistRunning || whitelistPlayer.trim() === "";
  const submitWhitelist = (kind: "add" | "remove") => {
    const player = whitelistPlayer.trim();
    if (!player) return;
    setWhitelistMessage("");
    if (kind === "add") {
      addWhitelistMutation.mutate(player);
    } else {
      removeWhitelistMutation.mutate(player);
    }
  };

  if (playersQuery.data && !playersQuery.data.supported) {
    return null;
  }

  const players = playersQuery.data?.players ?? [];

  return (
    <Card className="space-y-4 p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Users aria-hidden="true" className="size-4 text-panel-green" />
          <h3 className="text-sm font-semibold text-white">{t("playersPanelTitle")}</h3>
          {players.length > 0 && (
            <span className="rounded bg-emerald-500/15 border border-emerald-500/30 text-emerald-400 px-2 py-0.5 text-xs font-mono">
              {players.length} {isZh ? "人在线" : "online"}
            </span>
          )}
        </div>
        <button
          type="button"
          title={isZh ? "刷新" : "Refresh"}
          disabled={isRefreshing || playersQuery.isFetching}
          onClick={handleManualRefresh}
          className="size-7 flex items-center justify-center rounded border border-slate-800 bg-slate-950 text-slate-400 hover:text-white transition text-xs disabled:opacity-50"
        >
          <RefreshCw className={cn("size-3.5", (playersQuery.isFetching || isRefreshing) && "animate-spin")} />
        </button>
      </div>

      {players.length === 0 ? (
        <p className="text-sm text-slate-400">{playersQuery.isLoading ? t("loading") : t("playersNone")}</p>
      ) : (
        <ul className="space-y-2">
          {players.map((player, index) => {
            const name = player.name ?? `Player ${index + 1}`;
            const targetId = player.userId || name;
            const charImg = getDSTCharacterImage(player.character);
            const charName = getDSTCharacterDisplayName(player.character, isZh);

            return (
              <li
                key={`${targetId}-${index}`}
                className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2.5 hover:border-slate-800 transition"
              >
                <div className="flex items-center gap-3 min-w-0">
                  {charImg ? (
                    <Image
                      src={charImg}
                      alt={charName || player.name || "Player Avatar"}
                      width={36}
                      height={36}
                      className="size-9 rounded-full object-cover border border-slate-700 bg-slate-900 shrink-0"
                    />
                  ) : (
                    <div className="size-9 flex items-center justify-center rounded-full bg-slate-800 border border-slate-700 font-bold text-slate-300 shrink-0 text-sm">
                      {name.charAt(0).toUpperCase()}
                    </div>
                  )}
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm text-slate-100 truncate">{name}</span>
                      {player.isHost && (
                        <span className="rounded bg-amber-500/15 border border-amber-500/30 text-amber-300 px-1.5 py-0.5 text-[10px] font-medium">
                          {isZh ? "房主" : "Host"}
                        </span>
                      )}
                      {charName && (
                        <span className="rounded bg-purple-500/15 border border-purple-500/30 text-purple-300 px-1.5 py-0.5 text-[10px]">
                          {charName}
                        </span>
                      )}
                    </div>
                    {player.userId && (
                      <div className="text-[11px] font-mono text-slate-500 truncate">
                        {player.userId}
                      </div>
                    )}
                  </div>
                </div>

                <span className="flex shrink-0 items-center gap-2">
                  <Button
                    variant="ghost"
                    className="gap-1 px-2.5 py-1 text-xs text-panel-gold hover:text-panel-gold hover:bg-amber-500/10"
                    onClick={() => setPending({ player: targetId, playerName: name, kind: "kick" })}
                  >
                    <UserX aria-hidden="true" className="size-3.5" />
                    {t("playersKick")}
                  </Button>
                  <Button
                    variant="ghost"
                    className="gap-1 px-2.5 py-1 text-xs text-red-400 hover:text-red-300 hover:bg-red-500/10"
                    onClick={() => setPending({ player: targetId, playerName: name, kind: "ban" })}
                  >
                    <Ban aria-hidden="true" className="size-3.5" />
                    {t("playersBan")}
                  </Button>
                </span>
              </li>
            );
          })}
        </ul>
      )}

      {whitelistSupported ? (
        <div className="border-t border-panel-line pt-4">
          <div className="mb-3 flex items-center gap-2">
            <ShieldCheck aria-hidden="true" className="size-4 text-panel-green" />
            <h3 className="text-sm font-semibold text-white">{t("whitelistPanelTitle")}</h3>
          </div>
          <p className="mb-3 text-sm text-slate-400">
            {whitelistRunning ? t("whitelistPanelDescription") : t("whitelistRequiresRunning")}
          </p>
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
            <Input
              value={whitelistPlayer}
              placeholder={t("whitelistPlayerPlaceholder")}
              onChange={(event) => setWhitelistPlayer(event.target.value)}
              disabled={whitelistBusy}
            />
            <Button variant="secondary" disabled={whitelistDisabled} onClick={() => submitWhitelist("add")}>
              <UserPlus aria-hidden="true" className="size-4" />
              {addWhitelistMutation.isPending ? t("actionWorking") : t("whitelistAdd")}
            </Button>
            <Button variant="secondary" disabled={whitelistDisabled} onClick={() => submitWhitelist("remove")}>
              <UserMinus aria-hidden="true" className="size-4" />
              {removeWhitelistMutation.isPending ? t("actionWorking") : t("whitelistRemove")}
            </Button>
          </div>
          {whitelistMessage ? <p className="mt-2 text-sm text-slate-400">{whitelistMessage}</p> : null}
        </div>
      ) : null}

      <ConfirmDialog
        open={Boolean(pending)}
        eyebrow={t("confirmActionEyebrow")}
        title={pending?.kind === "ban" ? t("playersBan") : t("playersKick")}
        description={
          pending?.kind === "ban"
            ? t("playersBanConfirm", { name: pending?.playerName ?? pending?.player ?? "" })
            : t("playersKickConfirm", { name: pending?.playerName ?? pending?.player ?? "" })
        }
        cancelLabel={t("cancel")}
        confirmLabel={busy ? t("actionWorking") : pending?.kind === "ban" ? t("playersBan") : t("playersKick")}
        confirmVariant={pending?.kind === "ban" ? "danger" : "gold"}
        busy={busy}
        onCancel={() => setPending(null)}
        onConfirm={confirmAction}
      />
    </Card>
  );
}
