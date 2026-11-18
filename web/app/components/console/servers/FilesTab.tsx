"use client";

import { ChangeEvent, DragEvent, FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronRight, Download, File, FilePlus, FileText, Folder, FolderPlus, Pencil, RefreshCw, Search, Trash2, Upload } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatBytes, relativeTime } from "../../../console/lib/format";
import type { FileEntry } from "../../../console/lib/types";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Field,
  focusRing,
  IconButton,
  Input,
  Modal,
  Mono,
  Skeleton,
  Textarea,
} from "../ui";
import { useServerFiles } from "./api";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");
const join = (dir: string, name: string) => (dir === "." ? name : `${dir}/${name}`);
const errText = (err: unknown, fallback: string) => (err instanceof Error && err.message ? err.message : fallback);
const BINARY_EXT_RE =
  /\.(jar|zip|tar|gz|tgz|7z|rar|png|jpe?g|gif|webp|ico|pdf|so|dll|exe|bin|dat|db|sqlite|tmod|pak|pck)$/i;
const MAX_EDIT_BYTES = 2 * 1024 * 1024;

interface Editing {
  path: string;
  content: string;
  original: string;
}

export function FilesTab({ name }: { name: string }) {
  const { toast } = useConsole();
  const files = useServerFiles(name);
  const { list } = files;
  const uploadInputRef = useRef<HTMLInputElement>(null);

  const [dir, setDir] = useState(".");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");

  const [editing, setEditing] = useState<Editing | null>(null);
  const [opening, setOpening] = useState("");
  const [saving, setSaving] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);

  const [folderOpen, setFolderOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [creating, setCreating] = useState(false);

  const [newFileOpen, setNewFileOpen] = useState(false);
  const [newFileName, setNewFileName] = useState("");
  const [creatingFile, setCreatingFile] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<{ name: string; pct: number } | null>(null);
  const [downloadingPath, setDownloadingPath] = useState("");
  const [isDragging, setIsDragging] = useState(false);
  const dragDepthRef = useRef(0);

  const [toRename, setToRename] = useState<{ path: string; name: string; isDir?: boolean } | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renaming, setRenaming] = useState(false);

  const [toDelete, setToDelete] = useState<{ path: string; name: string; isDir?: boolean } | null>(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(
    async (target: string) => {
      setLoading(true);
      setLoadError("");
      try {
        const items = await list(target);
        items.sort((a, b) => Number(!!b.isDir) - Number(!!a.isDir) || a.name.localeCompare(b.name));
        setEntries(items);
        setDir(target);
        setSearchQuery("");
      } catch (err) {
        setLoadError(`${errText(err, "读取目录失败")}。若服务器刚创建，请等待启动完成后重试。`);
      } finally {
        setLoading(false);
      }
    },
    [list],
  );

  useEffect(() => {
    void load(".");
  }, [load]);

  const segments = dir === "." ? [] : dir.split("/").filter((s) => s && s !== ".");
  const pathOf = (e: FileEntry) => e.path || join(dir, e.name);
  const dirty = !!editing && editing.content !== editing.original;
  const filteredEntries = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter((e) => e.name.toLowerCase().includes(q));
  }, [entries, searchQuery]);

  async function openFile(path: string, size?: number) {
    if (BINARY_EXT_RE.test(path) || (typeof size === "number" && size > MAX_EDIT_BYTES)) {
      toast("error", "该文件为二进制或超过 2 MB，请下载到本地查看");
      return;
    }
    setOpening(path);
    try {
      const text = await files.read(path);
      setEditing({ path, content: text, original: text });
    } catch (err) {
      toast("error", `${errText(err, "读取文件失败")}，请稍后重试`);
    } finally {
      setOpening("");
    }
  }

  async function save() {
    if (!editing) return;
    setSaving(true);
    try {
      await files.write(editing.path, editing.content);
      setEditing({ ...editing, original: editing.content });
      toast("success", `已保存 ${editing.path}`);
      await load(dir);
    } catch (err) {
      toast("error", `${errText(err, "保存失败")}，请稍后重试`);
    } finally {
      setSaving(false);
    }
  }

  async function createFolder(e: FormEvent) {
    e.preventDefault();
    const n = folderName.trim();
    if (!n) return;
    setCreating(true);
    try {
      await files.mkdir(join(dir, n));
      toast("success", `已新建文件夹 ${n}`);
      setFolderName("");
      setFolderOpen(false);
      await load(dir);
    } catch (err) {
      toast("error", `${errText(err, "新建文件夹失败")}，请检查名称后重试`);
    } finally {
      setCreating(false);
    }
  }

  async function createFile(e: FormEvent) {
    e.preventDefault();
    const n = newFileName.trim();
    if (!n) return;
    const targetPath = join(dir, n);
    setCreatingFile(true);
    try {
      await files.write(targetPath, "");
      toast("success", `已新建文件 ${n}`);
      setNewFileName("");
      setNewFileOpen(false);
      await load(dir);
      setEditing({ path: targetPath, content: "", original: "" });
    } catch (err) {
      toast("error", `${errText(err, "新建文件失败")}，请检查名称后重试`);
    } finally {
      setCreatingFile(false);
    }
  }

  async function uploadFiles(selected: File[]) {
    if (!selected.length) return;
    setUploading(true);
    try {
      for (const file of selected) {
        setUploadProgress({ name: file.name, pct: 0 });
        await files.upload(join(dir, file.name), file, (pct) => {
          setUploadProgress({ name: file.name, pct });
        });
      }
      toast("success", selected.length === 1 ? `已上传 ${selected[0].name}` : `已上传 ${selected.length} 个文件`);
      await load(dir);
    } catch (err) {
      toast("error", `${errText(err, "上传文件失败")}，请稍后重试`);
    } finally {
      setUploadProgress(null);
      setUploading(false);
    }
  }

  async function handleUploadChange(e: ChangeEvent<HTMLInputElement>) {
    const selected = Array.from(e.target.files || []);
    e.target.value = "";
    await uploadFiles(selected);
  }

  function handleDragEnter(e: DragEvent<HTMLDivElement>) {
    if (!e.dataTransfer?.types?.includes("Files")) return;
    e.preventDefault();
    e.stopPropagation();
    dragDepthRef.current += 1;
    setIsDragging(true);
  }

  function handleDragOver(e: DragEvent<HTMLDivElement>) {
    if (!e.dataTransfer?.types?.includes("Files")) return;
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = "copy";
    if (!isDragging) setIsDragging(true);
  }

  function handleDragLeave(e: DragEvent<HTMLDivElement>) {
    if (!e.dataTransfer?.types?.includes("Files")) return;
    e.preventDefault();
    e.stopPropagation();
    dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
    if (dragDepthRef.current === 0) {
      setIsDragging(false);
    }
  }

  async function handleDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    e.stopPropagation();
    dragDepthRef.current = 0;
    setIsDragging(false);
    const dropped = Array.from(e.dataTransfer?.files || []);
    if (dropped.length > 0) {
      await uploadFiles(dropped);
    }
  }

  function openRenameModal(entry: { path: string; name: string; isDir?: boolean }) {
    setToRename(entry);
    setRenameValue(entry.name);
  }

  async function submitRename(e: FormEvent) {
    e.preventDefault();
    if (!toRename) return;
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === toRename.name) {
      setToRename(null);
      return;
    }
    if (trimmed.includes("/") || trimmed.includes("\\")) {
      toast("error", "名称不能包含路径分隔符");
      return;
    }
    const parentDir = toRename.path.includes("/")
      ? toRename.path.slice(0, toRename.path.lastIndexOf("/")) || "/"
      : dir;
    const newPath = parentDir === "." ? trimmed : `${parentDir.replace(/\/$/, "")}/${trimmed}`;
    setRenaming(true);
    try {
      await files.rename(toRename.path, newPath);
      if (editing?.path === toRename.path) {
        setEditing({ ...editing, path: newPath });
      }
      toast("success", `已重命名为 ${trimmed}`);
      setToRename(null);
      await load(dir);
    } catch (err) {
      toast("error", `${errText(err, "重命名失败")}，请稍后重试`);
    } finally {
      setRenaming(false);
    }
  }

  async function handleDownload(path: string, filename: string) {
    setDownloadingPath(path);
    try {
      await files.download(path, filename);
    } catch (err) {
      toast("error", `${errText(err, "下载文件失败")}，请稍后重试`);
    } finally {
      setDownloadingPath("");
    }
  }

  async function confirmDelete() {
    if (!toDelete) return;
    setDeleting(true);
    try {
      await files.rm(toDelete.path);
      if (editing?.path === toDelete.path) setEditing(null);
      toast("success", `已删除 ${toDelete.name}`);
      setToDelete(null);
      await load(dir);
    } catch (err) {
      toast("error", `${errText(err, "删除失败")}，请稍后重试`);
    } finally {
      setDeleting(false);
    }
  }

  const closeEditor = () => (dirty ? setDiscardOpen(true) : setEditing(null));

  return (
    <div>
      <Card className="min-w-0 overflow-hidden">
        <div
          className="relative"
          onDragEnter={handleDragEnter}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={(e) => void handleDrop(e)}
        >
        {isDragging && (
          <div className="pointer-events-none absolute inset-2 z-20 flex flex-col items-center justify-center rounded-2xl border-2 border-dashed border-[#2B3AE8] bg-[#EEF1FF]/90 px-6 py-8 text-center backdrop-blur-[2px]">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[#2B3AE8] text-white shadow-sm">
              <Upload className="h-5 w-5" />
            </div>
            <p className="mt-3 text-sm font-semibold text-[#14161F]">
              松开鼠标上传至 <Mono>{dir === "." ? "根目录" : dir}</Mono>
            </p>
            <p className="mt-1 text-xs text-[#5B6075]">支持拖拽单个或多个文件快速上传</p>
          </div>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-3.5">
          <nav aria-label="当前路径" className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
            <button
              type="button"
              onClick={() => void load(".")}
              className={cx(
                "rounded-full px-2 py-0.5 font-medium",
                segments.length ? "text-[#2B3AE8] hover:bg-[#EEF1FF]" : "text-[#14161F]",
                focusRing,
              )}
            >
              根目录
            </button>
            {segments.map((seg, i) => {
              const target = segments.slice(0, i + 1).join("/");
              const last = i === segments.length - 1;
              return (
                <span key={target} className="inline-flex items-center gap-1">
                  <ChevronRight className="h-3.5 w-3.5 text-[#8A90A6]" aria-hidden />
                  <button
                    type="button"
                    disabled={last}
                    onClick={() => void load(target)}
                    className={cx(
                      "rounded-full px-2 py-0.5 font-mono text-[13px]",
                      last ? "text-[#14161F]" : "text-[#2B3AE8] hover:bg-[#EEF1FF]",
                      focusRing,
                    )}
                  >
                    {seg}
                  </button>
                </span>
              );
            })}
          </nav>
          <div className="flex flex-wrap items-center gap-2">
            {entries.length > 0 && (
              <div className="relative w-48">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]" />
                <Input
                  placeholder="过滤文件或目录..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="h-9 pl-8 text-[13px]"
                />
              </div>
            )}
            <input
              ref={uploadInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => void handleUploadChange(e)}
            />
            <Button
              size="sm"
              icon={<Upload className="h-3.5 w-3.5" />}
              loading={uploading}
              onClick={() => uploadInputRef.current?.click()}
            >
              上传文件
            </Button>
            <Button size="sm" icon={<FilePlus className="h-3.5 w-3.5" />} onClick={() => setNewFileOpen(true)}>
              新建文件
            </Button>
            <Button size="sm" icon={<FolderPlus className="h-3.5 w-3.5" />} onClick={() => setFolderOpen(true)}>
              新建文件夹
            </Button>
            <IconButton label="刷新" onClick={() => void load(dir)} disabled={loading}>
              <RefreshCw className={cx("h-4 w-4", loading && "animate-spin")} />
            </IconButton>
          </div>
        </div>

        {uploadProgress && (
          <div className="border-b border-[#EEF0F7] bg-[#F7F8FC] px-6 py-2.5">
            <div className="flex items-center justify-between gap-2 text-xs text-[#5B6075]">
              <span className="truncate">正在上传：{uploadProgress.name}</span>
              <span className="shrink-0 tabular-nums">{uploadProgress.pct}%</span>
            </div>
            <progress className="gp-progress mt-1.5 h-1.5 w-full" value={uploadProgress.pct} max={100} />
          </div>
        )}

        {loading && entries.length === 0 ? (
          <div className="space-y-2.5 px-6 py-5">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : loadError ? (
          <EmptyState
            image={null}
            title="无法读取目录"
            body={loadError}
            action={<Button onClick={() => void load(dir)}>重试</Button>}
          />
        ) : entries.length === 0 ? (
          <EmptyState image={null} title="当前目录为空" body="支持将本地文件直接拖拽到此区域上传，或点击上方按钮新建文件与文件夹。" />
        ) : filteredEntries.length === 0 ? (
          <div className="px-6 py-12 text-center text-sm text-[#8A90A6]">没有匹配的文件或文件夹</div>
        ) : (
          <div>
            <div className="hidden border-b border-[#EEF0F7] bg-[#FAFBFD] px-6 py-2.5 text-[12px] font-medium text-[#8A90A6] sm:flex sm:items-center sm:gap-3">
              <span className="min-w-0 flex-1">名称</span>
              <span className="w-24 shrink-0 text-right">大小</span>
              <span className="hidden w-28 shrink-0 text-right md:block">修改时间</span>
              <span className="w-[104px] shrink-0 text-right">操作</span>
            </div>
            <ul className="divide-y divide-[#EEF0F7]">
              {filteredEntries.map((e) => {
                const p = pathOf(e);
                const active = editing?.path === p;
                return (
                  <li
                    key={p}
                    className={cx(
                      "group flex items-center gap-3 px-6 py-3.5 transition-colors hover:bg-[#FAFBFD]",
                      active && "bg-[#F5F7FF]",
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => void (e.isDir ? load(p) : openFile(p, e.size))}
                      disabled={opening === p}
                      className={cx(
                        "flex min-w-0 flex-1 items-center gap-3 rounded-lg py-0.5 text-left",
                        focusRing,
                      )}
                    >
                      {e.isDir ? (
                        <Folder className="h-4 w-4 shrink-0 text-[#2B3AE8]" aria-hidden />
                      ) : /\.(txt|log|json|ya?ml|properties|cfg|conf|ini|toml|md|sh)$/i.test(e.name) ? (
                        <FileText className="h-4 w-4 shrink-0 text-[#5B6075]" aria-hidden />
                      ) : (
                        <File className="h-4 w-4 shrink-0 text-[#8A90A6]" aria-hidden />
                      )}
                      <Mono className="min-w-0 flex-1 truncate text-[#14161F]">{e.name}</Mono>
                      <span className="hidden w-24 shrink-0 text-right text-[13px] tabular-nums text-[#8A90A6] sm:block">
                        {e.isDir ? "—" : formatBytes(e.size)}
                      </span>
                      <span className="hidden w-28 shrink-0 text-right text-[13px] text-[#8A90A6] md:block">
                        {relativeTime(e.modTime)}
                      </span>
                    </button>
                    <div className="flex w-[104px] shrink-0 items-center justify-end gap-1">
                      <IconButton
                        label={`重命名 ${e.name}`}
                        onClick={() => openRenameModal({ path: p, name: e.name, isDir: e.isDir })}
                        className="bg-transparent opacity-100 hover:bg-[#EEF1FF] hover:text-[#2B3AE8] sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
                      >
                        <Pencil className="h-4 w-4" />
                      </IconButton>
                      {!e.isDir && (
                        <IconButton
                          label={`下载 ${e.name}`}
                          disabled={downloadingPath === p}
                          onClick={() => void handleDownload(p, e.name)}
                          className="bg-transparent opacity-100 hover:bg-[#EEF1FF] hover:text-[#2B3AE8] sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
                        >
                          <Download className="h-4 w-4" />
                        </IconButton>
                      )}
                      <IconButton
                        label={`删除 ${e.name}`}
                        onClick={() => setToDelete({ path: p, name: e.name, isDir: e.isDir })}
                        className="bg-transparent opacity-100 hover:bg-[#FEF3F2] hover:text-[#D92D20] sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
                      >
                        <Trash2 className="h-4 w-4" />
                      </IconButton>
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>
        )}
        </div>
      </Card>

      <Modal
        open={!!editing}
        onClose={closeEditor}
        width="max-w-3xl"
        title={
          <span className="inline-flex items-center gap-2">
            <Mono className="text-sm text-[#14161F]">{editing?.path || "编辑文件"}</Mono>
            {dirty && <Badge tone="warning">未保存</Badge>}
          </span>
        }
        footer={
          <>
            <Button variant="ghost" disabled={saving} onClick={closeEditor}>
              取消
            </Button>
            <Button
              variant="primary"
              loading={saving}
              disabled={!dirty}
              onClick={() => void save()}
            >
              保存
            </Button>
          </>
        }
      >
        {editing && (
          <Textarea
            aria-label={`编辑 ${editing.path}`}
            value={editing.content}
            spellCheck={false}
            onChange={(ev) => setEditing({ ...editing, content: ev.target.value })}
            onKeyDown={(ev) => {
              if ((ev.metaKey || ev.ctrlKey) && ev.key === "s") {
                ev.preventDefault();
                if (dirty && !saving) void save();
              }
            }}
            className="h-[440px] resize-y font-mono text-[12.5px] leading-relaxed"
          />
        )}
      </Modal>

      <Modal
        open={newFileOpen}
        onClose={() => setNewFileOpen(false)}
        title="新建文件"
        description={
          <>
            将创建在 <Mono>{dir === "." ? "根目录" : dir}</Mono> 下并打开在线编辑器。
          </>
        }
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewFileOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="new-file-form"
              variant="primary"
              loading={creatingFile}
              disabled={!newFileName.trim()}
            >
              新建并编辑
            </Button>
          </>
        }
      >
        <form id="new-file-form" onSubmit={createFile}>
          <Field label="文件名称" htmlFor="new-file-name">
            <Input
              id="new-file-name"
              value={newFileName}
              onChange={(e) => setNewFileName(e.target.value)}
              placeholder="例如 server.properties"
              autoComplete="off"
              autoFocus
            />
          </Field>
        </form>
      </Modal>

      <Modal
        open={folderOpen}
        onClose={() => setFolderOpen(false)}
        title="新建文件夹"
        description={
          <>
            将创建在 <Mono>{dir === "." ? "根目录" : dir}</Mono> 下。
          </>
        }
        footer={
          <>
            <Button variant="ghost" onClick={() => setFolderOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="new-folder-form"
              variant="primary"
              loading={creating}
              disabled={!folderName.trim()}
            >
              新建
            </Button>
          </>
        }
      >
        <form id="new-folder-form" onSubmit={createFolder}>
          <Field label="文件夹名称" htmlFor="new-folder-name">
            <Input
              id="new-folder-name"
              value={folderName}
              onChange={(e) => setFolderName(e.target.value)}
              placeholder="例如 plugins"
              autoComplete="off"
              autoFocus
            />
          </Field>
        </form>
      </Modal>

      <Modal
        open={!!toRename}
        onClose={() => {
          if (!renaming) setToRename(null);
        }}
        title={toRename?.isDir ? "重命名文件夹" : "重命名文件"}
        description={
          <>
            原名称：<Mono>{toRename?.name}</Mono>
          </>
        }
        footer={
          <>
            <Button variant="ghost" disabled={renaming} onClick={() => setToRename(null)}>
              取消
            </Button>
            <Button
              type="submit"
              form="rename-file-form"
              variant="primary"
              loading={renaming}
              disabled={!renameValue.trim() || renameValue.trim() === toRename?.name}
            >
              确认重命名
            </Button>
          </>
        }
      >
        <form id="rename-file-form" onSubmit={(e) => void submitRename(e)}>
          <Field label="新名称" htmlFor="rename-file-name">
            <Input
              id="rename-file-name"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              placeholder="输入新名称"
              autoComplete="off"
              autoFocus
            />
          </Field>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!toDelete}
        title={`删除${toDelete?.isDir ? "文件夹" : "文件"} ${toDelete?.name ?? ""}？`}
        body={toDelete?.isDir ? "文件夹里的所有内容都会被删除，且无法恢复。" : "删除后无法恢复。"}
        confirmLabel="删除"
        danger
        loading={deleting}
        onConfirm={() => void confirmDelete()}
        onClose={() => setToDelete(null)}
      />

      <ConfirmDialog
        open={discardOpen}
        title="放弃未保存的修改？"
        body="关闭后未保存的修改将丢失。"
        confirmLabel="放弃修改"
        danger
        onConfirm={() => {
          setDiscardOpen(false);
          setEditing(null);
        }}
        onClose={() => setDiscardOpen(false)}
      />
    </div>
  );
}
