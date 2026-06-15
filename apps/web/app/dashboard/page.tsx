"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Activity, Archive, Cpu, HardDrive, MemoryStick, Plus, Server, Users } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Card } from "@/components/ui";
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

      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <RuntimeCard
          icon={<Server aria-hidden="true" className="size-4" />}
          label={t("runningContainers")}
          value={String(runtimeStatsQuery.data?.runningContainers ?? 0)}
          color="#7bd978"
        />
        <RuntimeCard
          icon={<Cpu aria-hidden="true" className="size-4" />}
          label={t("runtimeCpu")}
          value={`${(runtimeStatsQuery.data?.totalCpuPercent ?? 0).toFixed(1)}%`}
          sparkline={<Sparkline data={runtimeCpuSeries} color="#7bd978" max={400} />}
          color="#7bd978"
        />
        <RuntimeCard
          icon={<MemoryStick aria-hidden="true" className="size-4" />}
          label={t("runtimeMemory")}
          value={runtimeStatsQuery.data ? `${runtimeStatsQuery.data.totalMemoryMb} MB` : "—"}
          sparkline={<Sparkline data={runtimeMemSeries} color="#a78bfa" max={memMax} />}
          color="#a78bfa"
        />
      </div>
      <section className="mt-6">
        <h2 className="mb-3 text-base font-semibold">{t("activeServers")}</h2>
        <div className="grid gap-3">
          {running.map((server) => <ServerCard key={server.id} server={server} />)}
        </div>
        {running.length === 0 && <p className="text-sm text-slate-400">{t("noRunningServers")}</p>}
      </section>
      <div className="mt-6 grid gap-4 lg:grid-cols-[1fr_360px]">
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
        <Card className="p-4">
          <h2 className="font-semibold">{t("quickActions")}</h2>
          <div className="mt-4 flex flex-col gap-3">
            <Link href={dashboardQuickActionHrefs.createServer}><Button className="w-full"><Plus aria-hidden="true" />{t("createServer")}</Button></Link>
            <Link href={dashboardQuickActionHrefs.importWorld}><Button variant="secondary" className="w-full">{t("importWorld")}</Button></Link>
            <Link href={dashboardQuickActionHrefs.createBackup}><Button variant="gold" className="w-full">{t("createBackup")}</Button></Link>
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

function RuntimeCard({ icon, label, value, sparkline, color = "#7bd978" }: { icon: React.ReactNode; label: string; value: string; sparkline?: React.ReactNode; color?: string }) {
  return (
    <div className="rounded-lg border border-panel-line bg-panel-card px-4 py-3">
      <div className="flex items-center gap-2">
        <span className="shrink-0" style={{ color }}>{icon}</span>
        <p className="text-xs text-slate-500">{label}</p>
      </div>
      <div className="mt-2 flex items-end justify-between gap-2">
        <p className="font-mono text-xl font-semibold text-white">{value}</p>
        {sparkline && <div className="shrink-0">{sparkline}</div>}
      </div>
    </div>
  );
}
