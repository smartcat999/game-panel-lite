"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Activity, Archive, HardDrive, Plus, Users } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Card } from "@/components/ui";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { formatBytes, listActivity, listBackups, listServers } from "@/lib/api";
import { attachLatestBackupTimes } from "@/lib/server-metrics";

export default function DashboardPage() {
  const { locale, t } = useI18n();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const activityQuery = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const backups = backupsQuery.data ?? [];
  const servers = attachLatestBackupTimes(serversQuery.data ?? [], backups);
  const activity = activityQuery.data ?? [];
  const running = servers.filter((server) => server.status === "running");
  const players = servers.reduce((sum, server) => sum + server.players, 0);
  const playerCapacity = servers.reduce((sum, server) => sum + server.maxPlayers, 0);
  const totalBackupBytes = backups.reduce((sum, backup) => sum + backup.sizeBytes, 0);
  const latestBackup = backups[0];
  return (
    <AppShell>
      <PageHeader title={t("dashboardTitle")} description={t("dashboardDescription")} />
      {(serversQuery.isError || backupsQuery.isError || activityQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiDataUnavailable")}</p>}
      <div className="grid gap-4 md:grid-cols-4">
        <Stat icon={<HardDrive />} label={t("runningServers")} value={`${running.length} / ${servers.length}`} hint={t("runningHint", { count: running.length })} />
        <Stat icon={<Users />} label={t("onlinePlayers")} value={`${players} / ${playerCapacity}`} hint={t("playersOnlineHint", { count: players })} />
        <Stat icon={<Archive />} label={t("latestBackup")} value={latestBackup ? localizeRelativeTime(latestBackup.created, locale) : t("none")} hint={latestBackup?.world ?? t("none")} />
        <Stat icon={<HardDrive />} label={t("storageUsed")} value={formatBytes(totalBackupBytes)} hint={t("storageHint", { count: backups.length })} />
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
              {activity.slice(0, 5).map((event) => (
                <div key={event.id} className="flex items-start gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
                  <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded bg-panel-green/15 text-panel-green">
                    <Activity aria-hidden="true" className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm text-slate-100">{event.message}</p>
                    <p className="mt-0.5 text-xs text-slate-500">{localizeRelativeTime(event.created, locale)}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
        <Card className="p-4">
          <h2 className="font-semibold">{t("quickActions")}</h2>
          <div className="mt-4 flex flex-col gap-3">
            <Link href="/servers/new"><Button className="w-full"><Plus aria-hidden="true" />{t("createServer")}</Button></Link>
            <Link href="/worlds"><Button variant="secondary" className="w-full">{t("importWorld")}</Button></Link>
            <Link href="/backups"><Button variant="gold" className="w-full">{t("createBackup")}</Button></Link>
          </div>
        </Card>
      </div>
    </AppShell>
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
