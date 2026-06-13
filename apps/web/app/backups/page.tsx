import { Archive, RotateCcw, Trash2 } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { backups } from "@/lib/mock-data";

export default function BackupsPage() {
  return (
    <AppShell>
      <PageHeader title="Backups" description="Your server backups." action={<Button variant="gold"><Archive aria-hidden="true" />Backup Now</Button>} />
      <Card className="overflow-hidden">
        <table className="w-full text-left text-sm">
          <thead className="bg-slate-950/50 text-xs text-slate-400">
            <tr>{["Backup Name", "Server", "World", "Type", "Size", "Created", "Actions"].map((head) => <th key={head} className="px-4 py-3 font-medium">{head}</th>)}</tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {backups.map((backup) => (
              <tr key={backup.id}>
                <td className="px-4 py-4">{backup.name}</td>
                <td className="px-4 py-4 text-slate-300">{backup.server}</td>
                <td className="px-4 py-4 text-slate-300">{backup.world}</td>
                <td className="px-4 py-4 text-slate-300">{backup.type}</td>
                <td className="px-4 py-4 text-slate-300">{backup.size}</td>
                <td className="px-4 py-4 text-slate-300">{backup.created}</td>
                <td className="px-4 py-4"><div className="flex gap-2"><Button variant="secondary"><RotateCcw aria-hidden="true" /></Button><Button variant="danger"><Trash2 aria-hidden="true" /></Button></div></td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </AppShell>
  );
}
