"use client";

import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, HelpCircle, UploadCloud, ChevronDown, ChevronUp, FolderArchive } from "lucide-react";
import { Button } from "@/components/ui";
import { useToast } from "@/components/toast-context";
import { useI18n } from "@/lib/i18n";
import { importWorld } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

interface WorldMigrationHubProps {
  servers?: GameServerResource[];
  targetServerId?: string;
  onImportSuccess?: () => void;
}

export function WorldMigrationHub({
  servers = [],
  targetServerId,
  onImportSuccess
}: WorldMigrationHubProps) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();
  const client = useQueryClient();

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selectedServerId, setSelectedServerId] = useState<string>(targetServerId || "unassigned");
  const [copiedKey, setCopiedKey] = useState<string>("");
  const [showPathGuide, setShowPathGuide] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  // Local Save Path Guides
  const pathGuides = [
    {
      key: "terraria",
      name: isZh ? "泰拉瑞亚 (Terraria / tModLoader)" : "Terraria / tModLoader",
      ext: ".wld / .twld",
      winPath: "%USERPROFILE%\\Documents\\My Games\\Terraria\\Worlds",
      macPath: "~/Library/Application Support/Terraria/Worlds",
      desc: isZh ? "直接上传单机创建的地图 .wld 文件" : "Upload your local .wld file"
    },
    {
      key: "palworld",
      name: isZh ? "幻兽帕鲁 (Palworld)" : "Palworld",
      ext: "SaveGames .zip",
      winPath: "%LOCALAPPDATA%\\Pal\\Saved\\SaveGames",
      macPath: "N/A (Windows Only)",
      desc: isZh ? "将数字编号存档文件夹打包为 .zip 上传" : "Zip your numeric save folder and upload"
    },
    {
      key: "minecraft",
      name: isZh ? "我的世界 (Minecraft Java)" : "Minecraft Java",
      ext: "world.zip",
      winPath: "%APPDATA%\\.minecraft\\saves",
      macPath: "~/Library/Application Support/minecraft/saves",
      desc: isZh ? "将包含 level.dat 的世界文件夹压缩为 .zip 上传" : "Zip world directory containing level.dat"
    },
    {
      key: "dst",
      name: isZh ? "饥荒联机版 (DST)" : "Don't Starve Together",
      ext: "Cluster.zip",
      winPath: "%USERPROFILE%\\Documents\\Klei\\DoNotStarveTogether",
      macPath: "~/Documents/Klei/DoNotStarveTogether",
      desc: isZh ? "将 Cluster 存档文件夹打包为 .zip 上传" : "Zip Cluster folder and upload"
    }
  ];

  const copyPath = (key: string, path: string) => {
    navigator.clipboard.writeText(path);
    setCopiedKey(key);
    toast.success(isZh ? "路径已复制" : "Path Copied", path);
    setTimeout(() => setCopiedKey(""), 2500);
  };

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      return await importWorld(file, selectedServerId);
    },
    onSuccess: async (data) => {
      toast.success(
        isZh ? "世界存档导入成功" : "World Imported",
        isZh ? `世界【${data.name}】已导入` : `World ${data.name} is ready`
      );
      if (fileInputRef.current) fileInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["worlds"] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
      onImportSuccess?.();
    },
    onError: (err) => {
      toast.error(isZh ? "导入失败" : "Import Failed", err instanceof Error ? err.message : "");
    }
  });

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      uploadMutation.mutate(file);
    }
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files?.[0];
    if (file) {
      uploadMutation.mutate(file);
    }
  };

  return (
    <div className="space-y-3">
      {/* Main Professional Import Bar */}
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        className={cn(
          "relative overflow-hidden rounded-xl border p-4 sm:p-5 transition shadow-lg",
          dragOver
            ? "border-panel-green bg-panel-green/10 ring-2 ring-panel-green/40"
            : "border-slate-800 bg-gradient-to-b from-slate-900/90 via-slate-900/60 to-slate-950/90"
        )}
      >
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
          <div className="space-y-1.5 min-w-0">
            <div className="flex items-center gap-2">
              <FolderArchive className="size-4 text-panel-green" />
              <h3 className="text-sm font-bold text-white tracking-tight">
                {isZh ? "导入世界存档" : "Import World Save"}
              </h3>
            </div>
            <p className="text-xs text-slate-400">
              {isZh
                ? "支持直接拖入 .wld 地图或 .zip 存档包，自动解析并挂载"
                : "Drag & drop .wld or .zip saves to import into servers."}
            </p>

            {/* Server target selector */}
            {servers.length > 0 && (
              <div className="pt-1 flex flex-wrap items-center gap-2">
                <span className="text-xs text-slate-400">{isZh ? "关联服务器:" : "Assign To:"}</span>
                <select
                  value={selectedServerId}
                  onChange={(e) => setSelectedServerId(e.target.value)}
                  className="rounded-lg border border-slate-700 bg-slate-950 px-2.5 py-1 text-xs text-slate-200 focus:border-panel-green focus:outline-none"
                >
                  <option value="unassigned">{isZh ? "暂不关联服务器 (存入世界库)" : "Save to World Library"}</option>
                  {servers.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.gameKey?.toUpperCase() || "GAME"})
                    </option>
                  ))}
                </select>
              </div>
            )}
          </div>

          {/* Concise Action Buttons */}
          <div className="flex items-center gap-2.5 shrink-0">
            <input
              ref={fileInputRef}
              type="file"
              accept=".wld,.twld,.zip,.tar.gz"
              onChange={handleFileChange}
              className="hidden"
            />

            <Button
              type="button"
              disabled={uploadMutation.isPending}
              onClick={() => fileInputRef.current?.click()}
              className="gap-1.5 px-4 h-9 bg-panel-green hover:bg-emerald-600 text-black font-bold text-xs shadow-md shadow-panel-green/10"
            >
              <UploadCloud className="size-3.5" />
              <span>
                {uploadMutation.isPending
                  ? isZh ? "正在上传..." : "Uploading..."
                  : isZh ? "上传存档" : "Upload Save"}
              </span>
            </Button>

            <button
              type="button"
              onClick={() => setShowPathGuide(!showPathGuide)}
              className="inline-flex items-center justify-center gap-1.5 rounded-lg border border-slate-700 bg-slate-900/80 px-3 h-9 text-xs font-medium text-slate-300 hover:bg-slate-800 transition"
            >
              <HelpCircle className="size-3.5 text-slate-400" />
              <span>{isZh ? "本地路径指南" : "Path Guide"}</span>
              {showPathGuide ? <ChevronUp className="size-3 text-slate-500" /> : <ChevronDown className="size-3 text-slate-500" />}
            </button>
          </div>
        </div>
      </div>

      {/* Path Guides Accordion Drawer */}
      {showPathGuide && (
        <div className="rounded-xl border border-slate-800 bg-slate-950 p-4 space-y-3">
          <div className="flex items-center justify-between">
            <h4 className="text-xs font-bold text-slate-300">
              {isZh ? "各游戏电脑本地单机存档默认路径" : "Default Local Save Paths"}
            </h4>
            <span className="text-[11px] text-slate-500">{isZh ? "点击一键复制路径" : "Click to copy"}</span>
          </div>

          <div className="grid gap-2.5 sm:grid-cols-2">
            {pathGuides.map((guide) => (
              <div
                key={guide.key}
                className="rounded-lg border border-slate-800 bg-slate-900/60 p-3 space-y-2 text-xs"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-bold text-white">{guide.name}</span>
                  <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] font-mono text-slate-400">
                    {guide.ext}
                  </span>
                </div>
                <p className="text-[11px] text-slate-400 leading-snug">{guide.desc}</p>
                <div className="flex items-center gap-1.5 pt-1">
                  <div className="min-w-0 flex-1 rounded bg-slate-950 px-2 py-1 font-mono text-[10px] text-slate-300 truncate">
                    {guide.winPath}
                  </div>
                  <button
                    type="button"
                    onClick={() => copyPath(guide.key, guide.winPath)}
                    className="inline-flex items-center gap-1 rounded border border-slate-700 bg-slate-800 px-2 py-1 text-[10px] font-medium text-slate-200 hover:bg-slate-700 transition shrink-0"
                  >
                    {copiedKey === guide.key ? (
                      <>
                        <Check className="size-3 text-panel-green" />
                        <span className="text-panel-green">{isZh ? "已复制" : "Copied"}</span>
                      </>
                    ) : (
                      <>
                        <Copy className="size-3 text-slate-400" />
                        <span>{isZh ? "复制" : "Copy"}</span>
                      </>
                    )}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
