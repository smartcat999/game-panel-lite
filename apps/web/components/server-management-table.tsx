"use client";

import { useI18n } from "@/lib/i18n";
import type { TenantInstanceView } from "@/lib/types";

export function ServerManagementTable({ instances }: { instances: TenantInstanceView[] }) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  if (instances.length === 0) {
    return <div className="p-10 text-center text-xs text-slate-400">{isZh ? "当前空间还没有实例" : "No instances in this workspace yet."}</div>;
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[800px] text-left text-xs">
        <thead>
          <tr className="border-b border-slate-100 bg-slate-50/70 text-[10px] font-medium uppercase tracking-wider text-slate-400">
            <th className="py-2.5 pl-4 pr-3 font-medium">{isZh ? "实例" : "Instance"}</th>
            <th className="px-3 py-2.5 font-medium">{isZh ? "服务状态" : "Service state"}</th>
            <th className="px-3 py-2.5 font-medium">{isZh ? "运行环境" : "Runtime"}</th>
            <th className="px-3 py-2.5 font-medium">{isZh ? "区域" : "Region"}</th>
            <th className="px-3 py-2.5 font-medium">{isZh ? "规格" : "Resources"}</th>
            <th className="px-3 py-2.5 font-medium">{isZh ? "交付状态" : "Delivery"}</th>
            <th className="py-2.5 pl-3 pr-4 text-right font-medium">{isZh ? "配置版本" : "Revision"}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100">
          {instances.map((instance) => {
            const state = serviceState(instance, isZh);
            return (
              <tr key={instance.id} className="text-slate-600 transition-colors hover:bg-slate-50/70">
                <td className="py-3 pl-4 pr-3">
                  <p className="font-semibold text-slate-900">{instance.name}</p>
                  <p className="mt-0.5 font-mono text-[10px] text-slate-400">{instance.id}</p>
                </td>
                <td className="px-3 py-3">
                  <span className={`inline-flex items-center gap-1.5 font-medium ${state.tone}`}>
                    <span className={`size-1.5 rounded-full ${state.dot}`} />
                    {state.label}
                  </span>
                </td>
                <td className="px-3 py-3">
                  <p className="font-medium text-slate-700">{providerName(instance.providerKey)}</p>
                  <p className="mt-0.5 font-mono text-[10px] text-slate-400">{instance.gameVersion}</p>
                </td>
                <td className="px-3 py-3 font-mono text-[11px] text-slate-600">{instance.regionId}</td>
                <td className="px-3 py-3 font-mono text-[11px] text-slate-600">{instance.cpu} vCPU · {formatMemory(instance.memoryMb)}</td>
                <td className="px-3 py-3">{deliveryLabel(instance, isZh)}</td>
                <td className="py-3 pl-3 pr-4 text-right font-mono text-[11px] text-slate-500">v{instance.specGeneration}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function serviceState(instance: TenantInstanceView, isZh: boolean) {
  const actual = instance.deployment?.actualState;
  if (actual === "running") return { label: isZh ? "运行中" : "Running", tone: "text-emerald-700", dot: "bg-emerald-500" };
  if (actual === "stopped") return { label: isZh ? "已停止" : "Stopped", tone: "text-slate-500", dot: "bg-slate-400" };
  if (actual === "missing") return { label: isZh ? "运行资源缺失" : "Runtime missing", tone: "text-rose-700", dot: "bg-rose-500" };
  if (instance.desiredState === "deleted") return { label: isZh ? "已删除" : "Deleted", tone: "text-slate-400", dot: "bg-slate-300" };
  return { label: isZh ? "等待激活" : "Awaiting activation", tone: "text-amber-700", dot: "bg-amber-500" };
}

function deliveryLabel(instance: TenantInstanceView, isZh: boolean) {
  if (instance.deployment?.outcome === "failed") return isZh ? "交付失败" : "Delivery failed";
  if (instance.deployment?.outcome === "succeeded") return isZh ? "已完成" : "Completed";
  if (instance.latestOperation?.status === "failed") return isZh ? "请求失败" : "Request failed";
  if (instance.latestOperation?.status === "succeeded") return isZh ? "已接收" : "Accepted";
  return isZh ? "等待权益与区域执行" : "Waiting for entitlement and Region";
}

function providerName(providerKey: string) {
  if (providerKey === "terraria-tmodloader") return "tModLoader";
  if (providerKey === "terraria-vanilla") return "Terraria Vanilla";
  return providerKey;
}

function formatMemory(memoryMb: number) {
  return memoryMb >= 1024 && memoryMb % 1024 === 0 ? `${memoryMb / 1024} GB` : `${memoryMb} MB`;
}
