"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Box, Plus, RefreshCw, Star, Terminal, Trash2, Upload } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatBytes } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import { subscribeResourceWatch } from "../../../console/lib/ws";
import type {
  AppServerBinding,
  KubeList,
  RegistryArtifact,
  RegistryBlobUpload,
  RegistryPackage,
  RegistryRepository,
} from "../../../console/lib/types";
import { ArtifactLogModal } from "./ArtifactLogModal";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  IconButton,
  Input,
  type MenuItem,
  Modal,
  Mono,
  MoreMenu,
  Segmented,
  Select,
} from "../ui";

const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

function sha256Pure(bytes: Uint8Array): string {
  const rotr = (x: number, n: number) => (x >>> n) | (x << (32 - n));
  const bitLen = bytes.length * 8;
  const padLen = ((bytes.length + 9 + 63) >> 6) << 6;
  const padded = new Uint8Array(padLen);
  padded.set(bytes);
  padded[bytes.length] = 0x80;
  const view = new DataView(padded.buffer);
  view.setUint32(padLen - 8, Math.floor(bitLen / 0x100000000), false);
  view.setUint32(padLen - 4, bitLen >>> 0, false);

  let h0 = 0x6a09e667;
  let h1 = 0xbb67ae85;
  let h2 = 0x3c6ef372;
  let h3 = 0xa54ff53a;
  let h4 = 0x510e527f;
  let h5 = 0x9b05688c;
  let h6 = 0x1f83d9ab;
  let h7 = 0x5be0cd19;
  const w = new Uint32Array(64);

  for (let off = 0; off < padLen; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4, false);
    for (let i = 16; i < 64; i++) {
      const s0 = rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >>> 3);
      const s1 = rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >>> 10);
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) >>> 0;
    }
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    let e = h4;
    let f = h5;
    let g = h6;
    let h = h7;
    for (let i = 0; i < 64; i++) {
      const S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
      const ch = (e & f) ^ (~e & g);
      const t1 = (h + S1 + ch + SHA256_K[i] + w[i]) >>> 0;
      const S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const t2 = (S0 + maj) >>> 0;
      h = g;
      g = f;
      f = e;
      e = (d + t1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (t1 + t2) >>> 0;
    }
    h0 = (h0 + a) >>> 0;
    h1 = (h1 + b) >>> 0;
    h2 = (h2 + c) >>> 0;
    h3 = (h3 + d) >>> 0;
    h4 = (h4 + e) >>> 0;
    h5 = (h5 + f) >>> 0;
    h6 = (h6 + g) >>> 0;
    h7 = (h7 + h) >>> 0;
  }
  return [h0, h1, h2, h3, h4, h5, h6, h7]
    .map((v) => v.toString(16).padStart(8, "0"))
    .join("");
}

async function sha256Hex(file: File): Promise<string> {
  const buf = await file.arrayBuffer();
  if (typeof globalThis.crypto !== "undefined" && globalThis.crypto.subtle) {
    const hash = await globalThis.crypto.subtle.digest("SHA-256", buf);
    return Array.from(new Uint8Array(hash))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }
  return sha256Pure(new Uint8Array(buf));
}

function sanitizeKubeName(raw: string): string {
  const cleaned = raw
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return (cleaned || "custom-game").slice(0, 52).replace(/-+$/g, "");
}

function inferArchiveFormat(fileName: string): string {
  const lower = fileName.toLowerCase();
  if (lower.endsWith(".tar.gz") || lower.endsWith(".tgz")) return "tar.gz";
  if (lower.endsWith(".zip")) return "zip";
  if (lower.endsWith(".jar")) return "jar";
  if (lower.endsWith(".tmod")) return "tmod";
  return "raw";
}

export interface GameVersionsPanelProps {
  appName: string;
  appDisplayName: string;
  versions: string[];
  recommendedVersion: string;
  serverBinding?: AppServerBinding;
  onVersionsChange: (nextVersions: string[], nextRecommended: string) => void;
  onServerBindingChange: (next: AppServerBinding | undefined) => void;
  onPersistVersions?: (
    nextVersions: string[],
    nextRecommended: string,
    nextBinding?: AppServerBinding,
  ) => Promise<void>;
}

