"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card } from "@/components/ui";
import { getGameServer, listGameServers, ModInstallationError, requestModInstallation } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { ModFile } from "@/lib/types";

export function WorkspaceModInstallation({ mods, workspaces }: { mods: ModFile[]; workspaces: { id: string; name?: string }[] }) {
  const zh = useI18n().locale.startsWith("zh");
  const client = useQueryClient();
  const [modId, setModId] = useState("");
  const [serverId, setServerId] = useState("");
  const [needsRefresh, setNeedsRefresh] = useState(false);
  const mod = mods.find(item => item.id === modId && item.source === "upload" && item.organizationId);
  const servers = useQuery({ queryKey: ["mod-installation-targets"], queryFn: listGameServers, retry: false });
  const targets = (servers.data ?? []).filter(item => mod && item.organizationId === mod.organizationId && item.providerKey === mod.providerKey);
  const target = useQuery({ queryKey: ["mod-installation-target", serverId], queryFn: () => getGameServer(serverId), enabled: !!serverId, retry: false });
  const current = target.data;
  const matches = !!mod && !!current && current.id === serverId && current.organizationId === mod.organizationId && current.providerKey === mod.providerKey && targets.some(item => item.id === serverId);
  const local = !!current && (!current.nodeId || current.nodeId === "node-local");
  const stopped = current?.spec.desiredState === "stopped" && current.status.phase === "stopped";
  const requested = matches && current.spec.modIds?.includes(modId);
  const install = useMutation({
    retry: false,
    mutationFn: (request: { serverId: string; modId: string; generation: number; serverName: string; modName: string }) => requestModInstallation(request.serverId, request.modId, request.generation),
    onSettled: async () => {
      setNeedsRefresh(true);
      await Promise.all([
        client.invalidateQueries({ queryKey: ["mod-installation-targets"] }),
        client.invalidateQueries({ queryKey: ["mod-installation-target"] }),
        client.invalidateQueries({ queryKey: ["game-servers"]  }),
        client.invalidateQueries({ queryKey: ["game-server"] }),
        client.invalidateQueries({ queryKey: ["game-servers-page"] })
      ]);
    }
  });
  const ready = matches && servers.isSuccess && target.isSuccess && !target.isFetching && local && stopped && !requested && !needsRefresh && !install.isPending;
  const selectClass = "mt-2 h-10 w-full rounded-md border border-panel-line bg-panel-card px-3 text-sm text-slate-100 disabled:opacity-50";
  const failure = install.error instanceof ModInstallationError ? install.error : undefined;
  return <Card className="p-5">
    <h2 className="font-semibold text-slate-100">{zh ? "安装工作区模组" : "Install a workspace mod"}</h2>
    <p className="mt-2 text-sm text-slate-400">{zh ? "选择同工作区的本地停服实例。请求保存后，在实例下次启动时安装文件；此操作不会启动实例。" : "Choose a stopped local instance in the same workspace. Files are installed on its next start. This request does not start the instance."}</p>
    <form className="mt-4 space-y-4" onSubmit={event => {
      event.preventDefault();
      if (ready && mod && current) install.mutate({ serverId, modId, generation: current.spec.generation, serverName: current.name, modName: mod.title || mod.fileName });
    }}>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="text-sm text-slate-300">{zh ? "待安装模组" : "Mod to install"}
          <select className={selectClass} value={mod?.id ?? ""} disabled={install.isPending} onChange={event => { setModId(event.target.value); setServerId(""); setNeedsRefresh(false); }}>
            <option value="">{zh ? "选择模组" : "Choose a mod"}</option>
            {mods.filter(item => item.source === "upload" && item.organizationId).map(item => <option key={item.id} value={item.id}>{item.title || item.fileName} · {workspaces.find(space => space.id === item.organizationId)?.name || item.organizationId}</option>)}
          </select>
        </label>
        <label className="text-sm text-slate-300">{zh ? "目标实例" : "Target instance"}
          <select className={selectClass} value={targets.some(item => item.id === serverId) ? serverId : ""} disabled={!mod || !servers.isSuccess || install.isPending} onChange={event => { setServerId(event.target.value); setNeedsRefresh(false); }}>
            <option value="">{zh ? "选择实例" : "Choose an instance"}</option>
            {targets.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
      </div>
      {servers.isError && <p role="alert" className="text-sm text-panel-gold">{zh ? "实例列表加载失败。" : "Unable to load instances."}</p>}
      {mod && servers.isSuccess && targets.length === 0 && <p className="text-sm text-slate-400">{zh ? "此工作区没有匹配游戏类型的实例。" : "No matching instances in this workspace."}</p>}
      {serverId && (target.isError ? <p role="alert" className="text-sm text-panel-gold">{zh ? "无法读取实例，请刷新后重新选择。" : "Unable to read the instance. Refresh and select again."}</p> : target.isFetching ? <p className="text-sm text-slate-400">{zh ? "正在核对实例…" : "Checking instance…"}</p> : matches && <p className="text-sm text-slate-400">{requested ? (zh ? "此模组已在实例配置中，请到实例页面查看启动与运行状态。" : "This mod is already in the instance configuration. Check its startup and runtime status on the instance page.") : !local ? (zh ? "远端节点的模组文件分发尚未开放。" : "Mod file delivery to remote nodes is not available yet.") : !stopped ? (zh ? "请先停止实例，再刷新状态。" : "Stop the instance, then refresh its state.") : (zh ? "实例已停服，可以保存安装请求。" : "Instance is stopped and ready for an installation request.")}</p>)}
      <div className="flex flex-wrap gap-3">
        <Button type="submit" disabled={!ready}>{install.isPending ? (zh ? "正在保存…" : "Saving…") : (zh ? "保存安装请求" : "Save installation request")}</Button>
        <Button type="button" variant="secondary" disabled={install.isPending || target.isFetching || servers.isFetching} onClick={async () => {
          const list = await servers.refetch();
          const detail = serverId ? await target.refetch() : undefined;
          if (list.isSuccess && (!detail || detail.isSuccess)) setNeedsRefresh(false);
        }}>{zh ? "刷新实例状态" : "Refresh instance state"}</Button>
      </div>
    </form>
    <div aria-live="polite" className="mt-4 text-sm">
      {install.variables && (install.isSuccess || install.isError) && <p className={install.isSuccess ? "text-panel-green" : "text-panel-gold"}>
        {install.variables.modName} → {install.variables.serverName} · {install.isSuccess ? (zh ? "请求已保存，尚不代表安装完成。下次启动时执行。" : "Request saved. Installation is not yet confirmed; it runs on the next start.") : failure?.uncertain ? (zh ? "结果待确认，请刷新实例核对；不会自动重试。" : "Result is uncertain. Refresh the instance to verify. No automatic retry.") : failure?.status === 409 ? (zh ? "实例或模组已变化，请刷新后核对。" : "The instance or mod changed. Refresh and verify.") : failure?.status === 403 || failure?.status === 404 || failure?.status === 401 ? (zh ? "实例或模组不可访问，请核对工作区权限并刷新。" : "Instance or mod is unavailable. Check workspace permissions and refresh.") : (zh ? "请求未能保存，请刷新后核对模组与实例。" : "Request could not be saved. Refresh and verify the mod and instance.")}
      </p>}
      {needsRefresh && <p className="mt-2 text-slate-400">{zh ? "再次操作前，请点击“刷新实例状态”。" : "Select “Refresh instance state” before another request."}</p>}
    </div>
  </Card>;
}
