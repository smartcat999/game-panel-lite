"use client";

/*
 * Server (Instance) API helpers shared by the server list and detail pages.
 * Endpoints, bodies and the file-ticket flow are ported from the legacy console.
 */

import { useCallback, useState } from "react";
import { useConsole } from "../../../console/ConsoleProvider";
import { errorMessage, localizeApiError } from "../../../console/lib/session";
import { buildWsUrl } from "../../../console/lib/ws";
import type { FileEntry, InstanceItem, NodeItem } from "../../../console/lib/types";

export function instancePath(tenant: string, name: string) {
  return `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/instances/${encodeURIComponent(name)}`;
}

export function buildExecTtyWsUrl(
  tenant: string,
  name: string,
  cols = 80,
  rows = 24,
  token?: string | null,
): string {
  return buildWsUrl(
    `${instancePath(tenant, name)}/exec`,
    {
      tty: true,
      cols: Math.max(10, Math.round(cols)),
      rows: Math.max(5, Math.round(rows)),
    },
    token,
  );
}

export function buildNodeExecTtyWsUrl(
  nodeName: string,
  cols = 80,
  rows = 24,
  token?: string | null,
): string {
  return buildWsUrl(
    `/apis/core.gamepanel.io/v1/nodes/${encodeURIComponent(nodeName)}/exec`,
    {
      tty: true,
      cols: Math.max(10, Math.round(cols)),
      rows: Math.max(5, Math.round(rows)),
    },
    token,
  );
}

const msg = (err: unknown, fallback: string) => (err instanceof Error && err.message ? err.message : fallback);

function isPrivateOrDockerHost(host: string): boolean {
  const h = host.trim().toLowerCase();
  if (!h || h === "localhost" || h === "node-agent" || h.startsWith("127.") || h.startsWith("10.") || h.startsWith("192.168.")) {
    return true;
  }
  if (h.startsWith("172.")) {
    const second = Number(h.split(".")[1]);
    if (second >= 16 && second <= 31) return true;
  }
  return false;
}

function toSameOriginDirectURL(rawUrl: string): URL {
  const url = new URL(rawUrl);
  if (typeof window === "undefined") return url;
  if (url.pathname.endsWith("/files") && url.searchParams.has("ticket")) {
    if (
      isPrivateOrDockerHost(url.hostname) ||
      url.hostname === window.location.hostname ||
      (window.location.protocol === "https:" && url.protocol === "http:")
    ) {
      return new URL(`${url.pathname}${url.search}`, window.location.origin);
    }
  }
  if (url.hostname === "seaweedfs" || url.hostname === "minio") {
    return new URL(`${url.pathname}${url.search}`, window.location.origin);
  }
  return url;
}

export type ServerBusy = "start" | "stop" | "restart" | "delete" | "update";

