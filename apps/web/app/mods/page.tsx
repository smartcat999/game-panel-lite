import { Package } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";

export default function ModsPage() {
  return (
    <AppShell>
      <PageHeader title="Mods" description="Manage tModLoader files." action={<Button variant="secondary"><Package aria-hidden="true" />Upload Mod</Button>} />
      <Card className="p-6 text-sm text-slate-400">
        tModLoader mod uploads are shown for modded servers only. Supported V1 files are `.tmod`, `install.txt`, and `enabled.json`.
      </Card>
    </AppShell>
  );
}
