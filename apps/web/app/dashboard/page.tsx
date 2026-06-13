import Link from "next/link";
import { Archive, HardDrive, Plus, Users } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Card } from "@/components/ui";
import { activity, servers } from "@/lib/mock-data";

export default function DashboardPage() {
  const running = servers.filter((server) => server.status === "running");
  const players = servers.reduce((sum, server) => sum + server.players, 0);
  return (
    <AppShell>
      <PageHeader title="Dashboard" description="Manage your game servers in one place." />
      <div className="grid gap-4 md:grid-cols-4">
        <Stat icon={<HardDrive />} label="Running Servers" value={`${running.length} / ${servers.length}`} hint={`${running.length} running`} />
        <Stat icon={<Users />} label="Online Players" value={`${players} / 32`} hint={`${players} players online`} />
        <Stat icon={<Archive />} label="Latest Backup" value="12 min ago" hint="Journey Friends" />
        <Stat icon={<HardDrive />} label="Storage Used" value="3.8 GB" hint="of 20 GB" />
      </div>
      <section className="mt-6">
        <h2 className="mb-3 text-base font-semibold">Active Servers</h2>
        <div className="grid gap-3">
          {running.map((server) => <ServerCard key={server.id} server={server} />)}
        </div>
      </section>
      <div className="mt-6 grid gap-4 lg:grid-cols-[1fr_360px]">
        <Card className="p-4">
          <h2 className="font-semibold">Recent Activity</h2>
          <div className="mt-3 divide-y divide-panel-line">
            {activity.map((item, index) => (
              <div key={item} className="flex items-center justify-between py-3 text-sm">
                <span className="text-slate-300">{item}</span>
                <span className="text-xs text-slate-500">{index === 0 ? "12 min ago" : `${index} h ago`}</span>
              </div>
            ))}
          </div>
        </Card>
        <Card className="p-4">
          <h2 className="font-semibold">Quick Actions</h2>
          <div className="mt-4 flex flex-col gap-3">
            <Link href="/servers/new"><Button className="w-full"><Plus aria-hidden="true" />Create Server</Button></Link>
            <Button variant="secondary">Import World</Button>
            <Button variant="gold">Backup All</Button>
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
