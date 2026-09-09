"use client";

import { useQuery } from "@tanstack/react-query";
import { Download, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listWorlds } from "@/lib/api";
import { Button } from "@/components/ui";
import type { World } from "@/lib/types";

export default function WorldsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  const worldsQuery = useQuery({
    queryKey: ["worlds"],
    queryFn: listWorlds,
    retry: false
  });

  const worlds = (worldsQuery.data ?? []) as World[];

  return (
    <div className="space-y-3.5">
      {/* Direct Header */}
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-base font-bold text-slate-900 tracking-tight">
          {isZh ? "世界存档" : "Worlds"}
        </h1>

        <Button className="h-8 text-xs bg-emerald-600 hover:bg-emerald-700 text-white">
          <Plus className="size-3.5" />
          <span>{isZh ? "导入存档 (.wld)" : "Import Save (.wld)"}</span>
        </Button>
      </div>

      <div className="bg-white border border-slate-200/80 rounded-xl shadow-2xs overflow-hidden">
        {worlds.length === 0 ? (
          <div className="p-12 text-center text-xs text-slate-400">
            {isZh ? "暂无独立世界存档，启动实例后将自动生成" : "No separate world saves found. Launching an instance will generate one."}
          </div>
        ) : (
          <table className="w-full text-left text-xs border-collapse">
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
                  <td className="py-3 px-3 text-slate-600 font-mono">{world.server || world.instanceId || "Default"}</td>
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
        )}
      </div>
    </div>
  );
}
