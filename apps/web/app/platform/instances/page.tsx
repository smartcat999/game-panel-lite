"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { ConsolePageHeader } from "@/components/console-page-header";
import { PlatformAccessGuard } from "@/components/platform-access-guard";
import { listGameServers, listOrganizations } from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";

export default function PlatformInstancesPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const instances = useQuery({ queryKey: ["platform", "instances"], queryFn: () => listGameServers(), retry: false });
  const organizations = useQuery({ queryKey: ["platform", "organizations"], queryFn: listOrganizations, retry: false });
  const organizationNames = new Map((organizations.data ?? []).map((organization) => [organization.id, organization.name]));

  return (
    <PlatformAccessGuard>
      <div className="space-y-3">
        <ConsolePageHeader title={isZh ? "平台实例" : "Platform instances"} />
        <section className="overflow-hidden rounded-xl border bg-white micro-border subtle-elevation">
          <div className="border-b border-slate-100 px-4 py-3">
            <h2 className="text-xs font-semibold text-slate-800">{isZh ? "逻辑实例与部署归属" : "Logical instances and placement"}</h2>
            <p className="mt-1 text-[11px] leading-4 text-slate-500">
              {isZh ? "这里用于平台排障和资源追踪。实例配置与日常操作仍属于租户控制台。" : "Use this view for platform diagnostics and resource tracing. Configuration and routine actions remain in the tenant console."}
            </p>
          </div>
          {instances.isLoading ? (
            <div className="p-8 text-center text-xs text-slate-400">{isZh ? "正在读取平台实例…" : "Loading platform instances…"}</div>
          ) : instances.isError ? (
            <div className="p-8 text-center text-xs text-slate-500">{isZh ? "平台实例暂时无法读取。" : "Platform instances are temporarily unavailable."}</div>
          ) : !instances.data?.length ? (
            <div className="p-8 text-center text-xs text-slate-500">{isZh ? "当前没有逻辑实例。" : "No logical instances are available."}</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[820px] text-left text-xs">
                <thead className="bg-slate-50 text-[10px] uppercase tracking-wider text-slate-400">
                  <tr>
                    <th className="px-4 py-2.5 font-medium">{isZh ? "实例" : "Instance"}</th>
                    <th className="px-3 py-2.5 font-medium">{isZh ? "租户" : "Tenant"}</th>
                    <th className="px-3 py-2.5 font-medium">{isZh ? "期望状态" : "Desired state"}</th>
                    <th className="px-3 py-2.5 font-medium">{isZh ? "观测状态" : "Observed state"}</th>
                    <th className="px-3 py-2.5 font-medium">{isZh ? "部署区域" : "Region"}</th>
                    <th className="px-3 py-2.5 font-medium">{isZh ? "配置版本" : "Revision"}</th>
                    <th className="px-4 py-2.5 text-right font-medium">{isZh ? "详情" : "Details"}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {instances.data.map((instance) => (
                    <tr key={instance.id} className="text-slate-600">
                      <td className="px-4 py-3">
                        <p className="font-semibold text-slate-800">{instance.name}</p>
                        <p className="mt-0.5 font-mono text-[10px] text-slate-400">{instance.id}</p>
                      </td>
                      <td className="px-3 py-3">
                        <p>{organizationNames.get(instance.organizationId ?? "") ?? (isZh ? "未知租户" : "Unknown tenant")}</p>
                        <p className="mt-0.5 font-mono text-[10px] text-slate-400">{instance.organizationId ?? "—"}</p>
                      </td>
                      <td className="px-3 py-3">{desiredStateLabel(instance.spec.desiredState, isZh)}</td>
                      <td className="px-3 py-3">{statusLabel(gameServerStatus(instance), isZh)}</td>
                      <td className="px-3 py-3 font-mono text-[11px]">{instance.region || (isZh ? "未分配" : "Unplaced")}</td>
                      <td className="px-3 py-3 font-mono">{instance.spec.generation}</td>
                      <td className="px-4 py-3 text-right">
                        <Link href={`/platform/instances/${encodeURIComponent(instance.id)}`} className="inline-flex items-center gap-1 font-medium text-emerald-700 hover:text-emerald-800">
                          {isZh ? "查看链路" : "Trace resource"}<ArrowRight className="size-3" />
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </PlatformAccessGuard>
  );
}

function desiredStateLabel(state: string, isZh: boolean) {
  if (state === "running") return isZh ? "运行" : "Running";
  if (state === "deleted") return isZh ? "删除" : "Deleted";
  return isZh ? "停止" : "Stopped";
}

function statusLabel(status: string, isZh: boolean) {
  const labels: Record<string, [string, string]> = {
    running: ["运行中", "Running"],
    stopped: ["已停止", "Stopped"],
    starting: ["启动中", "Starting"],
    stopping: ["停止中", "Stopping"],
    deleting: ["删除中", "Deleting"],
    errored: ["异常", "Error"]
  };
  const label = labels[status] ?? ["未知", "Unknown"];
  return isZh ? label[0] : label[1];
}
