"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, Globe, Play, Trash2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { useToast } from "@/components/toast-context";
import { useI18n } from "@/lib/i18n";
import { deleteWorld, downloadWorldFile, updateGameServerConfig } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { gameServerJoinPort } from "@/lib/game-server-resource";
import { cn } from "@/lib/utils";
import type { GameServerResource, World } from "@/lib/types";

interface WorldRadarGridProps {
  worlds: World[];
  servers?: GameServerResource[];
  currentServer?: GameServerResource;
}

export function WorldRadarGrid({
  worlds,
  servers = [],
  currentServer
}: WorldRadarGridProps) {
  const { locale, t } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();
  const client = useQueryClient();

  const [downloadingId, setDownloadingId] = useState<string>("");
  const [pendingDeleteWorld, setPendingDeleteWorld] = useState<World | null>(null);
  const [pendingActivateWorld, setPendingActivateWorld] = useState<{ world: World; server: GameServerResource } | null>(null);

  const serverNameMap = new Map(servers.map((s) => [s.id, s.name]));

  // Download World Handler
  const handleDownload = async (world: World) => {
    try {
      setDownloadingId(world.id);
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
      toast.success(
        isZh ? "世界存档已打包下载！" : "World Downloaded!",
        isZh ? "已成功保存到本地电脑，放入单机存档目录即可继续游玩" : "Saved to your local computer."
      );
    } catch (err) {
      toast.error(isZh ? "下载失败" : "Download Failed", err instanceof Error ? err.message : "");
    } finally {
      setDownloadingId("");
    }
  };

  // Delete Mutation
  const deleteMutation = useMutation({
    mutationFn: async (worldId: string) => {
      await deleteWorld(worldId);
    },
    onSuccess: async () => {
      toast.success(isZh ? "世界已删除" : "World Deleted");
      setPendingDeleteWorld(null);
      await client.invalidateQueries({ queryKey: ["worlds"] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (err) => {
      toast.error(isZh ? "删除失败" : "Delete Failed", err instanceof Error ? err.message : "");
    }
  });

  // Activate World to Server Mutation
  const activateMutation = useMutation({
    mutationFn: async ({ world, server }: { world: World; server: GameServerResource }) => {
      const draft = { ...((server.spec?.config ?? {}) as Record<string, unknown>), worldName: world.name, saveName: world.name };
      await updateGameServerConfig(server.id, draft, gameServerJoinPort(server));
    },
    onSuccess: async () => {
      toast.success(
        isZh ? "世界已切换激活！" : "World Activated!",
        isZh ? "请重启房间以加载新世界存档" : "Restart room to load new world save."
      );
      setPendingActivateWorld(null);
      await client.invalidateQueries({ queryKey: ["game-server"] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (err) => {
      toast.error(isZh ? "切换失败" : "Activation Failed", err instanceof Error ? err.message : "");
    }
  });

  if (worlds.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-slate-800 bg-slate-950/40 p-8 text-center space-y-3">
        <div className="mx-auto size-12 rounded-full bg-slate-900 flex items-center justify-center text-slate-500">
          <Globe className="size-6" />
        </div>
        <div>
          <h4 className="text-sm font-semibold text-white">{isZh ? "暂无世界地图存档" : "No World Saves Found"}</h4>
          <p className="text-xs text-slate-500 mt-1">
            {isZh ? "你可以使用上方的搬家工作台拖入单机存档，或在房间内自动生成新世界。" : "Import a local save above or generate a new world in game."}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {worlds.map((world) => {
          const isDownloading = downloadingId === world.id;
          const assignedServerId = world.instanceId;
          const assignedServerName = assignedServerId && assignedServerId !== "unassigned" ? serverNameMap.get(assignedServerId) : null;
          const isCurrentActive = currentServer && assignedServerId === currentServer.id;

          return (
            <div
              key={world.id}
              className={cn(
                "group relative flex flex-col justify-between rounded-xl border p-4 sm:p-5 transition shadow-lg",
                isCurrentActive
                  ? "border-panel-green/80 bg-panel-green/5 ring-1 ring-panel-green/40"
                  : "border-slate-800 bg-slate-900/70 hover:border-slate-700 hover:bg-slate-900"
              )}
            >
              <div>
                {/* Header Badge */}
                <div className="flex items-center justify-between gap-2">
                  <span className="inline-flex items-center gap-1.5 rounded-md bg-slate-800 px-2 py-0.5 text-[10px] font-mono text-slate-300">
                    <Globe className="size-3 text-panel-green" />
                    {world.providerKey ? world.providerKey.toUpperCase() : (world.gameKey?.toUpperCase() || "WORLD")}
                  </span>
                  <span className="text-[11px] font-mono text-slate-400">{world.bytes || world.size || "0 KB"}</span>
                </div>

                {/* Title */}
                <div className="mt-3">
                  <h4 className="text-sm font-bold text-white group-hover:text-panel-green transition truncate">
                    {world.name}
                  </h4>
                  <p className="mt-0.5 text-[11px] font-mono text-slate-500 truncate">
                    {world.name}.wld
                  </p>
                </div>

                {/* Assigned Status Tag */}
                <div className="mt-3">
                  {assignedServerName ? (
                    <span className="inline-flex items-center gap-1 text-[11px] text-sky-400 bg-sky-950/40 border border-sky-800/40 px-2 py-0.5 rounded">
                      <Sparkles className="size-3" />
                      {isZh ? `已绑定: ${assignedServerName}` : `Assigned: ${assignedServerName}`}
                    </span>
                  ) : (
                    <span className="text-[11px] text-slate-500">{isZh ? "公共世界库 (未绑定房间)" : "Unassigned World"}</span>
                  )}
                </div>
              </div>

              {/* Bottom Actions */}
              <div className="mt-5 pt-3 border-t border-slate-800/80 flex items-center justify-between gap-2">
                {/* Download */}
                <Button
                  type="button"
                  variant="secondary"
                  disabled={isDownloading}
                  onClick={() => handleDownload(world)}
                  className="gap-1.5 text-xs h-8 px-2.5 bg-slate-950/80 hover:bg-slate-800 text-slate-200 border border-slate-800"
                >
                  <Download className="size-3.5 text-panel-gold" />
                  <span>{isDownloading ? (isZh ? "打包中..." : "Downloading...") : (isZh ? "下载到电脑" : "Download")}</span>
                </Button>

                <div className="flex items-center gap-1.5">
                  {/* Switch to this world (if in server detail context) */}
                  {currentServer && !isCurrentActive && (
                    <Button
                      type="button"
                      variant="primary"
                      onClick={() => setPendingActivateWorld({ world, server: currentServer })}
                      className="gap-1 text-xs h-8 px-2.5 bg-panel-green hover:bg-emerald-600 text-black font-bold"
                    >
                      <Play className="size-3" />
                      <span>{isZh ? "切换此世界" : "Activate"}</span>
                    </Button>
                  )}

                  {/* Delete */}
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => setPendingDeleteWorld(world)}
                    className="size-8 p-0 text-slate-500 hover:text-rose-400 hover:bg-rose-950/20"
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Confirm Delete Dialog */}
      {pendingDeleteWorld && (
        <ConfirmDialog
          open={Boolean(pendingDeleteWorld)}
          eyebrow={t("destructiveAction")}
          title={isZh ? "确认删除该世界存档？" : "Delete World Save?"}
          description={
            isZh
              ? `确定要彻底删除世界【${pendingDeleteWorld.name}】吗？此操作无法撤销。`
              : `Are you sure you want to permanently delete world "${pendingDeleteWorld.name}"?`
          }
          cancelLabel={t("cancel")}
          confirmLabel={isZh ? "彻底删除" : "Delete"}
          confirmVariant="danger"
          busy={deleteMutation.isPending}
          onConfirm={() => {
            if (pendingDeleteWorld) deleteMutation.mutate(pendingDeleteWorld.id);
          }}
          onCancel={() => setPendingDeleteWorld(null)}
        />
      )}

      {/* Confirm Activate Dialog */}
      {pendingActivateWorld && (
        <ConfirmDialog
          open={Boolean(pendingActivateWorld)}
          eyebrow={t("confirmActionEyebrow")}
          title={isZh ? "确认切换为该世界存档？" : "Switch World Save?"}
          description={
            isZh
              ? `确定要将房间【${pendingActivateWorld.server.name}】的运行世界切换为【${pendingActivateWorld.world.name}】吗？保存后需要重启房间生效。`
              : `Switch world to "${pendingActivateWorld.world.name}" for server "${pendingActivateWorld.server.name}"?`
          }
          cancelLabel={t("cancel")}
          confirmLabel={isZh ? "确认切换世界" : "Confirm Switch"}
          confirmVariant="primary"
          busy={activateMutation.isPending}
          onConfirm={() => {
            if (pendingActivateWorld) activateMutation.mutate(pendingActivateWorld);
          }}
          onCancel={() => setPendingActivateWorld(null)}
        />
      )}
    </div>
  );
}
