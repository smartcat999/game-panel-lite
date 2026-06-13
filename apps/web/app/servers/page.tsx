import Link from "next/link";
import { Plus } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Input } from "@/components/ui";
import { servers } from "@/lib/mock-data";

export default function ServersPage() {
  return (
    <AppShell>
      <PageHeader
        title="Servers"
        description="Create and manage your game servers."
        action={<Link href="/servers/new"><Button><Plus aria-hidden="true" />Create Server</Button></Link>}
      />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Input className="max-w-sm" placeholder="Search servers..." />
        {["All", "Running", "Stopped", "Vanilla", "Modded"].map((filter) => (
          <Button key={filter} variant="secondary" className="px-3 py-2">{filter}</Button>
        ))}
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        {servers.map((server) => <ServerCard key={server.id} server={server} />)}
      </div>
    </AppShell>
  );
}
