"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Building2, MapPinned, Server } from "lucide-react";
import { ConsolePageHeader } from "@/components/console-page-header";
import { PlatformAccessGuard } from "@/components/platform-access-guard";
import { getPlatformInstanceView, listOrganizations } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function PlatformInstanceDetailPage() {
  const params = useParams<{ id: string }>();
  const id = decodeURIComponent(params.id);
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const instance = useQuery({ queryKey: ["platform", "instances", id], queryFn: () => getPlatformInstanceView(id), enabled: Boolean(id), retry: false, refetchInterval: 10_000 });
  const organizations = useQuery({ queryKey: ["platform", "organizations"], queryFn: listOrganizations, retry: false });
  const resource = instance.data;
  const organization = organizations.data?.find((item) => item.id === resource?.organizationId);

  return (
    <PlatformAccessGuard>
      <div className="space-y-3">
        <ConsolePageHeader
          title={resource?.name ?? (isZh ? "平台实例详情" : "Platform instance details")}
          action={<Link href="/platform/instances" className="inline-flex items-center gap-1.5 text-[11px] font-medium text-slate-500 hover:text-slate-800"><ArrowLeft className="size-3" />{isZh ? "返回实例列表" : "Back to instances"}</Link>}
        />

        {instance.isLoading ? (
          <div className="rounded-xl border bg-white p-8 text-center text-xs text-slate-400 micro-border">{isZh ? "正在读取实例关联信息…" : "Loading instance relationships…"}</div>
        ) : instance.isError || !resource ? (
          <div className="rounded-xl border bg-white p-8 text-center text-xs text-slate-500 micro-border">{isZh ? "实例不存在或暂时无法读取。" : "The instance does not exist or is temporarily unavailable."}</div>
        ) : (
          <>
            <section className="rounded-xl border bg-white p-4 micro-border subtle-elevation">
              <div className="flex items-start gap-3">
                <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500"><Server className="size-4" /></div>
                <div className="min-w-0">
                  <h2 className="text-sm font-semibold text-slate-900">{resource.name}</h2>
                  <p className="mt-1 font-mono text-[10px] text-slate-400">{resource.id}</p>
                  <p className="mt-2 max-w-2xl text-[11px] leading-5 text-slate-500">
                    {isZh ? "该页面用于追踪全局逻辑实例到区域部署的关系，不提供租户侧配置、启停或存档操作。" : "This page traces a global logical instance to its regional deployment. Tenant configuration, lifecycle, and save operations are handled in the tenant console."}
                  </p>
                </div>
              </div>
            </section>

            <div className="grid gap-3 lg:grid-cols-2">
              <RelationCard icon={<Building2 className="size-4" />} title={isZh ? "租户归属" : "Tenant ownership"}>
                <Definition label={isZh ? "租户" : "Tenant"} value={organization?.name ?? (isZh ? "未知租户" : "Unknown tenant")} />
                <Definition label={isZh ? "租户标识" : "Tenant ID"} value={resource.organizationId ?? "—"} mono />
              </RelationCard>

              <RelationCard icon={<MapPinned className="size-4" />} title={isZh ? "部署位置" : "Placement"}>
                <Definition label={isZh ? "区域" : "Region"} value={resource.regionId} mono />
                <Definition label={isZh ? "实际节点" : "Allocated node"} value={resource.nodeId || (isZh ? "尚未调度" : "Not scheduled")} mono />
                <Link href={`/platform/regions/${encodeURIComponent(resource.regionId)}`} className="mt-3 inline-flex text-[11px] font-medium text-emerald-700 hover:text-emerald-800">{isZh ? "查看区域运维" : "Open Region operations"}</Link>
              </RelationCard>
            </div>

            <section className="rounded-xl border bg-white p-4 micro-border subtle-elevation">
              <h2 className="text-xs font-semibold text-slate-800">{isZh ? "控制面状态" : "Control-plane state"}</h2>
              <div className="mt-3 grid gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
                <Definition label={isZh ? "期望状态" : "Desired state"} value={desiredStateLabel(resource.desiredState, isZh)} />
                <Definition label={isZh ? "观测状态" : "Observed state"} value={statusLabel(resource.deployment?.actualState, resource.latestOperation?.status, isZh)} />
                <Definition label={isZh ? "配置版本" : "Spec generation"} value={String(resource.specGeneration)} mono />
                <Definition label={isZh ? "意图版本" : "Intent version"} value={String(resource.intentVersion)} mono />
                <Definition label={isZh ? "游戏版本" : "Game version"} value={resource.gameVersion} mono />
                <Definition label={isZh ? "运行提供器" : "Provider"} value={resource.providerKey} mono />
                <Definition label="CPU" value={`${resource.cpu}`} mono />
                <Definition label={isZh ? "内存" : "Memory"} value={`${resource.memoryMb} MB`} mono />
              </div>
            </section>
          </>
        )}
      </div>
    </PlatformAccessGuard>
  );
}

function desiredStateLabel(state: string, isZh: boolean) {
  if (state === "running") return isZh ? "运行" : "Running";
  if (state === "deleted") return isZh ? "删除" : "Deleted";
  return isZh ? "停止" : "Stopped";
}

function statusLabel(actualState: string | undefined, operationStatus: string | undefined, isZh: boolean) {
  if (!actualState && operationStatus === "pending") return isZh ? "交付中" : "Provisioning";
  if (!actualState) return isZh ? "尚未上报" : "Not reported";
  const labels: Record<string, [string, string]> = {
    running: ["运行中", "Running"],
    stopped: ["已停止", "Stopped"],
    starting: ["启动中", "Starting"],
    stopping: ["停止中", "Stopping"],
    deleting: ["删除中", "Deleting"],
    errored: ["异常", "Error"]
  };
  const label = labels[actualState] ?? ["未知", "Unknown"];
  return isZh ? label[0] : label[1];
}

function RelationCard({ icon, title, children }: { icon: React.ReactNode; title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border bg-white p-4 micro-border subtle-elevation">
      <div className="flex items-center gap-2 text-slate-500">{icon}<h2 className="text-xs font-semibold text-slate-800">{title}</h2></div>
      <div className="mt-4 space-y-3">{children}</div>
    </section>
  );
}

function Definition({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="text-[10px] uppercase tracking-wider text-slate-400">{label}</p>
      <p className={`mt-1 text-xs text-slate-700 ${mono ? "font-mono" : "font-medium"}`}>{value}</p>
    </div>
  );
}
