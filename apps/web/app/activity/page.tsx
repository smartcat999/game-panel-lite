"use client";

import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { useI18n } from "@/lib/i18n";

export default function ActivityPage() {
  const { t } = useI18n();
  return (
    <AppShell>
      <PageHeader title={t("activityTitle")} description={t("activityDescription")} />
      <Card className="p-6 text-sm text-slate-400">
        {t("activityNoLiveData")}
      </Card>
    </AppShell>
  );
}
