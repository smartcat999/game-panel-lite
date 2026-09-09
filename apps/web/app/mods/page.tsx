"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, PackageOpen, Plus } from "lucide-react";
import { useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { listGlobalMods, uploadGlobalMod } from "@/lib/api";
import { Button } from "@/components/ui";
import { ConsolePageHeader } from "@/components/console-page-header";
import type { ModFile } from "@/lib/types";

export default function ModsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [notice, setNotice] = useState("");

  const modsQuery = useQuery({
    queryKey: ["global-mods"],
    queryFn: listGlobalMods,
    retry: false
  });

  const mods = (modsQuery.data ?? []) as ModFile[];

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadGlobalMod(file),
    onSuccess: async () => {
      setNotice(isZh ? "模组已上传" : "Mod uploaded");
      await queryClient.invalidateQueries({ queryKey: ["global-mods"] });
      if (inputRef.current) inputRef.current.value = "";
    },
    onError: (error) => {
      setNotice(error instanceof Error ? error.message : isZh ? "上传失败" : "Upload failed");
    }
  });

  return (
    <div className="space-y-3">
      <ConsolePageHeader
        title={isZh ? "模组工坊" : "Mod Workshop"}
        action={
          <>
            <input
              ref={inputRef}
              type="file"
              accept=".tmod"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) uploadMutation.mutate(file);
              }}
            />
            <Button
              type="button"
              disabled={uploadMutation.isPending}
              onClick={() => inputRef.current?.click()}
              className="h-7 bg-slate-900 px-2.5 text-xs text-white hover:bg-slate-800"
            >
              <Plus className="size-3.5" />
              <span>{uploadMutation.isPending ? (isZh ? "上传中" : "Uploading") : (isZh ? "上传模组" : "Upload mod")}</span>
            </Button>
          </>
        }
      />

      {notice ? <p role="status" className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600">{notice}</p> : null}

      <div className="overflow-hidden rounded-xl border bg-white micro-border subtle-elevation">
        {mods.length === 0 ? (
          <div className="flex min-h-56 flex-col items-center justify-center gap-2 px-6 text-center">
            <PackageOpen className="size-5 text-slate-300" />
            <p className="text-xs font-semibold text-slate-700">{isZh ? "还没有模组" : "No mods yet"}</p>
            <p className="max-w-sm text-[11px] leading-relaxed text-slate-400">
              {isZh ? "上传经过验证的 .tmod 文件后，可在 tModLoader 实例中使用。" : "Upload a validated .tmod file to make it available to tModLoader instances."}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
          <table className="min-w-[620px] w-full text-left text-xs border-collapse">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/70 text-[11px] font-semibold text-slate-400 select-none">
                <th className="py-3 pl-4 pr-3 font-medium">{isZh ? "模组名称" : "MOD NAME"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "版本" : "VERSION"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "文件大小" : "SIZE"}</th>
                <th className="py-3 pr-4 pl-3 font-medium text-right">{isZh ? "操作" : "ACTIONS"}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 text-xs">
              {mods.map((mod) => {
                const displayName = mod.title || mod.modName || mod.fileName || mod.id;
                return (
                  <tr key={mod.id} className="hover:bg-slate-50/80 transition-colors">
                    <td className="py-3 pl-4 pr-3 font-mono font-bold text-slate-900">{displayName}</td>
                    <td className="py-3 px-3 text-purple-700 font-mono text-[11px] font-semibold">{mod.modVersion || "1.0.0"}</td>
                    <td className="py-3 px-3 text-slate-500 font-mono">{mod.size || "4.8 MB"}</td>
                    <td className="py-3 pr-4 pl-3 text-right">
                      <Button variant="secondary" className="h-7 px-2.5 text-xs">
                        <Download className="size-3" />
                        <span>{isZh ? "下载" : "Download"}</span>
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          </div>
        )}
      </div>
    </div>
  );
}
