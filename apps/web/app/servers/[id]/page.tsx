import Link from "next/link";
import { notFound } from "next/navigation";
import { Copy } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import { backups, logs, servers, worlds } from "@/lib/mock-data";

export default async function ServerDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const server = servers.find((item) => item.id === id);
  if (!server) notFound();
  return (
    <AppShell>
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">Back to Servers</Link>
      <div className="mt-3 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold">{server.name}</h1>
            <ServerModeBadge mode={server.mode} />
            <ServerStatusBadge status={server.status} />
          </div>
          <p className="mt-2 text-sm text-slate-400">{server.players} / {server.maxPlayers} players · Port {server.port} · Version {server.version}</p>
        </div>
        <ServerActions server={server} />
      </div>
      <div className="mt-6 grid gap-4 xl:grid-cols-[1fr_320px]">
        <Card className="p-4">
          <div className="mb-4 flex gap-5 border-b border-panel-line pb-3 text-sm text-slate-400">
            {["Overview", "Console", "Logs", "Config", "Worlds", "Backups", ...(server.mode === "tmodloader" ? ["Mods"] : [])].map((tab) => (
              <span key={tab} className={tab === "Console" ? "text-panel-green" : ""}>{tab}</span>
            ))}
          </div>
          <div className="h-[420px] rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300">
            {logs.map((line) => <p key={line}><span className={line.includes("[Warn]") ? "text-panel-gold" : "text-panel-green"}>{line.slice(0, 18)}</span>{line.slice(18)}</p>)}
          </div>
          <div className="mt-3 flex gap-2">
            <Input placeholder="Enter command..." />
            <Button>Send</Button>
          </div>
        </Card>
        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">Join Server</h2>
            <CopyRow label="IP Address" value="192.168.1.20" />
            <CopyRow label="Port" value={String(server.port)} />
            <CopyRow label="Password" value={server.password || "None"} />
            <Button className="mt-4 w-full"><Copy aria-hidden="true" />Copy Invite Text</Button>
          </Card>
          <Card className="p-4">
            <h2 className="font-semibold">Server Info</h2>
            <Info label="World" value={server.world} />
            <Info label="Difficulty" value="Expert" />
            <Info label="World Size" value="Medium" />
            <Info label="Max Players" value={String(server.maxPlayers)} />
          </Card>
        </div>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card className="p-4"><h2 className="font-semibold">Worlds</h2><p className="mt-2 text-sm text-slate-400">{worlds[0]?.name}</p></Card>
        <Card className="p-4"><h2 className="font-semibold">Backups</h2><p className="mt-2 text-sm text-slate-400">{backups[0]?.name}</p></Card>
      </div>
    </AppShell>
  );
}

function CopyRow({ label, value }: { label: string; value: string }) {
  return <div className="mt-3"><p className="text-xs text-slate-400">{label}</p><div className="mt-1 flex items-center justify-between rounded-md bg-slate-950 px-3 py-2 text-sm"><span>{value}</span><button className="text-panel-green">Copy</button></div></div>;
}

function Info({ label, value }: { label: string; value: string }) {
  return <div className="mt-3 flex justify-between text-sm"><span className="text-slate-400">{label}</span><span>{value}</span></div>;
}
