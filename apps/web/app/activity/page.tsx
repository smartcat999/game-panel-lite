import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { activity } from "@/lib/mock-data";

export default function ActivityPage() {
  return (
    <AppShell>
      <PageHeader title="Activity" description="Recent server activity." />
      <Card className="divide-y divide-panel-line p-4">
        {activity.map((item) => <div key={item} className="py-3 text-sm text-slate-300">{item}</div>)}
      </Card>
    </AppShell>
  );
}