/** Start / stop / restart / delete with per-server busy state and toasts. */
export function useServerActions() {
  const { tenant, apiFetch, toast, reloadInstances } = useConsole();
  const [busy, setBusy] = useState<Record<string, ServerBusy | undefined>>({});
  const mark = (name: string, v?: ServerBusy) => setBusy((prev) => ({ ...prev, [name]: v }));

  const putState = useCallback(
    async (name: string, desired: "Running" | "Stopped" | "Restart") => {
      const fallback = desired === "Running" ? "启动失败" : desired === "Restart" ? "重启失败" : "停止失败";
      const res = await apiFetch(`${instancePath(tenant, name)}/state`, {
        method: "PUT",
        body: JSON.stringify({ state: desired }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, fallback));
    },
    [apiFetch, tenant],
  );

  async function setState(inst: InstanceItem, desired: "Running" | "Stopped") {
    const name = inst.metadata.name;
    mark(name, desired === "Running" ? "start" : "stop");
    try {
      await putState(name, desired);
      toast(
        "success",
        desired === "Running" ? `${name} 正在启动，稍等片刻就能连接` : `${name} 正在停止，存档会在停止前保存`,
      );
      await reloadInstances();
      return true;
    } catch (err) {
      toast("error", `${msg(err, "操作失败")}，请稍后重试`);
      return false;
    } finally {
      mark(name);
    }
  }

  /** Native container restart via PUT /state {spec:{state:"Restart"}}. */
  async function restart(inst: InstanceItem) {
    const name = inst.metadata.name;
    mark(name, "restart");
    try {
      await putState(name, "Restart");
      toast("success", `${name} 正在重启`);
      await reloadInstances();
      return true;
    } catch (err) {
      toast("error", `重启没有完成：${msg(err, "未知错误")}。可以手动再点一次启动`);
      return false;
    } finally {
      mark(name);
    }
  }

  async function remove(inst: InstanceItem) {
    const name = inst.metadata.name;
    mark(name, "delete");
    try {
      const res = await apiFetch(instancePath(tenant, name), { method: "DELETE" });
      if (!res.ok) throw new Error(await errorMessage(res, "删除失败"));
      toast("success", `服务器 ${name} 已删除`);
      await reloadInstances();
      return true;
    } catch (err) {
      toast("error", `${msg(err, "删除失败")}，请稍后重试`);
      return false;
    } finally {
      mark(name);
    }
  }

  function applyInstancePatch(
    base: InstanceItem,
    specPatch: Partial<InstanceItem["spec"]>,
    annotationsPatch?: Record<string, string>,
  ): InstanceItem {
    const nextAnnotations = { ...(base.metadata.annotations || {}) };
    if (annotationsPatch) {
      for (const [k, v] of Object.entries(annotationsPatch)) {
        if (v === "") {
          delete nextAnnotations[k];
        } else {
          nextAnnotations[k] = v;
        }
      }
    }
    return {
      ...base,
      metadata: annotationsPatch
        ? {
            ...base.metadata,
            annotations: nextAnnotations,
          }
        : base.metadata,
      spec: {
        ...base.spec,
        ...specPatch,
      },
    };
  }

  async function updateSpec(
    inst: InstanceItem,
    specPatch: Partial<InstanceItem["spec"]>,
    successMsg = "配置已保存，容器正在自动应用新配置",
    annotationsPatch?: Record<string, string>,
  ) {
    const name = inst.metadata.name;
    const url = instancePath(tenant, name);
    mark(name, "update");
    try {
      let current = inst;
      let res: Response | null = null;
      for (let attempt = 0; attempt < 3; attempt++) {
        const payload = applyInstancePatch(current, specPatch, annotationsPatch);
        res = await apiFetch(url, {
          method: "PUT",
          body: JSON.stringify(payload),
        });
        if (res.status !== 409 || attempt === 2) break;
        const latestRes = await apiFetch(url);
        if (!latestRes.ok) break;
        current = (await latestRes.json()) as InstanceItem;
      }
      if (!res || !res.ok) {
        throw new Error(res ? await errorMessage(res, "保存配置失败") : "保存配置失败");
      }
      if (successMsg) toast("success", successMsg);
      await reloadInstances();
      return true;
    } catch (err) {
      toast("error", `${msg(err, "保存配置失败")}，请稍后重试`);
      return false;
    } finally {
      mark(name);
    }
  }

  return { busy, setState, restart, remove, updateSpec };
}

/** Logs + exec for one server. */
export function useServerShell(name: string) {
  const { tenant, apiFetch } = useConsole();

  const fetchLogs = useCallback(async (): Promise<string> => {
    try {
      const res = await apiFetch(`${instancePath(tenant, name)}/log?tailLines=200`);
      if (!res.ok) return `读取日志失败：${await errorMessage(res, "服务器暂时无法响应")}`;
      const text = await res.text();
      return text || "暂无日志输出";
    } catch (err) {
      return `读取日志失败：${msg(err, "网络异常")}`;
    }
  }, [apiFetch, tenant, name]);

  /**
   * Follows the container stdout in real time (`follow=true`) and pushes decoded chunks to `onChunk`.
   * Resolves when the stream ends (container stopped / restarted) or `signal` aborts.
   */
  const streamLogs = useCallback(
    async (onChunk: (text: string) => void, signal: AbortSignal, tailLines = 200): Promise<void> => {
      const res = await apiFetch(`${instancePath(tenant, name)}/log?follow=true&tailLines=${tailLines}`, { signal });
      if (!res.ok) throw new Error(await errorMessage(res, "读取日志失败"));
      if (!res.body) {
        onChunk(await res.text());
        return;
      }
      onChunk("");
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      try {
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          if (value) onChunk(decoder.decode(value, { stream: true }));
        }
      } finally {
        reader.releaseLock();
      }
    },
    [apiFetch, tenant, name],
  );

  const exec = useCallback(
    async (command: string): Promise<{ ok: boolean; output: string }> => {
      try {
        const res = await apiFetch(`${instancePath(tenant, name)}/exec`, {
          method: "POST",
          body: JSON.stringify({ command: ["sh", "-c", command] }),
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) return { ok: false, output: localizeApiError(data?.message || "", res.status, "命令执行失败") };
        const output: string = data?.output ?? "";
        return { ok: true, output: output.trim() ? output : "(命令执行成功，无输出)" };
      } catch (err) {
        return { ok: false, output: msg(err, "命令执行异常") };
      }
    },
    [apiFetch, tenant, name],
  );

  const sendGameCommand = useCallback(
    async (gameCommand: string, delivery?: string): Promise<{ ok: boolean; output: string }> => {
      const trimmed = gameCommand.trim();
      if (!trimmed) return { ok: false, output: "指令不能为空" };
      const channel = (delivery || "stdin").trim();
      let argv: string[];
      if (!channel || channel === "stdin") {
        argv = ["__gp_stdin__", trimmed];
      } else if (channel.includes("{{cmd}}")) {
        const safeShell = channel.replace(/\{\{\s*cmd\s*\}\}/g, '"$1"');
        argv = ["sh", "-c", safeShell, "gp-cmd", trimmed];
      } else {
        argv = [channel, trimmed];
      }
      try {
        const res = await apiFetch(`${instancePath(tenant, name)}/exec`, {
          method: "POST",
          body: JSON.stringify({ command: argv }),
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) return { ok: false, output: localizeApiError(data?.message || "", res.status, "指令发送失败") };
        return { ok: true, output: data?.output ?? "" };
      } catch (err) {
        return { ok: false, output: msg(err, "指令发送异常") };
      }
    },
    [apiFetch, tenant, name],
  );

  return { fetchLogs, streamLogs, exec, sendGameCommand };
}

/** node-agent errors are plain text (http.Error); apiserver errors are JSON. */
async function ensureOk(res: Response, fallback: string) {
  if (res.ok) return res;
  const text = (await res.text().catch(() => "")).trim();
  let message = text;
  try {
    const body = JSON.parse(text);
    message = body?.message || body?.error || text;
  } catch {
    /* plain text */
  }
  const localized = localizeApiError(message, res.status, fallback);
  throw new Error(localized.startsWith(fallback) ? localized : `${fallback}：${localized}`);
}

/** Ticket actions accepted by node-agent; "delete" maps to HTTP DELETE on the agent. */
export type FileAction = "list" | "read" | "write" | "mkdir" | "rename" | "delete";

export interface RestoreArchiveOptions {
  sourcePath?: string;
  targetPath?: string;
  replaceTarget?: boolean;
}

export interface BackupConfigPreviewItem {
  name: string;
  label: string;
  group?: string;
  type?: string;
  archiveValue?: string;
  currentValue?: string;
  archiveSet?: boolean;
  currentSet?: boolean;
  different?: boolean;
  overriddenByPanel?: boolean;
  secret?: boolean;
}

export interface BackupAddonPreviewItem {
  repository?: string;
  package: string;
  title?: string;
  backupVersion?: string;
  currentVersion?: string;
  backupDisabled?: boolean;
  currentDisabled?: boolean;
  installedNow?: boolean;
  different?: boolean;
  configCount?: number;
  scopes?: string[];
}

export interface BackupInspectResult {
  path: string;
  phase?: string;
  sizeBytes?: number;
  completedAt?: string;
  archiveFormat?: string;
  recommendedSource?: string;
  sourcePath?: string;
  targetPath?: string;
  replaceTarget?: boolean;
  candidateSources?: string[];
  archiveDirectories?: string[];
  sourceEntries?: string[];
  managedFiles?: string[];
  fileCount?: number;
  uncompressedBytes?: number;
  serverVersion?: string;
  currentServerVersion?: string;
  configPreview?: BackupConfigPreviewItem[];
  addonPreview?: BackupAddonPreviewItem[];
}

export interface ArchiveResult {
  path: string;
  operationID?: string;
  phase?: string;
  sizeBytes?: number;
  sha256?: string;
  completedAt?: string;
}

/**
 * File operations for one server.
 *
 * list/read/write/mkdir/delete: POST apiserver …/files {spec:{action,path}} issues a short-lived
 * signed ticket and `status.directURL` (node-agent, query already has ticket+path). The bulk call
 * then goes straight to node-agent:
 *   list → GET (JSON InstanceFiles, entries in status.entries) · read → GET (raw file bytes)
 *   write → PUT (raw body) · delete → DELETE · mkdir → POST {spec:{action:"mkdir",path}}
 *
 * backup/restore/delete_backup: Declarative async workflow — POST apiserver …/files records the stateful
 * backup command (`PendingQuota` / `Restoring` / `Deleting`), projects it into `status.backups`, and
 * publishes an MQ event for `controller-manager` to execute asynchronously.
 */
async function xhrPutBlob(
  url: string,
  body: Blob,
  contentType: string,
  onProgress?: (pct: number) => void,
  networkErrMsg = "无法连接到存储节点，请检查网络后重试",
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url, true);
    if (contentType) {
      xhr.setRequestHeader("Content-Type", contentType);
    }
    xhr.upload.onprogress = (ev) => {
      if (!onProgress) return;
      const total = ev.lengthComputable && ev.total > 0 ? ev.total : body.size;
      if (total > 0) {
        onProgress(Math.min(100, Math.round((ev.loaded / total) * 100)));
      }
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.(100);
        resolve();
      } else {
        reject(new Error(`上传文件失败（HTTP ${xhr.status}）`));
      }
    };
    xhr.onerror = () => reject(new Error(networkErrMsg));
    xhr.send(body);
  });
}

