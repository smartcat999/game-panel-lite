"use client";

import { useQuery } from "@tanstack/react-query";
import { Database, ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getDockerStatus, getSettings } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const { t } = useI18n();
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false, refetchInterval: 5000 });
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const configuredHost = settings.data?.dockerHost ?? docker.data?.host ?? "GAMEPANEL_DOCKER_HOST";

  return (
    <>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.6fr)]">
        <Card className="p-5">
          <div className="flex min-w-0 items-start gap-3">
            <ShieldCheck className={docker.data?.available ? "mt-1 shrink-0 text-panel-green" : "mt-1 shrink-0 text-panel-gold"} aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="font-semibold text-white">{t("dockerRuntime")}</h2>
                <span
                  className={cn(
                    "inline-flex min-w-14 items-center justify-center rounded px-2 py-0.5 text-xs",
                    docker.data?.available ? "bg-panel-green/15 text-panel-green" : "bg-panel-gold/15 text-panel-gold"
                  )}
                >
                  {docker.data?.available ? t("available") : t("unavailable")}
                </span>
              </div>
              <p className="mt-1 text-sm text-slate-400">
                {docker.data
                  ? docker.data.available ? t("dockerRuntimeReady") : t("dockerRuntimeUnavailable")
                  : docker.isError ? t("dockerStatusUnavailable")
                  : t("dockerStatusLoading")}
              </p>
              <div className="mt-5 rounded-md border border-panel-line bg-slate-950/60 px-4 py-3">
                <div className="text-xs font-medium text-slate-500">{t("configuredDockerHost")}</div>
                <div className="mt-1 break-all font-mono text-sm text-slate-200">{configuredHost}</div>
              </div>
              <p className="mt-3 text-xs text-slate-500">{t("dockerHostConfigNote")}</p>
            </div>
          </div>
        </Card>

        <Card className="p-5">
          <div className="flex items-start gap-3">
            <Database className="mt-1 shrink-0 text-slate-400" aria-hidden="true" />
            <div>
              <h2 className="font-semibold">{t("dataDirectories")}</h2>
              <p className="mt-3 text-sm text-slate-400">{t("dataDirectoriesDescription")}</p>
              {settings.data ? (
                <div className="mt-4 space-y-3">
                  <SettingValue label={t("dataDir")} value={settings.data.dataDir} />
                  <SettingValue label={t("dbPath")} value={settings.data.dbPath} />
                </div>
              ) : null}
            </div>
          </div>
        </Card>
      </div>
    </>
  );
}

function SettingValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/45 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 break-all font-mono text-sm text-slate-200">{value}</p>
    </div>
  );
}