type AddVersionMode = "official" | "upload";

async function putWithProgress(
  url: string,
  body: Blob,
  onProgress: (loaded: number, total: number) => void,
): Promise<{ etag?: string }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url, true);
    xhr.upload.onprogress = (ev) => {
      onProgress(ev.loaded, ev.lengthComputable && ev.total > 0 ? ev.total : body.size);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve({ etag: xhr.getResponseHeader("ETag") || undefined });
      } else {
        reject(new Error(`文件分片上传失败 (${xhr.status})`));
      }
    };
    xhr.onerror = () => reject(new Error("上传网络中断，请稍后重试"));
    xhr.send(body);
  });
}

export function GameVersionsPanel({
  appName,
  appDisplayName,
  versions,
  recommendedVersion,
  serverBinding,
  onVersionsChange,
  onServerBindingChange,
  onPersistVersions,
}: GameVersionsPanelProps) {
  const { apiFetch, toast, token } = useConsole();

  const [serverRepo, setServerRepo] = useState<RegistryRepository | null>(null);
  const [serverArtifacts, setServerArtifacts] = useState<RegistryArtifact[]>([]);
  const [upstreamVersions, setUpstreamVersions] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyVersion, setBusyVersion] = useState<string | null>(null);

  const [addModalOpen, setAddModalOpen] = useState(false);
  const [addMode, setAddMode] = useState<AddVersionMode>("official");
  const [officialInput, setOfficialInput] = useState("");
  const [customOfficialInput, setCustomOfficialInput] = useState("");
  const [uploadVersion, setUploadVersion] = useState("");
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadPercent, setUploadPercent] = useState<number | null>(null);
  const [savingAdd, setSavingAdd] = useState(false);

  const [removingVersion, setRemovingVersion] = useState<string | null>(null);
  const [removingBusy, setRemovingBusy] = useState(false);
  const [logArtifact, setLogArtifact] = useState<RegistryArtifact | null>(null);

  const serverRepoName = serverBinding?.repository?.trim() || "";
  const serverPkgName = serverBinding?.package?.trim() || "";
  const hasOfficialSource = Boolean(serverRepoName && serverPkgName) && serverRepo?.spec.mode !== "Hosted";

  const loadData = useCallback(async () => {
    if (!serverRepoName) {
      setServerRepo(null);
      setServerArtifacts([]);
      setUpstreamVersions([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const repoPromise = apiFetch(
        `/apis/registry.gamepanel.io/v1/repositories/${encodeURIComponent(serverRepoName)}`,
      );
      const artPromise = apiFetch(
        `/apis/registry.gamepanel.io/v1/artifacts?fieldSelector=${encodeURIComponent(`repository=${serverRepoName}`)}&limit=200`,
      );
      const pkgPromise = serverPkgName
        ? apiFetch(
            `/apis/registry.gamepanel.io/v1/packages/${encodeURIComponent(`${serverRepoName}.${serverPkgName}`)}`,
          )
        : null;

      const [repoRes, artRes] = await Promise.all([repoPromise, artPromise]);
      let loadedRepo: RegistryRepository | null = null;
      if (repoRes?.ok) {
        loadedRepo = (await repoRes.json()) as RegistryRepository;
        setServerRepo(loadedRepo);
      } else {
        setServerRepo(null);
      }
      if (artRes?.ok) {
        const list = (await artRes.json()) as KubeList<RegistryArtifact>;
        setServerArtifacts((list.items || []).filter((a) => !a.metadata.deletionTimestamp));
      } else {
        setServerArtifacts([]);
      }
      if (pkgPromise && loadedRepo?.spec.mode !== "Hosted") {
        void pkgPromise
          .then(async (pkgRes) => {
            if (!pkgRes.ok) {
              setUpstreamVersions([]);
              return;
            }
            const pkg = (await pkgRes.json()) as RegistryPackage;
            setUpstreamVersions((pkg.availableVersions || []).filter(Boolean));
          })
          .catch(() => {
            setUpstreamVersions([]);
          });
      } else {
        setUpstreamVersions([]);
      }
    } finally {
      setLoading(false);
    }
  }, [apiFetch, serverRepoName, serverPkgName]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    if (!token || !serverRepoName) return;
    return subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: () => void loadData(),
      onEvent: (ev) => {
        const obj = ev.object;
        if (!obj?.metadata?.name || obj.spec?.repository !== serverRepoName) return;
        setServerArtifacts((prev) => {
          if (ev.type === "DELETED") {
            return prev.filter((x) => x.metadata.name !== obj.metadata.name);
          }
          const idx = prev.findIndex((x) => x.metadata.name === obj.metadata.name);
          if (idx === -1) return [obj, ...prev];
          const next = [...prev];
          next[idx] = obj;
          return next;
        });
      },
    });
  }, [token, serverRepoName, loadData]);

  async function triggerSyncVersion(ver: string) {
    if (!serverRepoName || !serverPkgName) {
      toast("error", "当前游戏尚未绑定镜像站仓库");
      return;
    }
    setBusyVersion(ver);
    try {
      const existing = artifactByVersion.get(ver);
      if (existing) {
        await apiFetch(
          `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(existing.metadata.name)}`,
          { method: "DELETE" },
        );
      }
      const res = await apiFetch("/apis/registry.gamepanel.io/v1/artifacts", {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "registry.gamepanel.io/v1",
          kind: "Artifact",
          spec: {
            repository: serverRepoName,
            package: serverPkgName,
            version: ver,
          },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "触发镜像站同步失败"));
      const created = (await res.json().catch(() => null)) as RegistryArtifact | null;
      toast("success", `已开始同步版本 ${ver}`);
      if (created?.metadata?.name) {
        setServerArtifacts((prev) => [
          created,
          ...prev.filter((x) => x.metadata.name !== created.metadata.name),
        ]);
      }
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "同步版本失败");
    } finally {
      setBusyVersion(null);
    }
  }

  const artifactByVersion = useMemo(() => {
    const map = new Map<string, RegistryArtifact>();
    for (const a of serverArtifacts) {
      if (serverPkgName && a.spec.package !== serverPkgName) continue;
      map.set(a.spec.version, a);
    }
    return map;
  }, [serverArtifacts, serverPkgName]);

  const displayVersions = useMemo(() => {
    const out: string[] = [];
    const seen = new Set<string>();
    const push = (v: string) => {
      const t = v.trim();
      if (!t || seen.has(t)) return;
      seen.add(t);
      out.push(t);
    };
    if (recommendedVersion) push(recommendedVersion);
    for (const v of versions) push(v);
    for (const a of serverArtifacts) {
      if (a.spec.descriptor && !a.spec.sourceURL) push(a.spec.version);
    }
    return out;
  }, [versions, recommendedVersion, serverArtifacts]);

  const availableOfficialChoices = useMemo(
    () => upstreamVersions.filter((v) => !displayVersions.includes(v)),
    [upstreamVersions, displayVersions],
  );

  const effectiveDefault = recommendedVersion || displayVersions[0] || "";

  async function commitVersions(
    nextVersions: string[],
    nextRec: string,
    nextBinding?: AppServerBinding,
  ) {
    onVersionsChange(nextVersions, nextRec);
    if (onPersistVersions) {
      await onPersistVersions(nextVersions, nextRec, nextBinding);
    }
  }

  async function handleSetDefault(ver: string) {
    setBusyVersion(ver);
    try {
      const nextList = displayVersions.includes(ver) ? displayVersions : [ver, ...displayVersions];
      await commitVersions(nextList, ver);
      toast("success", `已将 ${ver} 设为默认版本`);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "设置默认版本失败");
    } finally {
      setBusyVersion(null);
    }
  }

  function openAddVersionModal(prefillUploadVersion?: string) {
    if (prefillUploadVersion) {
      setAddMode("upload");
      setUploadVersion(prefillUploadVersion);
    } else {
      setAddMode(hasOfficialSource ? "official" : "upload");
      setUploadVersion("");
    }
    setOfficialInput(availableOfficialChoices[0] || "");
    setCustomOfficialInput("");
    setUploadFile(null);
    setAddModalOpen(true);
  }

  async function ensureServerBindingForUpload(): Promise<AppServerBinding> {
    if (serverBinding?.repository && serverBinding?.package) {
      return serverBinding;
    }
    const baseName = sanitizeKubeName(appName || appDisplayName || "custom-game");
    const repoName = `${baseName}-server`;
    const checkRes = await apiFetch(
      `/apis/registry.gamepanel.io/v1/repositories/${encodeURIComponent(repoName)}`,
    );
    if (checkRes.status === 404) {
      const createRepo = await apiFetch("/apis/registry.gamepanel.io/v1/repositories", {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "registry.gamepanel.io/v1",
          kind: "Repository",
          metadata: { name: repoName },
          spec: {
            displayName: appDisplayName || baseName,
            mode: "Hosted",
            format: "server_package",
          },
        }),
      });
      if (!createRepo.ok && createRepo.status !== 409) {
        throw new Error(await errorMessage(createRepo, "准备镜像站存储空间失败"));
      }
    }
    const binding: AppServerBinding = {
      repository: repoName,
      package: baseName,
      targetPath: ".",
    };
    onServerBindingChange(binding);
    return binding;
  }

  async function submitOfficialVersion() {
    const ver = (
      availableOfficialChoices.length > 0 && officialInput !== "__custom__"
        ? officialInput
        : officialInput === "__custom__"
          ? customOfficialInput
          : officialInput
    ).trim();
    if (!ver) {
      toast("error", "请选择或输入游戏版本号");
      return;
    }
    if (displayVersions.includes(ver)) {
      toast("info", `版本 ${ver} 已在列表中`);
      setAddModalOpen(false);
      return;
    }
    const nextList = [ver, ...displayVersions];
    await commitVersions(nextList, effectiveDefault || ver);
    toast("success", `已添加版本 ${ver}`);
    setAddModalOpen(false);
  }

  async function submitLocalUpload() {
    const ver = uploadVersion.trim();
    if (!ver) {
      toast("error", "请填写游戏版本号");
      return;
    }
    if (!uploadFile) {
      toast("error", "请选择要上传的服务端版本包文件");
      return;
    }
    setUploadPercent(0);
    const binding = await ensureServerBindingForUpload();
    const hex = await sha256Hex(uploadFile);
    const digest = `sha256:${hex}`;
    const archiveFormat = inferArchiveFormat(uploadFile.name);

    const initRes = await apiFetch("/apis/registry.gamepanel.io/v1/blobuploads", {
      method: "POST",
      body: JSON.stringify({
        apiVersion: "registry.gamepanel.io/v1",
        kind: "BlobUpload",
        digest,
        sizeBytes: uploadFile.size,
        mediaType: uploadFile.type || "application/octet-stream",
        fileName: uploadFile.name,
      }),
    });
    if (!initRes.ok) throw new Error(await errorMessage(initRes, "初始化版本包上传失败"));
    const session = (await initRes.json()) as RegistryBlobUpload;

    if (session.parts && session.parts.length > 0 && session.partSize) {
      const completedParts = [];
      for (const part of session.parts) {
        if (!part.url) continue;
        const start = (part.partNumber - 1) * session.partSize;
        const end = Math.min(uploadFile.size, start + session.partSize);
        const putRes = await putWithProgress(part.url, uploadFile.slice(start, end), (loaded) => {
          const totalLoaded = start + loaded;
          setUploadPercent(Math.min(99, Math.round((totalLoaded / Math.max(1, uploadFile.size)) * 100)));
        });
        completedParts.push({ partNumber: part.partNumber, etag: putRes.etag || "" });
      }
      const compRes = await apiFetch(
        `/apis/registry.gamepanel.io/v1/blobuploads/${encodeURIComponent(session.metadata.name)}`,
        {
          method: "PUT",
          body: JSON.stringify({ digest, parts: completedParts }),
        },
      );
      if (!compRes.ok) throw new Error(await errorMessage(compRes, "校验合并版本包失败"));
    } else if (session.presignedURL) {
      await putWithProgress(session.presignedURL, uploadFile, (loaded, total) => {
        setUploadPercent(Math.min(99, Math.round((loaded / Math.max(1, total)) * 100)));
      });
      const compRes = await apiFetch(
        `/apis/registry.gamepanel.io/v1/blobuploads/${encodeURIComponent(session.metadata.name)}`,
        {
          method: "PUT",
          body: JSON.stringify({ digest }),
        },
      );
      if (!compRes.ok) throw new Error(await errorMessage(compRes, "确认版本包上传失败"));
    }
    setUploadPercent(100);

    const descriptor = {
      digest,
      sizeBytes: uploadFile.size,
      mediaType: uploadFile.type || "application/octet-stream",
      archiveFormat,
      fileName: uploadFile.name,
    };

    const existingArt = artifactByVersion.get(ver);
    if (existingArt) {
      await apiFetch(
        `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(existingArt.metadata.name)}`,
        { method: "DELETE" },
      );
    }

    const createArtRes = await apiFetch("/apis/registry.gamepanel.io/v1/artifacts", {
      method: "POST",
      body: JSON.stringify({
        apiVersion: "registry.gamepanel.io/v1",
        kind: "Artifact",
        spec: {
          repository: binding.repository,
          package: binding.package,
          version: ver,
          archiveFormat,
          descriptor,
        },
      }),
    });
    if (!createArtRes.ok) throw new Error(await errorMessage(createArtRes, "保存服务端版本包失败"));

    const nextList = displayVersions.includes(ver) ? displayVersions : [ver, ...displayVersions];
    await commitVersions(nextList, effectiveDefault || ver, binding);
    toast("success", `版本 ${ver} 版本包已上传就绪`);
    setAddModalOpen(false);
    setUploadVersion("");
    setUploadFile(null);
    setUploadPercent(null);
    await loadData();
  }

  async function handleAddSubmit(e: FormEvent) {
    e.preventDefault();
    setSavingAdd(true);
    try {
      if (addMode === "official" && hasOfficialSource) {
        await submitOfficialVersion();
      } else {
        await submitLocalUpload();
      }
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "添加版本失败");
    } finally {
      setSavingAdd(false);
      setUploadPercent(null);
    }
  }

  async function confirmRemoveVersion() {
    if (!removingVersion) return;
    const target = removingVersion;
    setRemovingBusy(true);
    try {
      const art = artifactByVersion.get(target);
      const isLocalUpload = Boolean(art?.spec.descriptor && !art?.spec.sourceURL);
      if (isLocalUpload && art) {
        const delRes = await apiFetch(
          `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(art.metadata.name)}`,
          { method: "DELETE" },
        );
        if (!delRes.ok && delRes.status !== 404) {
          throw new Error(await errorMessage(delRes, "清理上传安装包失败"));
        }
      }
      const nextList = displayVersions.filter((v) => v !== target);
      const nextRec = target === effectiveDefault ? nextList[0] || "" : effectiveDefault;
      await commitVersions(nextList, nextRec);
      toast("success", `已移除版本 ${target}`);
      setRemovingVersion(null);
      await loadData();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "移除版本失败");
    } finally {
      setRemovingBusy(false);
    }
  }

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-3.5">
        <p className="text-[13px] text-[#5B6075]">
          管理向玩家开放的服务端版本，新建服务器时默认选中推荐版本（共 {displayVersions.length} 个版本）
        </p>
        <Button
          type="button"
          variant="primary"
          size="sm"
          icon={<Plus className="h-4 w-4" aria-hidden />}
          onClick={() => openAddVersionModal()}
        >
          添加版本
        </Button>
      </div>

      {displayVersions.length === 0 ? (
        <EmptyState
          image={null}
          title="尚未配置可选游戏版本"
          body="添加一个官方版本号或上传自定义服务端版本包"
          action={
            <Button
              type="button"
              variant="primary"
              icon={<Plus className="h-4 w-4" aria-hidden />}
              onClick={() => openAddVersionModal()}
            >
              添加版本
            </Button>
          }
        />
      ) : (
        <div
          tabIndex={0}
          role="region"
          aria-label="游戏版本列表"
          className="overflow-x-auto [scrollbar-gutter:stable]"
        >
          <table className="w-full min-w-[640px] table-fixed text-left text-sm">
            <colgroup>
              <col className="w-[34%]" />
              <col className="w-[24%]" />
              <col className="w-[22%]" />
              <col className="w-[20%]" />
            </colgroup>
            <thead>
              <tr className="border-b border-[#EEF0F7] bg-[#FAFBFD] text-[12px] font-semibold text-[#5B6075]">
                <th className="px-6 py-3">游戏版本</th>
                <th className="px-4 py-3">来源与大小</th>
                <th className="px-4 py-3">缓存状态</th>
                <th className="px-6 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#EEF0F7]">
              {displayVersions.map((ver) => {
                const isDefault = ver === effectiveDefault;
                const art = artifactByVersion.get(ver);
                const isLocalUpload =
                  serverRepo?.spec.mode === "Hosted" ||
                  Boolean(art?.spec.descriptor && !art?.spec.sourceURL);
                const sizeBytes =
                  art?.status?.descriptor?.sizeBytes ?? art?.spec.descriptor?.sizeBytes;
                const phase = art?.status?.phase;
                const syncing = phase === "Pending" || phase === "Syncing";
                const progress = art?.status?.progress ?? 0;
                const rowBusy = busyVersion === ver || loading;
                const badge: { tone: "success" | "accent" | "danger" | "neutral"; label: string } =
                  phase === "Ready"
                    ? { tone: "success", label: "已就绪" }
                    : syncing
                      ? { tone: "accent", label: "同步中" }
                      : phase === "Failed"
                        ? { tone: "danger", label: "同步失败" }
                        : { tone: "neutral", label: "按需拉取" };
                return (
                  <tr key={ver} className="transition-colors hover:bg-[#F7F8FC]/60">
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <Mono className="font-semibold text-[#14161F]">{ver}</Mono>
                        {isDefault && (
                          <span className="text-xs font-medium text-[#2B3AE8]">· 默认版本</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-4 text-[13px] text-[#5B6075]">
                      {isLocalUpload ? (
                        <span className="inline-flex items-center gap-1.5">
                          <Box className="h-3.5 w-3.5 text-[#8A90A6]" aria-hidden />
                          本地上传
                          {sizeBytes && sizeBytes > 0 ? ` · ${formatBytes(sizeBytes)}` : ""}
                        </span>
                      ) : (
                        <span>
                          官方发行版
                          {sizeBytes && sizeBytes > 0 ? ` · ${formatBytes(sizeBytes)}` : ""}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-4">
                      <div className="flex items-center gap-2.5">
                        <Badge tone={badge.tone}>{badge.label}</Badge>
                        {syncing && (
                          progress > 0 ? (
                            <progress max={100} value={progress} className="gp-progress w-24" />
                          ) : (
                            <progress max={100} className="gp-progress w-24" />
                          )
                        )}
                      </div>
                    </td>
                    <td className="px-6 py-4 text-right">
                      {(() => {
                        const overflowItems: MenuItem[] = [];
                        if (!isDefault) {
                          overflowItems.push({
                            label: "设为默认版本",
                            icon: <Star className="h-4 w-4" aria-hidden="true" />,
                            disabled: rowBusy,
                            onSelect: () => void handleSetDefault(ver),
                          });
                        }
                        if (art) {
                          overflowItems.push({
                            label: "查看日志",
                            icon: <Terminal className="h-4 w-4" aria-hidden="true" />,
                            onSelect: () => setLogArtifact(art),
                          });
                        }
                        overflowItems.push({
                          label: "移除版本",
                          icon: <Trash2 className="h-4 w-4" aria-hidden="true" />,
                          danger: true,
                          disabled: rowBusy || displayVersions.length <= 1,
                          onSelect: () => setRemovingVersion(ver),
                        });
                        return (
                          <div className="inline-flex items-center justify-end gap-1">
                            {hasOfficialSource && !isLocalUpload && (
                              <IconButton
                                label={`同步版本缓存 ${ver}`}
                                disabled={rowBusy || syncing}
                                onClick={() => void triggerSyncVersion(ver)}
                              >
                                <RefreshCw
                                  className={`h-4 w-4 ${syncing ? "animate-spin" : ""}`}
                                  aria-hidden
                                />
                              </IconButton>
                            )}
                            {isLocalUpload && (
                              <IconButton
                                label={`重新上传版本包 ${ver}`}
                                disabled={rowBusy}
                                onClick={() => openAddVersionModal(ver)}
                              >
                                <Upload className="h-4 w-4" aria-hidden />
                              </IconButton>
                            )}
                            <MoreMenu
                              label={`更多操作 (${ver})`}
                              className="h-8 w-8 bg-transparent"
                              items={overflowItems}
                            />
                          </div>
                        );
                      })()}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <Modal
        open={addModalOpen}
        onClose={() => !savingAdd && setAddModalOpen(false)}
        title="添加游戏版本"
        description="新增玩家创建服务器时可选的服务端版本，或上传本地服务端版本包"
        footer={
          <>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setAddModalOpen(false)}
              disabled={savingAdd}
            >
              取消
            </Button>
            <Button
              type="submit"
              form="add-game-version-form"
              loading={savingAdd}
              className="min-w-[116px]"
            >
              {addMode === "upload" ? "上传并添加" : "确认添加"}
            </Button>
          </>
        }
      >
        <form id="add-game-version-form" onSubmit={(e) => void handleAddSubmit(e)} className="space-y-4">
          {hasOfficialSource && (
            <div className="space-y-1.5">
              <div className="text-[13px] font-medium text-[#3D4258]">版本来源</div>
              <Segmented
                label="版本来源"
                value={addMode}
                onChange={(v) => setAddMode(v as AddVersionMode)}
                items={[
                  { value: "official", label: "官方发行版" },
                  { value: "upload", label: "本地上传版本包" },
                ]}
              />
            </div>
          )}

          {(() => {
            const isOfficialMode = hasOfficialSource && addMode === "official";
            const useCustomOfficial =
              availableOfficialChoices.length > 0 && officialInput === "__custom__";
            const uploadFields = (
              <div>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-5">
                  <div className="sm:col-span-2">
                    <Field label="版本号标识" htmlFor="ver-upload-name">
                      <Input
                        id="ver-upload-name"
                        name="uploadVersion"
                        value={uploadVersion}
                        onChange={(e) => setUploadVersion(e.target.value)}
                        disabled={isOfficialMode}
                        placeholder="如 1.21.4-custom"
                        autoCorrect="off"
                        autoCapitalize="none"
                        spellCheck={false}
                      />
                    </Field>
                  </div>
                  <div className="sm:col-span-3">
                    <Field label="服务端版本包" htmlFor="ver-upload-file">
                      <label
                        htmlFor="ver-upload-file"
                        className="flex h-10 cursor-pointer items-center justify-between gap-2 rounded-xl border border-dashed border-[#CFD5EA] bg-[#F7F8FC] px-3.5 text-sm text-[#3D4258] transition-colors hover:border-[#2B3AE8]"
                      >
                        <span className="truncate">
                          {uploadFile
                            ? `${uploadFile.name} (${formatBytes(uploadFile.size)})`
                            : "点击选择本地版本包"}
                        </span>
                        <Upload className="h-4 w-4 shrink-0 text-[#5B6075]" aria-hidden />
                        <input
                          id="ver-upload-file"
                          type="file"
                          accept=".zip,.tar.gz,.tgz,.jar,.tmod"
                          aria-describedby="ver-upload-file-hint"
                          disabled={isOfficialMode}
                          className="sr-only"
                          onChange={(e) => setUploadFile(e.target.files?.[0] || null)}
                        />
                      </label>
                    </Field>
                  </div>
                </div>
                <p id="ver-upload-file-hint" className="mt-1.5 text-[13px] text-[#8A90A6]">
                  支持 .zip、.tar.gz、.tgz、.jar 或 .tmod 格式
                </p>
                <div className="mt-2 flex h-5 items-center gap-3">
                  {savingAdd && uploadPercent !== null && (
                    <>
                      <progress max={100} value={uploadPercent} className="gp-progress flex-1" />
                      <span className="font-mono text-xs tabular-nums text-[#5B6075]">
                        {uploadPercent}%
                      </span>
                    </>
                  )}
                </div>
              </div>
            );

            if (!hasOfficialSource) {
              return <div className="min-h-[156px]">{uploadFields}</div>;
            }

            return (
              <div className="grid min-h-[156px] grid-cols-1">
                <div
                  role="tabpanel"
                  aria-label="官方发行版"
                  inert={!isOfficialMode}
                  aria-hidden={!isOfficialMode}
                  className={
                    isOfficialMode
                      ? "col-start-1 row-start-1"
                      : "invisible pointer-events-none col-start-1 row-start-1 select-none"
                  }
                >
                  <Field
                    label="选择官方版本号"
                    htmlFor={useCustomOfficial ? "ver-official-custom" : "ver-official-select"}
                  >
                    {availableOfficialChoices.length > 0 ? (
                      useCustomOfficial ? (
                        <div className="flex items-center gap-2">
                          <Input
                            id="ver-official-custom"
                            name="customOfficialVersion"
                            value={customOfficialInput}
                            onChange={(e) => setCustomOfficialInput(e.target.value)}
                            disabled={!isOfficialMode}
                            placeholder="输入指定版本号，如 1.21.4"
                            autoCorrect="off"
                            autoCapitalize="none"
                            spellCheck={false}
                            className="flex-1"
                          />
                          <Button
                            type="button"
                            variant="secondary"
                            disabled={!isOfficialMode}
                            onClick={() => setOfficialInput(availableOfficialChoices[0] || "")}
                          >
                            返回列表
                          </Button>
                        </div>
                      ) : (
                        <Select
                          id="ver-official-select"
                          name="officialVersion"
                          value={officialInput}
                          onChange={(e) => setOfficialInput(e.target.value)}
                          disabled={!isOfficialMode}
                        >
                          {availableOfficialChoices.map((v) => (
                            <option key={v} value={v}>
                              {v}
                            </option>
                          ))}
                          <option value="__custom__">手动输入其他版本号...</option>
                        </Select>
                      )
                    ) : (
                      <Input
                        id="ver-official-select"
                        name="officialVersion"
                        value={officialInput}
                        onChange={(e) => setOfficialInput(e.target.value)}
                        disabled={!isOfficialMode}
                        placeholder="例如 latest 或 1.21.4"
                        autoCorrect="off"
                        autoCapitalize="none"
                        spellCheck={false}
                      />
                    )}
                  </Field>
                </div>
                <div
                  role="tabpanel"
                  aria-label="本地上传版本包"
                  inert={isOfficialMode}
                  aria-hidden={isOfficialMode}
                  className={
                    !isOfficialMode
                      ? "col-start-1 row-start-1"
                      : "invisible pointer-events-none col-start-1 row-start-1 select-none"
                  }
                >
                  {uploadFields}
                </div>
              </div>
            );
          })()}
        </form>
      </Modal>

      <ArtifactLogModal
        open={Boolean(logArtifact)}
        onClose={() => setLogArtifact(null)}
        artifact={
          (logArtifact && artifactByVersion.get(logArtifact.spec.version)) || logArtifact
        }
        title={`${appDisplayName || appName} ${logArtifact?.spec.version || ""}`.trim()}
        token={token}
      />

      <ConfirmDialog
        open={Boolean(removingVersion)}
        onClose={() => !removingBusy && setRemovingVersion(null)}
        title="确认移除游戏版本"
        body={
          removingVersion ? (
            <>
              确定从可选列表移除版本 <Mono>{removingVersion}</Mono> 吗？已创建使用该版本的服务器不受影响。
            </>
          ) : undefined
        }
        confirmLabel="确认移除"
        danger
        loading={removingBusy}
        onConfirm={() => void confirmRemoveVersion()}
      />
    </div>
  );
}
