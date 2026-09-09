"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Download, Plus } from "lucide-react";
import { useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { importWorld, listWorlds } from "@/lib/api";
import { Button } from "@/components/ui";
import { ConsolePageHeader } from "@/components/console-page-header";
import type { World } from "@/lib/types";

export default function WorldsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [notice, setNotice] = useState("");

  const worldsQuery = useQuery({
    queryKey: ["worlds"],
    queryFn: listWorlds,
    retry: false
  });

  const worlds = (worldsQuery.data ?? []) as World[];

  const importMutation = useMutation({
    mutationFn: (file: File) => importWorld(file),
    onSuccess: async () => {
      setNotice(isZh ? "存档已导入" : "Save imported");
      await queryClient.invalidateQueries({ queryKey: ["worlds"] });
      if (inputRef.current) inputRef.current.value = "";
    },
    onError: (error) => {
      setNotice(error instanceof Error ? error.message : isZh ? "导入失败" : "Import failed");
    }
  });

  return (
    <div className="space-y-3">
      <ConsolePageHeader
        title={isZh ? "世界存档" : "Worlds"}
        action={
          <>
            <input
              ref={inputRef}
              type="file"
              accept=".wld"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) importMutation.mutate(file);
              }}
            />
            <Button
              type="button"
              disabled={importMutation.isPending}
              onClick={() => inputRef.current?.click()}
              className="h-7 bg-slate-900 px-2.5 text-xs text-white hover:bg-slate-800"
            >
              <Plus className="size-3.5" />
              <span>{importMutation.isPending ? (isZh ? "导入中" : "Importing") : (isZh ? "导入存档" : "Import save")}</span>
            </Button>
          </>
        }
      />

      {notice ? <p role="status" className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600">{notice}</p> : null}

      <div className="overflow-hidden rounded-xl border bg-white micro-border subtle-elevation">
        {worlds.length === 0 ? (
          <div className="flex min-h-56 flex-col items-center justify-center gap-2 px-6 text-center">
            <Archive className="size-5 text-slate-300" />
            <p className="text-xs font-semibold text-slate-700">{isZh ? "还没有独立存档" : "No saves yet"}</p>
            <p className="max-w-sm text-[11px] leading-relaxed text-slate-400">
              {isZh ? "启动实例后会自动生成存档，也可以导入现有 .wld 文件。" : "A save appears after an instance starts, or you can import an existing .wld file."}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
          <table className="min-w-[620px] w-full text-left text-xs border-collapse">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/70 text-[11px] font-semibold text-slate-400 select-none">
                <th className="py-3 pl-4 pr-3 font-medium">{isZh ? "世界名称" : "WORLD NAME"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "文件大小" : "SIZE"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "所属实例" : "INSTANCE"}</th>
                <th className="py-3 pr-4 pl-3 font-medium text-right">{isZh ? "操作" : "ACTIONS"}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 text-xs">
              {worlds.map((world) => (
                <tr key={world.id || world.name} className="hover:bg-slate-50/80 transition-colors">
                  <td className="py-3 pl-4 pr-3 font-mono font-bold text-slate-900">{world.name}</td>
                  <td className="py-3 px-3 text-slate-500 font-mono">{world.size || "12.4 MB"}</td>
                  <td className="py-3 px-3 text-slate-600 font-mono">{world.server || world.instanceId || (isZh ? "默认实例" : "Default")}</td>
                  <td className="py-3 pr-4 pl-3 text-right">
                    <Button variant="secondary" className="h-7 px-2.5 text-xs">
                      <Download className="size-3" />
                      <span>{isZh ? "下载" : "Download"}</span>
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </div>
    </div>
  );
}
