"use client";

import { useQuery } from "@tanstack/react-query";
import { ShieldCheck } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getDockerStatus } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function SettingsPage() {
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false });
  const { t } = useI18n();
  return (
    <AppShell>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />
      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="p-5">
          <div className="flex items-center gap-3 text-panel-green">
            <ShieldCheck aria-hidden="true" />
            <h2 className="font-semibold text-white">{t("dockerRuntime")}</h2>
          </div>
          <p className="mt-3 text-sm text-slate-400">
            {docker.data ? docker.data.message : docker.isError ? t("dockerApiUnavailable") : t("dockerChecking")}
          </p>
          {docker.data && (
            <p className={docker.data.available ? "mt-2 text-sm text-panel-green" : "mt-2 text-sm text-panel-gold"}>
              {docker.data.available ? t("available") : t("unavailable")}
            </p>
          )}
        </Card>
        <Card className="p-5">
          <h2 className="font-semibold">{t("dockerSockTitle")}</h2>
          <p className="mt-3 text-sm text-slate-400">{t("dockerSockDescription")}</p>
          <div className="mt-4 break-all rounded-md border border-panel-line bg-slate-950/70 px-3 py-2 font-mono text-xs text-slate-300">
            {t("configuredValue")}: {docker.data?.host ?? "GAMEPANEL_DOCKER_HOST"}
          </div>
        </Card>
        <Card className="p-5">
          <h2 className="font-semibold">{t("dataDirectories")}</h2>
          <p className="mt-3 text-sm text-slate-400">{t("dataDirectoriesDescription")}</p>
        </Card>
      </div>
    </AppShell>
  );
}
