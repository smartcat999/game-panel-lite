"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { applyDockerHost, getDockerHosts, getDockerStatus } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false, refetchInterval: 5000 });
  const dockerHosts = useQuery({ queryKey: ["docker-hosts"], queryFn: getDockerHosts, retry: false, enabled: false });
  const { t } = useI18n();
  const [selectedHost, setSelectedHost] = useState("");
  const [applyMessage, setApplyMessage] = useState("");
  const [scanMessage, setScanMessage] = useState("");
  const [scanMessageTone, setScanMessageTone] = useState<"success" | "warning">("success");
  const [isHostEditorOpen, setIsHostEditorOpen] = useState(false);
  const candidates = dockerHosts.data?.candidates ?? [];
  const singleCandidate = candidates.length === 1 ? candidates[0] : undefined;
  const selectedHostTrimmed = selectedHost.trim();
  const configuredHost = docker.data?.host ?? "GAMEPANEL_DOCKER_HOST";
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
    setScanMessage("");
  };

  const scanDockerHosts = async () => {
    setScanMessage("");
    const result = await dockerHosts.refetch();
    if (result.isError) {
      setScanMessage(t("dockerScanFailed"));
      setScanMessageTone("warning");
      return;
    }

    const count = result.data?.candidates.length ?? 0;
    setScanMessage(count > 0 ? t("dockerScanFound", { count }) : t("dockerScanEmpty"));
    setScanMessageTone(count > 0 ? "success" : "warning");
  };

  return (
    <AppShell>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.6fr)]">
        <Card className="p-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex min-w-0 items-start gap-3">
              <ShieldCheck className={docker.data?.available ? "mt-1 shrink-0 text-panel-green" : "mt-1 shrink-0 text-panel-gold"} aria-hidden="true" />
              <div className="min-w-0">
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
                    : docker.isError ? t("dockerApiUnavailable")
                    : t("dockerStatusLoading")}
                </p>
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-1 rounded-md border border-panel-line bg-slate-950/45 p-1">
              <Button className="h-9 shrink-0 px-3" variant="ghost" onClick={() => void scanDockerHosts()} disabled={dockerHosts.isFetching}>
                <RefreshCw aria-hidden="true" className={cn("size-4", dockerHosts.isFetching && "animate-spin")} />
                {dockerHosts.isFetching ? t("scanning") : t("scanDockerHosts")}
              </Button>
              <Button className="h-9 w-24 shrink-0 px-3" variant="ghost" onClick={() => setIsHostEditorOpen((value) => !value)} aria-expanded={isHostEditorOpen}>
                {isHostEditorOpen ? <ChevronUp aria-hidden="true" className="size-4" /> : <ChevronDown aria-hidden="true" className="size-4" />}
                {isHostEditorOpen ? t("hideDockerHostOptions") : t("changeDockerHost")}
              </Button>
            </div>
          </div>

          <div className="mt-5 rounded-md border border-panel-line bg-slate-950/60 px-4 py-3">
            <div className="text-xs font-medium text-slate-500">{t("currentDockerHost")}</div>
            <div className="mt-1 break-all font-mono text-sm text-slate-200">{configuredHost}</div>
          </div>

          {(scanMessage || dockerHosts.isError) && (
            <p className={cn("mt-3 text-sm", scanMessageTone === "success" && scanMessage ? "text-panel-green" : "text-panel-gold")}>
              {scanMessage || t("dockerHostsUnavailable")}
            </p>
          )}

          {isHostEditorOpen && (
            <div className="mt-4 border-t border-panel-line pt-4">
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <div className="text-xs font-medium text-slate-400">{t("dockerHostSelectLabel")}</div>
                  {candidates.length === 0 && (
                    <p className="rounded-md border border-panel-line bg-slate-950/45 px-3 py-2 text-sm text-slate-500">
                      {t("noDockerHostCandidates")}
                    </p>
                  )}
                  {singleCandidate && (
                    <button
                      type="button"
                      className={cn(
                        "flex min-h-11 items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/70 px-3 text-left text-sm text-white transition hover:border-panel-green",
                        selectedHost === singleCandidate.host && "border-panel-green"
                      )}
                      onClick={() => updateSelectedHost(singleCandidate.host)}
                    >
                      <span className="min-w-0">
                        <span className="block font-medium">{singleCandidate.label}</span>
                        <span className="block truncate font-mono text-xs text-slate-500">{singleCandidate.host}</span>
                      </span>
                      <span className={singleCandidate.exists ? "shrink-0 text-xs text-panel-green" : "shrink-0 text-xs text-panel-gold"}>
                        {singleCandidate.exists ? t("socketFound") : t("notDetected")}
                      </span>
                    </button>
                  )}
                  {candidates.length > 1 && (
                    <select
                      className="h-11 rounded-md border border-panel-line bg-slate-950/70 px-3 text-sm text-white outline-none transition focus:border-panel-green"
                      value={candidates.some((candidate) => candidate.host === selectedHost) ? selectedHost : ""}
                      onChange={(event) => updateSelectedHost(event.target.value)}
                    >
                      <option value="">{t("chooseDockerHostCandidate")}</option>
                      {candidates.map((candidate) => (
                        <option key={`${candidate.source}-${candidate.host}`} value={candidate.host}>
                          {candidate.exists ? t("socketFound") : t("notDetected")} - {candidate.label}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
                <label className="grid gap-1 text-xs font-medium text-slate-400">
                  {t("customDockerHost")}
                  <Input
                    value={selectedHost}
                    onChange={(event) => updateSelectedHost(event.target.value)}
                    placeholder="unix:///Users/you/.docker/run/docker.sock"
                  />
                </label>
              </div>
              <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <p className="break-all text-sm text-slate-400">
                  {t("selectedDockerHost")}: <span className="font-mono text-slate-300">{selectedHostTrimmed || t("none")}</span>
                </p>
                <Button
                  className="h-11 shrink-0"
                  variant="secondary"
                  onClick={() => dockerHostMutation.mutate(selectedHostTrimmed)}
                  disabled={!selectedHostTrimmed || dockerHostMutation.isPending}
                >
                  <RefreshCw aria-hidden="true" className={dockerHostMutation.isPending ? "animate-spin" : undefined} />
                  {dockerHostMutation.isPending ? t("applyingDockerHost") : t("applyDockerHost")}
                </Button>
              </div>
              {applyMessage && (
                <p className={dockerHostMutation.isError ? "mt-2 text-xs text-panel-gold" : "mt-2 text-xs text-panel-green"}>{applyMessage}</p>
              )}
              <p className="mt-2 text-xs text-slate-500">{t("dockerHostReconnectNote")}</p>
            </div>
          )}
        </Card>
        <Card className="p-5">
          <h2 className="font-semibold">{t("dataDirectories")}</h2>
          <p className="mt-3 text-sm text-slate-400">{t("dataDirectoriesDescription")}</p>
        </Card>
      </div>
    </AppShell>
  );
}
