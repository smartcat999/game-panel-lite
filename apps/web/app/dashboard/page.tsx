"use client";

import Link from "next/link";
import { Archive, HardDrive, Plus, Users } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Card } from "@/components/ui";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { activity, servers } from "@/lib/mock-data";

export default function DashboardPage() {
  const { locale, t } = useI18n();
  const running = servers.filter((server) => server.status === "running");
  const players = servers.reduce((sum, server) => sum + server.players, 0);
  const activityMessages = [t("activityBackupJourney"), t("activityPlayerJoined"), t("activityClassicStarted"), t("activityBackupClassic")];
  return (
    <AppShell>
      <PageHeader title={t("dashboardTitle")} description={t("dashboardDescription")} />
      <div className="grid gap-4 md:grid-cols-4">
        <Stat icon={<HardDrive />} label={t("runningServers")} value={`${running.length} / ${servers.length}`} hint={t("runningHint", { count: running.length })} />
        <Stat icon={<Users />} label={t("onlinePlayers")} value={`${players} / 32`} hint={t("playersOnlineHint", { count: players })} />
        <Stat icon={<Archive />} label={t("latestBackup")} value={localizeRelativeTime("12 min ago", locale)} hint="Journey Friends" />
        <Stat icon={<HardDrive />} label={t("storageUsed")} value="3.8 GB" hint={t("storageHint")} />
      </div>
      <section className="mt-6">
        <h2 className="mb-3 text-base font-semibold">{t("activeServers")}</h2>
        <div className="grid gap-3">
          {running.map((server) => <ServerCard key={server.id} server={server} />)}
        </div>
      </section>
      <div className="mt-6 grid gap-4 lg:grid-cols-[1fr_360px]">
        <Card className="p-4">
          <h2 className="font-semibold">{t("recentActivity")}</h2>
          <div className="mt-3 divide-y divide-panel-line">
            {activity.map((item, index) => (
              <div key={item} className="flex items-center justify-between py-3 text-sm">
                <span className="text-slate-300">{activityMessages[index] ?? item}</span>
                <span className="text-xs text-slate-500">{localizeRelativeTime(index === 0 ? "12 min ago" : `${index} h ago`, locale)}</span>
              </div>
            ))}
          </div>
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
