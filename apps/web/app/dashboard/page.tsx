"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Activity, Archive, ArrowRight, Cpu, HardDrive, MemoryStick, Plus, Users } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Card } from "@/components/ui";
import { formatActivityEvent } from "@/lib/activity-display";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { formatBytes, getRuntimeStats, listActivity, listBackups, listServers } from "@/lib/api";
import { dashboardQuickActionHrefs } from "@/lib/dashboard-quick-actions";
import { attachLatestBackupTimes } from "@/lib/server-metrics";
import { Sparkline, useTimeSeries } from "@/lib/sparkline";

export default function DashboardPage() {
  const { locale, t } = useI18n();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const activityQuery = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const runtimeStatsQuery = useQuery({ queryKey: ["runtime-stats"], queryFn: getRuntimeStats, refetchInterval: 5000, retry: false });
  const runtimeCpuSeries = useTimeSeries(runtimeStatsQuery.data?.totalCpuPercent);
  const runtimeMemSeries = useTimeSeries(runtimeStatsQuery.data?.totalMemoryMb, 60);
  const backups = backupsQuery.data ?? [];
  const servers = attachLatestBackupTimes(serversQuery.data ?? [], backups);
  const activity = activityQuery.data ?? [];
  const running = servers.filter((server) => server.status === "running");
  const players = servers.reduce((sum, server) => sum + server.players, 0);
  const playerCapacity = servers.reduce((sum, server) => sum + server.maxPlayers, 0);
  const totalBackupBytes = backups.reduce((sum, backup) => sum + backup.sizeBytes, 0);
  const latestBackup = backups[0];
  const memMax = Math.max(1024, runtimeStatsQuery.data?.memoryLimitMb ?? 1024);
  const runtimeCpu = runtimeStatsQuery.data?.totalCpuPercent ?? 0;
  const runtimeMemory = runtimeStatsQuery.data?.totalMemoryMb ?? 0;
  const runtimeMemoryLimit = runtimeStatsQuery.data?.memoryLimitMb ?? memMax;
  const memoryPercent = runtimeMemoryLimit > 0 ? runtimeMemory / runtimeMemoryLimit * 100 : 0;
  const runtimeMemoryLimitLabel = runtimeStatsQuery.data ? `${runtimeMemoryLimit} MB` : "";
  return (
    <>
      <PageHeader title={t("dashboardTitle")} description={t("dashboardDescription")} />
      {(serversQuery.isError || backupsQuery.isError || activityQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiDataUnavailable")}</p>}
      <div className="grid gap-4 md:grid-cols-4">
        <Stat icon={<HardDrive />} label={t("runningServers")} value={`${running.length} / ${servers.length}`} hint={t("runningHint", { count: running.length })} />
        <Stat icon={<Users />} label={t("onlinePlayers")} value={`${players} / ${playerCapacity}`} hint={t("playersOnlineHint", { count: players })} />
        <Stat icon={<Archive />} label={t("latestBackup")} value={latestBackup ? localizeRelativeTime(latestBackup.created, locale) : t("none")} hint={latestBackup?.world ?? t("none")} />
        <Stat icon={<HardDrive />} label={t("storageUsed")} value={formatBytes(totalBackupBytes)} hint={t("storageHint", { count: backups.length })} />
      </div>

      <div className="mt-6 grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="min-w-0 space-y-5">
          <section>
            <div className="mb-3 flex items-center justify-between gap-3">
              <h2 className="text-base font-semibold">{t("activeServers")}</h2>
              <span className="rounded-md border border-panel-line bg-slate-950/50 px-2 py-1 text-xs text-slate-500">
                {running.length} / {servers.length}
              </span>
            </div>
            <div className="grid gap-3">
              {running.map((server) => <ServerCard key={server.id} server={server} />)}
            </div>
            {running.length === 0 && (
              <div className="rounded-lg border border-panel-line bg-panel-card px-4 py-5 text-sm text-slate-400">
                {t("noRunningServers")}
              </div>
            )}
          </section>

          <Card className="p-4">
            <h2 className="font-semibold">{t("recentActivity")}</h2>
            {activity.length === 0 ? (
              <p className="mt-3 text-sm text-slate-400">{activityQuery.isLoading ? t("loading") : t("noActivityYet")}</p>
            ) : (
              <div className="mt-3 space-y-2">
                {activity.slice(0, 5).map((event) => {
                  const display = formatActivityEvent(event, locale);
                  return (
                    <div key={event.id} className="flex items-start gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
                      <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded bg-panel-green/15 text-panel-green">
                        <Activity aria-hidden="true" className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <p className="truncate text-sm text-slate-100">{display.message}</p>
                        <p className="mt-0.5 text-xs text-slate-500">{display.typeLabel} · {localizeRelativeTime(event.created, locale)}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </Card>
        </div>

        <Card className="overflow-hidden">
          <div className="border-b border-panel-line px-4 py-4">
            <h2 className="font-semibold">{t("quickActions")}</h2>
            <p className="mt-1 text-xs text-slate-500">{t("quickActionsHint")}</p>
          </div>
          <div className="space-y-2 p-3">
            <ActionLink
              href={dashboardQuickActionHrefs.createServer}
              icon={<Plus aria-hidden="true" className="size-4" />}
              label={t("createServer")}
              hint={t("createServerQuickHint")}
              tone="primary"
            />
            <ActionLink
              href={dashboardQuickActionHrefs.createBackup}
              icon={<Archive aria-hidden="true" className="size-4" />}
              label={t("createBackup")}
              hint={t("createBackupQuickHint")}
              tone="gold"
            />
          </div>
          <div className="border-t border-panel-line px-4 py-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-slate-100">{t("runtimeOverview")}</h3>
                <p className="mt-1 text-xs text-slate-500">{t("runtimeOverviewHint")}</p>
              </div>
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-panel-green/30 bg-panel-green/10 text-panel-green">
                <Activity aria-hidden="true" className="size-4" />
              </span>
            </div>
            <div className="space-y-3">
              <RuntimeSignal
                icon={<Cpu aria-hidden="true" className="size-4" />}
                label={t("cpu")}
                value={`${runtimeCpu.toFixed(1)}%`}
                subValue={t("cpuUsageHint")}
                percent={Math.min(runtimeCpu / 400 * 100, 100)}
                sparkline={<Sparkline data={runtimeCpuSeries} width={92} height={26} color="#7bd978" max={400} />}
              />
              <RuntimeSignal
                icon={<MemoryStick aria-hidden="true" className="size-4" />}
                label={t("memory")}
                value={runtimeStatsQuery.data ? `${runtimeMemory} MB` : "—"}
                subValue={runtimeMemoryLimitLabel ? t("memoryLimitHint", { limit: runtimeMemoryLimitLabel }) : ""}
                percent={Math.min(memoryPercent, 100)}
                sparkline={<Sparkline data={runtimeMemSeries} width={92} height={26} color="#a78bfa" max={memMax} />}
                tone="purple"
              />
            </div>
          </div>
        </Card>
      </div>
    </>
  );
}

function Stat({ icon, label, value, hint }: { icon: React.ReactNode; label: string; value: string; hint: string }) {
  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <span className="flex size-11 items-center justify-center rounded-md bg-panel-green/15 text-panel-green">{icon}</span>
        <div>
          <p className="text-sm text-slate-400">{label}</p>
          <p className="mt-1 text-2xl font-semibold">{value}</p>
          <p className="text-xs text-slate-500">{hint}</p>
        </div>
      </div>
    </Card>
  );
}

function ActionLink({
  href,
  icon,
  label,
  hint,
  tone = "neutral"
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
  hint: string;
  tone?: "primary" | "neutral" | "gold";
}) {
  return (
    <Link
      href={href}
      className={[
        "group flex items-center gap-3 rounded-md border px-3 py-3 transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
        tone === "primary" ? "border-panel-green/30 bg-panel-green/10 hover:bg-panel-green/15" : "",
        tone === "gold" ? "border-panel-gold/25 bg-panel-gold/10 hover:bg-panel-gold/15" : "",
        tone === "neutral" ? "border-panel-line bg-slate-950/35 hover:bg-slate-900/80" : ""
      ].join(" ")}
    >
      <span
        className={[
          "flex size-9 shrink-0 items-center justify-center rounded-md border",
          tone === "primary" ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "",
          tone === "gold" ? "border-panel-gold/30 bg-panel-gold/10 text-panel-gold" : "",
          tone === "neutral" ? "border-panel-line bg-slate-900 text-slate-300" : ""
        ].join(" ")}
      >
        {icon}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-slate-100">{label}</span>
        <span className="mt-0.5 block truncate text-xs text-slate-500">{hint}</span>
      </span>
      <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500 transition group-hover:translate-x-0.5 group-hover:text-slate-200" />
    </Link>
  );
}

function RuntimeSignal({
  icon,
  label,
  value,
  subValue,
  percent,
  sparkline,
  tone = "green"
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  subValue?: string;
  percent?: number;
  sparkline?: React.ReactNode;
  tone?: "green" | "purple";
}) {
  const colorClass = tone === "purple" ? "text-panel-purple" : "text-panel-green";
  const barClass = tone === "purple" ? "bg-panel-purple" : "bg-panel-green";

  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className={colorClass}>{icon}</span>
            <p className="text-xs text-slate-500">{label}</p>
          </div>
          <div className="mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <p className="font-mono text-lg font-semibold text-slate-100">{value}</p>
            {subValue && <p className="text-xs text-slate-500">{subValue}</p>}
          </div>
        </div>
        {sparkline && <div className="shrink-0 pt-1">{sparkline}</div>}
      </div>
      {percent !== undefined && (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-800">
          <div className={`h-full rounded-full ${barClass}`} style={{ width: `${Math.max(4, Math.min(percent, 100))}%` }} />
        </div>
      )}
    </div>
  );
}
