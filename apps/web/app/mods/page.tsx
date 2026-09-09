"use client";

import { useQuery } from "@tanstack/react-query";
import { Download, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listGlobalMods } from "@/lib/api";
import { Button } from "@/components/ui";
import type { ModFile } from "@/lib/types";

export default function ModsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  const modsQuery = useQuery({
    queryKey: ["global-mods"],
    queryFn: listGlobalMods,
    retry: false
  });

  const mods = (modsQuery.data ?? []) as ModFile[];

  return (
    <div className="space-y-3.5">
      {/* Direct Header */}
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-base font-bold text-slate-900 tracking-tight">
          {isZh ? "模组工坊" : "Mod Workshop"}
        </h1>

        <Button className="h-8 text-xs bg-emerald-600 hover:bg-emerald-700 text-white">
          <Plus className="size-3.5" />
          <span>{isZh ? "上传模组 (.tmod)" : "Upload (.tmod)"}</span>
        </Button>
      </div>

      <div className="bg-white border border-slate-200/80 rounded-xl shadow-2xs overflow-hidden">
        {mods.length === 0 ? (
          <div className="p-12 text-center text-xs text-slate-400">
            {isZh ? "暂无已安装模组，点击右上角上传或从创意工坊导入" : "No mods installed. Click upload above to install mods."}
          </div>
        ) : (
          <table className="w-full text-left text-xs border-collapse">
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
        )}
      </div>
    </div>
  );
}
