"use client";

import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { activity } from "@/lib/mock-data";
import { useI18n } from "@/lib/i18n";

export default function ActivityPage() {
  const { t } = useI18n();
  const activityMessages = [t("activityBackupJourney"), t("activityPlayerJoined"), t("activityClassicStarted"), t("activityBackupClassic")];
  return (
    <AppShell>
      <PageHeader title={t("activityTitle")} description={t("activityDescription")} />
      <Card className="divide-y divide-panel-line p-4">
        {activity.map((item, index) => <div key={item} className="py-3 text-sm text-slate-300">{activityMessages[index] ?? item}</div>)}
      </Card>
    </AppShell>
  );
}