export function useServerFiles(name: string) {
  const { tenant, apiFetch } = useConsole();

  const direct = useCallback(
    async (action: FileAction, path: string, init?: RequestInit) => {
      const ticketRes = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({ action, path }),
      });
      const ticketBody = await ticketRes.json().catch(() => ({}));
      if (!ticketRes.ok) {
        throw new Error(localizeApiError(ticketBody?.message || "", ticketRes.status, "获取文件访问凭证失败"));
      }
      const directURL: string = ticketBody?.directURL || "";
      if (!directURL) throw new Error("服务器所在节点没有返回文件访问地址");
      const url = toSameOriginDirectURL(directURL);
      url.searchParams.set("path", path);
      try {
        // Plain fetch: the signed ticket authorizes the request, no console bearer token needed.
        return await fetch(url.toString(), init);
      } catch {
        throw new Error("无法连接到服务器所在节点，请检查网络后重试");
      }
    },
    [apiFetch, tenant, name],
  );

  const list = useCallback(
    async (dir: string): Promise<FileEntry[]> => {
      const res = await ensureOk(await direct("list", dir), "读取目录失败");
      const body = await res.json().catch(() => ({}));
      return body?.entries || [];
    },
    [direct],
  );

  const read = useCallback(
    async (path: string) => (await ensureOk(await direct("read", path), "读取文件失败")).text(),
    [direct],
  );

  const write = useCallback(
    async (path: string, content: string) => {
      await ensureOk(
        await direct("write", path, {
          method: "PUT",
          headers: { "Content-Type": "text/plain; charset=utf-8" },
          body: content,
        }),
        "保存失败",
      );
    },
    [direct],
  );

  const mkdir = useCallback(
    async (path: string) => {
      await ensureOk(
        await direct("mkdir", path, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action: "mkdir", path }),
        }),
        "新建文件夹失败",
      );
    },
    [direct],
  );

  const rename = useCallback(
    async (oldPath: string, newPath: string) => {
      await ensureOk(
        await direct("rename", oldPath, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action: "rename", path: oldPath, targetPath: newPath }),
        }),
        "重命名失败",
      );
    },
    [direct],
  );

  const rm = useCallback(
    async (path: string) => {
      await ensureOk(await direct("delete", path, { method: "DELETE" }), "删除失败");
    },
    [direct],
  );

  const upload = useCallback(
    async (path: string, file: Blob, onProgress?: (pct: number) => void) => {
      const ticketRes = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({ action: "write", path }),
      });
      const ticketBody = await ticketRes.json().catch(() => ({}));
      if (!ticketRes.ok) {
        throw new Error(localizeApiError(ticketBody?.message || "", ticketRes.status, "获取文件访问凭证失败"));
      }
      const directURL: string = ticketBody?.directURL || "";
      if (!directURL) throw new Error("服务器所在节点没有返回文件访问地址");
      const url = toSameOriginDirectURL(directURL);
      url.searchParams.set("path", path);
      await xhrPutBlob(
        url.toString(),
        file,
        file.type || "application/octet-stream",
        onProgress,
        "无法连接到服务器所在节点，请检查网络后重试",
      );
    },
    [apiFetch, tenant, name],
  );

  const download = useCallback(
    async (path: string, filename: string) => {
      const ticketRes = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({ action: "read", path }),
      });
      const ticketBody = await ticketRes.json().catch(() => ({}));
      if (!ticketRes.ok) {
        throw new Error(localizeApiError(ticketBody?.message || "", ticketRes.status, "获取文件访问凭证失败"));
      }
      const directURL: string = ticketBody?.directURL || "";
      if (!directURL) throw new Error("服务器所在节点没有返回文件访问地址");
      const url = toSameOriginDirectURL(directURL);
      url.searchParams.set("path", path);
      url.searchParams.set("download", "1");

      // Direct browser-to-node-agent streaming download — zero apiserver bandwidth and zero JS Blob buffering.
      const a = document.createElement("a");
      a.href = url.toString();
      a.download = filename || path.split("/").pop() || "download";
      a.rel = "noopener noreferrer";
      document.body.appendChild(a);
      a.click();
      a.remove();
    },
    [apiFetch, tenant, name],
  );

  const inspectBackup = useCallback(
    async (path: string, opts?: { sourcePath?: string; targetPath?: string }): Promise<BackupInspectResult> => {
      const payload: Record<string, unknown> = { action: "inspect_backup", path };
      if (opts?.sourcePath !== undefined) payload.sourcePath = opts.sourcePath;
      if (opts?.targetPath !== undefined) payload.targetPath = opts.targetPath;
      const res = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      await ensureOk(res, "识别备份目录结构失败");
      const body = await res.json().catch(() => ({}));
      return {
        path: body?.path || path,
        phase: body?.phase,
        sizeBytes: typeof body?.sizeBytes === "number" ? body.sizeBytes : undefined,
        completedAt: body?.completedAt,
        archiveFormat: body?.archiveFormat,
        recommendedSource: body?.recommendedSource,
        sourcePath: body?.sourcePath ?? "",
        targetPath: body?.targetPath ?? ".",
        replaceTarget: typeof body?.replaceTarget === "boolean" ? body.replaceTarget : undefined,
        candidateSources: Array.isArray(body?.candidateSources) ? body.candidateSources : [],
        archiveDirectories: Array.isArray(body?.archiveDirectories) ? body.archiveDirectories : ["."],
        sourceEntries: Array.isArray(body?.sourceEntries) ? body.sourceEntries : [],
        managedFiles: Array.isArray(body?.managedFiles) ? body.managedFiles : [],
        fileCount: typeof body?.fileCount === "number" ? body.fileCount : undefined,
        uncompressedBytes: typeof body?.uncompressedBytes === "number" ? body.uncompressedBytes : undefined,
        serverVersion: typeof body?.serverVersion === "string" && body.serverVersion ? body.serverVersion : undefined,
        currentServerVersion:
          typeof body?.currentServerVersion === "string" && body.currentServerVersion
            ? body.currentServerVersion
            : undefined,
        configPreview: Array.isArray(body?.configPreview) ? body.configPreview : [],
        addonPreview: Array.isArray(body?.addonPreview) ? body.addonPreview : [],
      };
    },
    [apiFetch, tenant, name],
  );

  const archive = useCallback(
    async (mode: "backup" | "restore", path: string, opts?: RestoreArchiveOptions): Promise<ArchiveResult> => {
      const payload: Record<string, unknown> = { action: mode, path };
      if (mode === "restore" && opts) {
        if (opts.sourcePath !== undefined) payload.sourcePath = opts.sourcePath;
        if (opts.targetPath !== undefined) payload.targetPath = opts.targetPath;
        if (opts.replaceTarget !== undefined) payload.replaceTarget = opts.replaceTarget;
      }
      const res = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      await ensureOk(res, mode === "backup" ? "备份失败" : "恢复失败");
      const body = await res.json().catch(() => ({}));
      return {
        path: body?.path || path,
        operationID: body?.operationID,
        phase: body?.phase,
        sizeBytes: body?.sizeBytes,
        sha256: body?.sha256,
        completedAt: body?.completedAt,
      };
    },
    [apiFetch, tenant, name],
  );

  const deleteBackup = useCallback(
    async (path: string): Promise<void> => {
      const res = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({ action: "delete_backup", path }),
      });
      await ensureOk(res, "删除备份失败");
    },
    [apiFetch, tenant, name],
  );

  const downloadBackup = useCallback(
    async (path: string, displayName?: string): Promise<void> => {
      const res = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({ action: "download_backup", path }),
      });
      await ensureOk(res, "获取备份下载链接失败");
      const body = await res.json().catch(() => ({}));
      const directURL: string = body?.directURL || "";
      if (!directURL) throw new Error("未返回备份下载地址");
      const downloadURL = toSameOriginDirectURL(directURL).toString();

      const cleanPath = path.split("?")[0];
      const extMatch = cleanPath.match(/\.(tar\.gz|tgz|zip)$/i);
      const ext = extMatch ? (extMatch[1].toLowerCase() === "tar.gz" ? ".tar.gz" : `.${extMatch[1].toLowerCase()}`) : ".tar.gz";
      const safeLabel = (displayName || path.split("/").pop() || name)
        .replace(/\.(tar\.gz|tgz|zip)$/i, "")
        .replace(/[\\/:*?"<>|\s]+/g, "-");
      const filename = `${name}-${safeLabel}${ext}`;

      // Direct S3 Pre-signed URL download streams straight from object storage
      // with zero control-plane (apiserver / web) bandwidth.
      const a = document.createElement("a");
      a.href = downloadURL;
      a.download = filename;
      a.rel = "noopener noreferrer";
      document.body.appendChild(a);
      a.click();
      a.remove();
    },
    [apiFetch, tenant, name],
  );

  const uploadArchive = useCallback(
    async (archivePath: string, file: Blob, onProgress?: (pct: number) => void): Promise<ArchiveResult> => {
      const initRes = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({
          action: "upload_backup",
          path: archivePath,
          sizeBytes: file.size || 0,
        }),
      });
      await ensureOk(initRes, "获取备份上传地址失败");
      const initBody = await initRes.json().catch(() => ({}));
      const directURL: string = initBody?.directURL || "";
      const operationID: string = initBody?.operationID || "";
      const objectPath: string = initBody?.path || archivePath;
      if (!directURL) throw new Error("平台对象存储未返回上传地址");

      const uploadURL = toSameOriginDirectURL(directURL).toString();
      const contentType = /\.zip$/i.test(objectPath || archivePath) ? "application/zip" : "application/gzip";
      await xhrPutBlob(
        uploadURL,
        file,
        contentType,
        onProgress,
        "无法连接到备份存储服务，请检查网络后重试",
      );

      const doneRes = await apiFetch(`${instancePath(tenant, name)}/files`, {
        method: "POST",
        body: JSON.stringify({
          action: "complete_upload_backup",
          operationID,
          path: objectPath,
        }),
      });
      await ensureOk(doneRes, "确认备份记录失败");
      const doneBody = await doneRes.json().catch(() => ({}));
      return {
        path: doneBody?.path || objectPath,
        operationID: doneBody?.operationID || operationID,
        phase: doneBody?.phase || "Unknown",
        sizeBytes: doneBody?.sizeBytes || file.size,
        sha256: doneBody?.sha256,
        completedAt: doneBody?.completedAt,
      };
    },
    [apiFetch, tenant, name],
  );

  return { list, read, write, mkdir, rename, rm, upload, download, inspectBackup, archive, deleteBackup, downloadBackup, uploadArchive };
}

/** Public host for a server: endpoint hostIP, else the node's External/Internal IP. */
export function hostOf(inst: InstanceItem, nodes: NodeItem[]): string {
  if (inst.status?.endpoint?.hostIP) return inst.status.endpoint.hostIP;
  if (!inst.spec.nodeName) return "";
  const node = nodes.find((n) => n.metadata.name === inst.spec.nodeName);
  return (
    node?.status?.addresses?.find((a) => a.type === "ExternalIP")?.address ||
    node?.status?.addresses?.find((a) => a.type === "InternalIP")?.address ||
    ""
  );
}

export { portsOf } from "../../../console/lib/format";
