"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { applyDockerHost, getDockerHosts, getDockerStatus } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false });
  const dockerHosts = useQuery({ queryKey: ["docker-hosts"], queryFn: getDockerHosts, retry: false });
  const { t } = useI18n();
  const [selectedHost, setSelectedHost] = useState("");
  const [applyMessage, setApplyMessage] = useState("");
  const candidates = dockerHosts.data?.candidates ?? [];
  const selectedHostTrimmed = selectedHost.trim();
  const dockerHostMutation = useMutation({
    mutationFn: applyDockerHost,
    onSuccess: async (status) => {
      setApplyMessage(status.available ? t("dockerHostApplied") : status.message || t("dockerHostAppliedUnavailable"));
      window.localStorage.setItem("gamepanel.dockerHostDraft", status.host);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["docker-status"] }),
        queryClient.invalidateQueries({ queryKey: ["docker-hosts"] })
      ]);
    },
    onError: (error) => {
      setApplyMessage(error instanceof Error ? error.message : t("dockerHostApplyFailed"));
    }
  });

  useEffect(() => {
    const stored = window.localStorage.getItem("gamepanel.dockerHostDraft");
    setSelectedHost(stored || dockerHosts.data?.currentHost || docker.data?.host || "");
  }, [docker.data?.host, dockerHosts.data?.currentHost]);

  const updateSelectedHost = (host: string) => {
    setSelectedHost(host);
    window.localStorage.setItem("gamepanel.dockerHostDraft", host);
    setApplyMessage("");
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
      <Card className="mt-4 p-4">
        <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 className="font-semibold">{t("dockerHostScannerTitle")}</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-400">{t("dockerHostScannerDescription")}</p>
          </div>
          <Button className="shrink-0" variant="secondary" onClick={() => void dockerHosts.refetch()} disabled={dockerHosts.isFetching}>
            {dockerHosts.isFetching ? t("scanning") : t("scanDockerHosts")}
          </Button>
        </div>
        {dockerHosts.isError && <p className="mt-3 text-sm text-panel-gold">{t("dockerHostsUnavailable")}</p>}
        <div className="mt-4 grid gap-3 xl:grid-cols-[minmax(240px,320px)_1fr_auto]">
          <label className="grid gap-1 text-xs font-medium text-slate-400">
            {t("dockerHostSelectLabel")}
            <select
              className={cn(
                "h-11 rounded-md border border-panel-line bg-slate-950/70 px-3 text-sm text-white outline-none transition focus:border-panel-green",
                !candidates.length && "text-slate-500"
              )}
              value={candidates.some((candidate) => candidate.host === selectedHost) ? selectedHost : ""}
              onChange={(event) => updateSelectedHost(event.target.value)}
              disabled={!candidates.length}
            >
              <option value="">{t("customDockerHost")}</option>
              {candidates.map((candidate) => (
                <option key={`${candidate.source}-${candidate.host}`} value={candidate.host}>
                  {candidate.exists ? t("socketFound") : t("notDetected")} - {candidate.label}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-xs font-medium text-slate-400">
            {t("customDockerHost")}
            <Input
              value={selectedHost}
              onChange={(event) => updateSelectedHost(event.target.value)}
              placeholder="unix:///Users/you/.docker/run/docker.sock"
            />
          </label>
          <Button
            className="mt-auto h-11 shrink-0"
            variant="secondary"
            onClick={() => dockerHostMutation.mutate(selectedHostTrimmed)}
            disabled={!selectedHostTrimmed || dockerHostMutation.isPending}
          >
            <RefreshCw aria-hidden="true" className={dockerHostMutation.isPending ? "animate-spin" : undefined} />
            {dockerHostMutation.isPending ? t("applyingDockerHost") : t("applyDockerHost")}
          </Button>
        </div>
        <div className="mt-3 flex flex-col gap-1 text-xs text-slate-500 md:flex-row md:items-center md:justify-between">
          <p className="break-all">
            {t("selectedDockerHost")}: <span className="font-mono text-slate-300">{selectedHostTrimmed || t("none")}</span>
          </p>
          {applyMessage && (
            <p className={dockerHostMutation.isError ? "text-panel-gold" : "text-panel-green"}>{applyMessage}</p>
          )}
        </div>
        <p className="mt-2 text-xs text-slate-500">{t("dockerHostReconnectNote")}</p>
      </Card>
    </AppShell>
  );
}
