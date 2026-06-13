import { Plus, Upload } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { worlds } from "@/lib/mock-data";

export default function WorldsPage() {
  return (
    <AppShell>
      <PageHeader title="Worlds" description="Manage your world files." action={<Button variant="secondary"><Upload aria-hidden="true" />Import World</Button>} />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {worlds.map((world) => (
          <Card key={world.id} className="p-4">
            <div className="flex items-start justify-between">
              <div>
                <h2 className="font-semibold">{world.name}</h2>
                <div className="mt-3 flex gap-2 text-xs text-slate-300"><span className="rounded bg-slate-800 px-2 py-1">{world.size}</span><span className="rounded bg-slate-800 px-2 py-1">{world.difficulty}</span></div>
              </div>
              {world.server && <span className="rounded bg-panel-green/15 px-2 py-1 text-xs text-panel-green">In Use</span>}
            </div>
            <p className="mt-4 text-sm text-slate-400">Modified: {world.modified}</p>
            <p className="text-sm text-slate-400">Used by: {world.server || "Not in use"}</p>
            <p className="text-sm text-slate-400">Size: {world.bytes}</p>
            <div className="mt-4 flex gap-2"><Button variant="secondary">Backup</Button><Button variant="ghost">More</Button></div>
          </Card>
        ))}
        <Card className="flex min-h-52 items-center justify-center border-dashed p-4 text-slate-400">
          <div className="text-center"><Plus aria-hidden="true" className="mx-auto" /><p className="mt-2 text-sm">Create New World</p></div>
        </Card>
      </div>
    </AppShell>
  );
}
