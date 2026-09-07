"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card } from "@/components/ui";
import { listGames, listMyOrganizations, uploadWorkspaceMod, WorkspaceModUploadError } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { ModFile } from "@/lib/types";

type UploadResult = { name: string; workspace: string; ok: boolean; uncertain?: boolean; message?: string; uploadId?: string };

export function WorkspaceModLibrary({ mods }: { mods: ModFile[] }) {
  const { locale } = useI18n();
  const zh = locale.startsWith("zh");
  const client = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const spaces = useQuery({ queryKey: ["my-organizations"], queryFn: listMyOrganizations, retry: false });
  const games = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 300_000 });
  const [spaceId, setSpaceId] = useState("");
  const [providerKey, setProviderKey] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [results, setResults] = useState<UploadResult[]>([]);
  const availableSpaces = useMemo(() => spaces.data ?? [], [spaces.data]);
  const providers = useMemo(() => (games.data ?? []).flatMap(game => game.providers
    .filter(provider => provider.capabilities.mods && (provider.uploadExtensions?.length ?? 0) > 0)
    .map(provider => ({ ...provider, label: `${game.name} · ${provider.name}` }))), [games.data]);
  const space = availableSpaces.find(item => item.id === spaceId);
  const provider = providers.find(item => item.key === providerKey);
  useEffect(() => {
    if (!spaceId && availableSpaces.length === 1 && availableSpaces[0]) setSpaceId(availableSpaces[0].id);
  }, [availableSpaces, spaceId]);
  useEffect(() => {
    if (!providerKey && providers.length === 1 && providers[0]) setProviderKey(providers[0].key);
  }, [providers, providerKey]);
  const clearFiles = () => { setFiles([]); if (input.current) input.current.value = ""; };
  const upload = useMutation({
    retry: false,
    mutationFn: async (request: { organizationId: string; workspace: string; providerKey: string; files: File[] }) => {
      const outcomes: UploadResult[] = [];
      for (const file of request.files) {
        try {
          await uploadWorkspaceMod(request.organizationId, request.providerKey, file);
          outcomes.push({ name: file.name, workspace: request.workspace, ok: true });
        } catch (error) {
          outcomes.push({ name: file.name, workspace: request.workspace, ok: false,
            uncertain: error instanceof WorkspaceModUploadError && error.uncertain,
            uploadId: error instanceof WorkspaceModUploadError ? error.uploadId : undefined,
            message: uploadFailureMessage(error, zh) });
        }
      }
      return outcomes;
    },
    onSuccess: async outcomes => {
      setResults(current => [...current, ...outcomes]);
      clearFiles();
      await Promise.all([
        client.invalidateQueries({ queryKey: ["global-mods"] }),
        client.invalidateQueries({ queryKey: ["recommended-mods"] }),
        client.invalidateQueries({ queryKey: ["my-organizations"] })
      ]);
    }
  });
  const invalidFiles = !!provider && files.some(file => !provider.uploadExtensions?.some(extension => file.name.toLowerCase().endsWith(extension.toLowerCase())));
  const ready = spaces.isSuccess && games.isSuccess && !!space && !!provider && files.length > 0 && !invalidFiles && !upload.isPending;
  const selectClass = "mt-2 h-10 w-full rounded-md border border-panel-line bg-panel-card px-3 text-sm text-slate-100 disabled:opacity-50";
  return <div className="space-y-4">
    <Card className="p-5">
      <h2 className="font-semibold text-slate-100">{zh ? "上传到工作区" : "Upload to workspace"}</h2>
      <p className="mt-2 text-sm text-slate-400">{zh ? "文件仅对所属工作区成员可见。同一工作区、同一游戏类型内的同名模组不会被覆盖。" : "Files are visible to workspace members. Existing mods with the same name and provider will not be overwritten."}</p>
      <form className="mt-4 space-y-4" onSubmit={event => {
        event.preventDefault();
        if (ready && space && provider) upload.mutate({ organizationId: space.id, workspace: space.name || space.id, providerKey: provider.key, files: [...files] });
      }}>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="text-sm text-slate-300">{zh ? "所属工作区" : "Workspace"}
            <select className={selectClass} value={space?.id ?? ""} disabled={upload.isPending || !spaces.isSuccess} onChange={event => { setSpaceId(event.target.value); clearFiles(); }}>
              <option value="">{zh ? "选择工作区" : "Choose a workspace"}</option>
              {availableSpaces.map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
            </select>
          </label>
          <label className="text-sm text-slate-300">{zh ? "游戏服务类型" : "Game provider"}
            <select className={selectClass} value={provider?.key ?? ""} disabled={upload.isPending || !games.isSuccess} onChange={event => { setProviderKey(event.target.value); clearFiles(); }}>
              <option value="">{zh ? "选择游戏类型" : "Choose a provider"}</option>
              {providers.map(item => <option key={item.key} value={item.key}>{item.label}</option>)}
            </select>
          </label>
        </div>
        {(spaces.isError || games.isError) && <div role="alert" className="text-sm text-panel-gold">
          {zh ? "上传选项加载失败。" : "Unable to load upload options."}
          <Button type="button" variant="ghost" onClick={() => { void spaces.refetch(); void games.refetch(); }}>{zh ? "重试加载" : "Retry loading"}</Button>
        </div>}
        {spaces.isSuccess && availableSpaces.length === 0 && <p className="text-sm text-panel-gold">{zh ? "你还没有可用工作区，请先加入工作区。" : "Join a workspace before uploading."}</p>}
        {games.isSuccess && providers.length === 0 && <p className="text-sm text-panel-gold">{zh ? "当前没有支持本地模组上传的游戏类型。" : "No providers currently support local mod uploads."}</p>}
        <label className="block text-sm text-slate-300">{zh ? "模组文件" : "Mod files"}
          <input ref={input} type="file" multiple accept={provider?.uploadExtensions?.join(",")} disabled={!space || !provider || upload.isPending}
            className="mt-2 block w-full text-sm text-slate-400 file:mr-4 file:rounded-md file:border-0 file:bg-slate-800 file:px-3 file:py-2 file:text-slate-100"
            onChange={event => setFiles(Array.from(event.target.files ?? []))} />
        </label>
        {provider && <p className="text-xs text-slate-400">{zh ? "支持格式：" : "Supported: "}{provider.uploadExtensions?.join(", ")}</p>}
        {invalidFiles && <p role="alert" className="text-sm text-panel-gold">{zh ? "所选文件与游戏类型不匹配，请重新选择。" : "Select files matching the provider's supported formats."}</p>}
        <Button type="submit" disabled={!ready}>{upload.isPending ? (zh ? "正在上传…" : "Uploading…") : (zh ? "上传所选文件" : "Upload selected files")}</Button>
      </form>
      <div aria-live="polite" className="mt-4 space-y-2">
        {results.map((result, index) => <div key={index} className={`break-words text-sm ${result.ok ? "text-panel-green" : "text-panel-gold"}`}>
          <p>{result.workspace} · {result.name} — {result.ok ? (zh ? "上传成功" : "Uploaded") : result.uncertain ? (zh ? "结果待确认，请刷新列表核对后再操作。不会自动重传。" : "Check the refreshed library before trying again. No automatic retry.") : result.message}</p>
          {result.uploadId && <p>{zh ? "上传记录 ID：" : "Upload ID: "}<code className="select-all">{result.uploadId}</code></p>}
        </div>)}
      </div>
    </Card>
    <Card className="overflow-x-auto p-4">
      <p className="mb-3 text-sm text-slate-400">{zh ? "模组按所属工作区保存。安装、删除与在线导入暂未开放。" : "Mods are stored in their workspace. Installation, deletion and online import are not yet available."}</p>
      <table className="w-full text-left text-sm">
        <thead><tr className="text-slate-400"><th className="p-2">{zh ? "模组" : "Mod"}</th><th className="p-2">{zh ? "工作区" : "Workspace"}</th><th className="p-2">{zh ? "游戏类型" : "Provider"}</th><th className="p-2">{zh ? "大小" : "Size"}</th></tr></thead>
        <tbody>{mods.map(item => <tr key={item.id} className="border-t border-panel-line"><td className="p-2 text-slate-100">{item.title || item.modName || item.fileName}<p className="text-xs text-slate-500">{item.fileName}</p></td><td className="p-2 text-slate-300">{availableSpaces.find(space => space.id === item.organizationId)?.name || item.organizationId}</td><td className="p-2 text-slate-300">{providers.find(provider => provider.key === item.providerKey)?.label || item.providerKey}</td><td className="p-2 text-slate-300">{item.size}</td></tr>)}</tbody>
      </table>
      {mods.length === 0 && <p className="p-2 text-sm text-slate-400">{zh ? "暂无模组" : "No mods yet"}</p>}
    </Card>
  </div>;
}

function uploadFailureMessage(error: unknown, zh: boolean): string {
  const messages: Record<number, [string, string]> = {
    400: ["文件格式或内容不符合所选游戏类型的要求。", "The file format or contents do not match this provider."],
    403: ["你没有此工作区的上传权限，或权限已失效。", "Workspace upload permission is missing or was revoked."],
    404: ["工作区已不存在，请重新选择。", "The workspace no longer exists. Choose another workspace."],
    409: ["此工作区、游戏类型内已存在同名模组，原文件未被覆盖。", "A mod with this filename and provider already exists in the workspace. It was not overwritten."],
    413: ["文件超过服务器允许的上传大小。", "The file exceeds the server's upload size limit."]
  };
  const message = error instanceof WorkspaceModUploadError ? messages[error.status] : undefined;
  return message ? message[zh ? 0 : 1] : error instanceof Error ? error.message : String(error);
}
