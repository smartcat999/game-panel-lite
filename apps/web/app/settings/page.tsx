"use client";

import { useQuery } from "@tanstack/react-query";
import { Copy, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { getDockerHosts, getDockerStatus } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false });
  const dockerHosts = useQuery({ queryKey: ["docker-hosts"], queryFn: getDockerHosts, retry: false });
  const { t } = useI18n();
  const [selectedHost, setSelectedHost] = useState("");
  const [copied, setCopied] = useState(false);
  const candidates = dockerHosts.data?.candidates ?? [];
  const restartCommand = useMemo(() => {
    const host = selectedHost.trim();
    if (!host) return "";
    return `GAMEPANEL_DOCKER_HOST=${JSON.stringify(host)} go run ./apps/api/cmd/server`;
  }, [selectedHost]);

  useEffect(() => {
    const stored = window.localStorage.getItem("gamepanel.dockerHostDraft");
    setSelectedHost(stored || dockerHosts.data?.currentHost || docker.data?.host || "");
  }, [docker.data?.host, dockerHosts.data?.currentHost]);

  const updateSelectedHost = (host: string) => {
    setSelectedHost(host);
    window.localStorage.setItem("gamepanel.dockerHostDraft", host);
    setCopied(false);
  };

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
      <Card className="mt-4 p-5">
        <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
          <div>
            <h2 className="font-semibold">{t("dockerHostScannerTitle")}</h2>
            <p className="mt-2 max-w-3xl text-sm text-slate-400">{t("dockerHostScannerDescription")}</p>
          </div>
          <Button variant="secondary" onClick={() => void dockerHosts.refetch()} disabled={dockerHosts.isFetching}>
            {dockerHosts.isFetching ? t("scanning") : t("scanDockerHosts")}
          </Button>
        </div>
        {dockerHosts.isError && <p className="mt-4 text-sm text-panel-gold">{t("dockerHostsUnavailable")}</p>}
        <div className="mt-5 grid gap-3 lg:grid-cols-2">
          {candidates.map((candidate) => (
            <button
              key={`${candidate.source}-${candidate.host}`}
              className={cn(
                "rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left transition hover:border-panel-green",
                selectedHost === candidate.host && "border-panel-green bg-panel-green/10"
              )}
              type="button"
              onClick={() => updateSelectedHost(candidate.host)}
            >
              <div className="flex items-center justify-between gap-3">
                <p className="font-medium text-white">{candidate.label}</p>
                <span className={candidate.exists ? "text-xs text-panel-green" : "text-xs text-slate-500"}>
                  {candidate.exists ? t("socketFound") : t("notDetected")}
                </span>
              </div>
              <p className="mt-2 break-all font-mono text-xs text-slate-400">{candidate.host}</p>
              {candidate.active && <p className="mt-2 text-xs text-panel-green">{t("currentlyActive")}</p>}
            </button>
          ))}
        </div>
        <div className="mt-5 grid gap-3 lg:grid-cols-[1fr_auto]">
          <Input
            value={selectedHost}
            onChange={(event) => updateSelectedHost(event.target.value)}
            placeholder="unix:///Users/you/.docker/run/docker.sock"
          />
          <Button
            variant="secondary"
            onClick={() => {
              if (!restartCommand) return;
              void navigator.clipboard.writeText(restartCommand).then(() => setCopied(true));
            }}
            disabled={!restartCommand}
          >
            <Copy aria-hidden="true" />
            {copied ? t("copied") : t("copyRestartCommand")}
          </Button>
        </div>
        {restartCommand && (
          <div className="mt-3 break-all rounded-md border border-panel-line bg-slate-950/70 px-3 py-2 font-mono text-xs text-slate-300">
            {restartCommand}
          </div>
        )}
        <p className="mt-3 text-xs text-slate-500">{t("dockerHostRestartNote")}</p>
      </Card>
    </AppShell>
  );
}
