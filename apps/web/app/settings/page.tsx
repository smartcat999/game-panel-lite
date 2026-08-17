"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Download, ExternalLink, Globe2, LockKeyhole, RefreshCw, RotateCcw, Save, ServerCog, ShieldCheck, Wrench } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card, Input, ToastNotice } from "@/components/ui";
import {
  applySystemUpdate,
  checkSystemUpdate,
  getDeploymentStatus,
  getSettings,
  getSystemUpdateStatus,
  reconcileDeployment,
  renewDeploymentHTTPS,
  restartDeployment,
  setupDeploymentHTTPS,
  updateImageRegion,
  updatePublicHost,
  updateSystemAutoCheck
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type ImageRegion = "global" | "cn";
type SettingsTab = "basic" | "access" | "maintenance";

export default function SettingsPage() {
  const { t } = useI18n();
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const [publicHost, setPublicHost] = useState<string | null>(null);
  const [imageRegion, setImageRegion] = useState<ImageRegion | null>(null);
  const [notice, setNotice] = useState<{ message: string; tone: "success" | "error" } | null>(null);
  const [activeTab, setActiveTab] = useState<SettingsTab>("basic");
  const settingsTabs: { icon: ReactNode; key: SettingsTab; label: string }[] = [
    { key: "basic", label: t("settingsTabBasic"), icon: <ServerCog className="size-4" /> },
    { key: "access", label: t("settingsTabAccess"), icon: <LockKeyhole className="size-4" /> },
    { key: "maintenance", label: t("settingsTabMaintenance"), icon: <Wrench className="size-4" /> }
  ];

  const savedPublicHost = settings.data?.publicHost ?? "";
  const savedImageRegion: ImageRegion = settings.data?.imageRegion === "cn" ? "cn" : "global";
  const publicHostValue = publicHost ?? savedPublicHost;
  const imageRegionValue = imageRegion ?? savedImageRegion;
  const normalizedPublicHost = publicHostValue.trim();
  const publicHostDirty = publicHost !== null && normalizedPublicHost !== savedPublicHost.trim();
  const imageRegionDirty = imageRegion !== null && imageRegion !== savedImageRegion;
  const dirty = publicHostDirty || imageRegionDirty;
  const publicHostError = validatePublicHost(normalizedPublicHost, t("publicHostInvalid"));
  const configuredRegistry = imageRegionValue === savedImageRegion
    ? settings.data?.gameImageRegistry
    : undefined;
  const resolvedRegistry = formatRegistrySource(
    configuredRegistry ?? (imageRegionValue === "cn"
      ? "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite"
      : "smartcat99999"),
    imageRegionValue
  );

  const saveSettings = useMutation({
    mutationFn: async () => {
      if (publicHostDirty) await updatePublicHost(normalizedPublicHost);
      if (imageRegionDirty) await updateImageRegion(imageRegionValue);
    },
    onSuccess: async () => {
      const restartRequired = imageRegionDirty;
      await settings.refetch();
      setPublicHost(null);
      setImageRegion(null);
      setNotice({ message: restartRequired ? t("settingsSavedRestartRequired") : t("settingsSaved"), tone: "success" });
    },
    onError: (error) => setNotice({
      message: error instanceof Error ? error.message : t("settingsSaveFailed"),
      tone: "error"
    })
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!dirty || publicHostError) return;
    setNotice(null);
    saveSettings.mutate();
  };

  const discard = () => {
    setPublicHost(null);
    setImageRegion(null);
    setNotice(null);
  };

  return (
    <>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />

      {notice ? (
        <div className="pointer-events-none fixed inset-x-4 bottom-4 z-[60] flex justify-end md:inset-x-auto md:bottom-auto md:right-6 md:top-24">
          <ToastNotice closeLabel={t("close")} message={notice.message} tone={notice.tone} onClose={() => setNotice(null)} />
        </div>
      ) : null}

      <label className="mb-5 block sm:hidden">
        <span className="sr-only">{t("settingsSections")}</span>
        <select
          aria-label={t("settingsSections")}
          className="h-11 w-full rounded-md border border-panel-line bg-panel-card px-3 text-sm font-medium text-slate-100 outline-none focus:border-panel-green focus:ring-2 focus:ring-panel-green/20"
          value={activeTab}
          onChange={(event) => setActiveTab(event.target.value as SettingsTab)}
        >
          {settingsTabs.map((tab) => <option key={tab.key} value={tab.key}>{tab.label}</option>)}
        </select>
      </label>
      <nav aria-label={t("settingsSections")} className="mb-5 hidden overflow-x-auto border-b border-panel-line sm:flex">
        {settingsTabs.map((tab) => <SettingsTabButton key={tab.key} active={activeTab === tab.key} icon={tab.icon} label={tab.label} onClick={() => setActiveTab(tab.key)} />)}
      </nav>

      {activeTab === "basic" ? <form onSubmit={submit}>
        <Card className="overflow-hidden">
          <div className="border-b border-panel-line px-5 py-4 md:px-6">
            <h2 className="font-semibold text-white">{t("basicSettings")}</h2>
            <p className="mt-1 text-sm text-slate-400">{t("basicSettingsDescription")}</p>
          </div>

          <SettingRow label={t("publicHostTitle")} description={t("publicHostDescription")}>
            <div className="w-full max-w-xl">
              <Input
                aria-describedby="public-host-hint"
                aria-invalid={Boolean(publicHostError)}
                className={cn("w-full font-mono", publicHostError && "border-red-400 focus:border-red-400")}
                disabled={settings.isLoading || saveSettings.isPending}
                placeholder={t("publicHostPlaceholder")}
                value={publicHostValue}
                onChange={(event) => {
                  setPublicHost(event.target.value);
                  setNotice(null);
                }}
              />
              <p id="public-host-hint" className={cn("mt-2 text-xs", publicHostError ? "text-red-300" : "text-slate-500")}>
                {publicHostError || t("publicHostInputHint")}
              </p>
            </div>
          </SettingRow>

          <SettingRow label={t("imageRegion")} description={t("imageRegionDescription")} badge={t("restartPanelRequired")}>
            <div className="w-full max-w-xl">
              <fieldset disabled={settings.isLoading || saveSettings.isPending}>
                <legend className="sr-only">{t("imageRegion")}</legend>
                <div className="grid gap-2 sm:grid-cols-2">
                  <RegionOption
                    checked={imageRegionValue === "global"}
                    description="Docker Hub"
                    label={t("imageRegionGlobal")}
                    name="image-region"
                    onChange={() => {
                      setImageRegion("global");
                      setNotice(null);
                    }}
                    value="global"
                  />
                  <RegionOption
                    checked={imageRegionValue === "cn"}
                    description={t("aliyunContainerRegistry")}
                    label={t("imageRegionChina")}
                    name="image-region"
                    onChange={() => {
                      setImageRegion("cn");
                      setNotice(null);
                    }}
                    value="cn"
                  />
                </div>
              </fieldset>
              <div className="mt-3 flex min-w-0 items-center gap-2 text-xs text-slate-500">
                <Globe2 aria-hidden="true" className="size-4 shrink-0" />
                <span>{t("resolvedGameImageSource")}</span>
                <code className="min-w-0 truncate text-slate-300" title={resolvedRegistry}>{resolvedRegistry}</code>
              </div>
            </div>
          </SettingRow>

          {dirty ? (
            <div className="flex flex-wrap items-center justify-end gap-2 border-t border-panel-line bg-slate-950/25 px-5 py-4 md:px-6">
              <Button type="button" variant="ghost" disabled={saveSettings.isPending} onClick={discard}>
                <RotateCcw aria-hidden="true" className="size-4" />
                {t("discardChanges")}
              </Button>
              <Button type="submit" disabled={saveSettings.isPending || Boolean(publicHostError)}>
                <Save aria-hidden="true" className="size-4" />
                {saveSettings.isPending ? t("saving") : t("saveSettings")}
              </Button>
            </div>
          ) : null}
        </Card>
      </form> : null}

      {activeTab === "access" ? <HTTPSSettings onNotice={setNotice} /> : null}
      {activeTab === "maintenance" ? <><DeploymentMaintenance onNotice={setNotice} /><PanelUpdateCard onNotice={setNotice} /></> : null}
    </>
  );
}

function SettingsTabButton({ active, icon, label, onClick }: { active: boolean; icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      aria-current={active ? "page" : undefined}
      className={cn(
        "inline-flex h-11 shrink-0 items-center gap-2 whitespace-nowrap border-b-2 px-4 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/40",
        active ? "border-panel-green text-white" : "border-transparent text-slate-500 hover:text-slate-200"
      )}
      type="button"
      onClick={onClick}
    >
      {icon}<span>{label}</span>
    </button>
  );
}

function DeploymentMaintenance({ onNotice }: { onNotice: (notice: { message: string; tone: "success" | "error" } | null) => void }) {
  const { locale, t } = useI18n();
  const queryClient = useQueryClient();
  const [restartConfirmOpen, setRestartConfirmOpen] = useState(false);
  const deployment = useQuery({
    queryKey: ["system-deployment"],
    queryFn: getDeploymentStatus,
    retry: false,
    refetchInterval: (query) => query.state.data?.job?.status === "running" ? 3_000 : 15_000
  });
  const reconcile = useMutation({
    mutationFn: reconcileDeployment,
    onSuccess: async () => {
      onNotice({ message: t("deploymentReconcileQueued"), tone: "success" });
      await queryClient.invalidateQueries({ queryKey: ["system-deployment"] });
    },
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("deploymentActionFailed"), tone: "error" })
  });
  const restart = useMutation({
    mutationFn: restartDeployment,
    onSuccess: async () => {
      setRestartConfirmOpen(false);
      onNotice({ message: t("deploymentRestartQueued"), tone: "success" });
      await queryClient.invalidateQueries({ queryKey: ["system-deployment"] });
    },
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("deploymentActionFailed"), tone: "error" })
  });
  const data = deployment.data;
  const jobRunning = data?.job?.status === "running";
  const checkedAt = data?.checkedAt ? formatDateTime(data.checkedAt, locale) : "—";

  return (
    <>
      <Card className="overflow-hidden">
        <div className="flex flex-col gap-3 border-b border-panel-line px-5 py-4 sm:flex-row sm:items-start sm:justify-between md:px-6">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-semibold text-white">{t("deploymentTitle")}</h2>
              {data ? <Badge className={data.manager === "standalone" ? "bg-slate-800 text-slate-300" : data.healthy ? "bg-panel-green/12 text-panel-green" : "bg-panel-gold/12 text-panel-gold"}>{data.manager === "standalone" ? t("deploymentStandalone") : data.healthy ? t("deploymentHealthy") : t("deploymentNeedsAttention")}</Badge> : null}
            </div>
            <p className="mt-1 text-sm text-slate-400">{t("deploymentDescription")}</p>
            {data?.manager === "standalone" ? <p className="mt-1 text-xs text-slate-500">{t("deploymentStandaloneHint")}</p> : null}
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button type="button" variant="secondary" disabled={deployment.isFetching || jobRunning} onClick={() => void deployment.refetch()}>
              <RefreshCw className={cn("size-4", deployment.isFetching && "animate-spin")} />{t("refreshStatus")}
            </Button>
            {data?.capabilities.reconcile && !data.healthy ? <Button type="button" disabled={reconcile.isPending || jobRunning} onClick={() => reconcile.mutate()}><Wrench className="size-4" />{t("restoreServices")}</Button> : null}
            <Button type="button" variant="secondary" disabled={!data?.capabilities.restart || restart.isPending || jobRunning} onClick={() => setRestartConfirmOpen(true)}><RotateCcw className="size-4" />{t("restartControlPlane")}</Button>
          </div>
        </div>

        {deployment.isLoading ? (
          <div className="space-y-px bg-panel-line"><div className="h-12 animate-pulse bg-panel-card" /><div className="h-12 animate-pulse bg-panel-card" /><div className="h-12 animate-pulse bg-panel-card" /></div>
        ) : deployment.isError ? (
          <div className="px-5 py-5 text-sm text-panel-gold md:px-6">{t("deploymentUnavailable")}</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[680px] table-fixed border-collapse text-left text-sm">
              <thead className="bg-slate-950/45 text-xs text-slate-500">
                <tr><th className="w-48 px-5 py-2.5 md:px-6">{t("deploymentService")}</th><th className="w-32 px-3 py-2.5">{t("status")}</th><th className="px-3 py-2.5">{t("deploymentImage")}</th><th className="w-44 px-5 py-2.5 text-right md:px-6">{t("lastChecked")}</th></tr>
              </thead>
              <tbody className="divide-y divide-panel-line">
                {(data?.services ?? []).map((service) => {
                  const running = service.state === "running" && service.health !== "unhealthy";
                  return <tr key={service.name}><td className="px-5 py-3 font-medium text-slate-200 md:px-6">{deploymentServiceLabel(service.name, t)}</td><td className="px-3 py-3"><span className={cn("inline-flex items-center gap-1.5", running ? "text-panel-green" : "text-panel-gold")}><span className={cn("size-1.5 rounded-full", running ? "bg-panel-green" : "bg-panel-gold")} />{running ? t("statusRunning") : deploymentStateLabel(service.state, t)}</span></td><td className="truncate px-3 py-3 font-mono text-xs text-slate-500" title={service.image}>{service.image || "—"}</td><td className="px-5 py-3 text-right text-xs text-slate-500 md:px-6">{checkedAt}</td></tr>;
                })}
              </tbody>
            </table>
          </div>
        )}

        {data?.job?.status && (data.job.kind === "reconcile" || data.job.kind === "restart") ? <OperationStatus job={data.job} /> : null}
        <details className="border-t border-panel-line px-5 py-4 md:px-6">
          <summary className="cursor-pointer text-sm font-medium text-slate-300">{t("advancedCommandMaintenance")}</summary>
          <p className="mt-2 text-xs leading-5 text-slate-500">{t("advancedCommandMaintenanceHint")}</p>
          <pre className="mt-3 overflow-x-auto rounded-md bg-slate-950/70 p-3 text-xs leading-6 text-slate-300">sudo sh scripts/manage.sh status{"\n"}sudo sh scripts/manage.sh stop</pre>
        </details>
      </Card>

      <ConfirmDialog
        busy={restart.isPending}
        cancelLabel={t("cancel")}
        confirmLabel={t("confirmRestartControlPlane")}
        confirmVariant="primary"
        description={t("restartControlPlaneDescription")}
        eyebrow={t("panelMaintenance")}
        eyebrowTone="gold"
        open={restartConfirmOpen}
        title={t("restartControlPlane")}
        onCancel={() => !restart.isPending && setRestartConfirmOpen(false)}
        onConfirm={() => restart.mutate()}
      />
    </>
  );
}

function HTTPSSettings({ onNotice }: { onNotice: (notice: { message: string; tone: "success" | "error" } | null) => void }) {
  const { locale, t } = useI18n();
  const queryClient = useQueryClient();
  const [domain, setDomain] = useState("");
  const [email, setEmail] = useState("");
  const [setupConfirmOpen, setSetupConfirmOpen] = useState(false);
  const deployment = useQuery({
    queryKey: ["system-deployment"],
    queryFn: getDeploymentStatus,
    retry: false,
    refetchInterval: (query) => query.state.data?.job?.status === "running" ? 3_000 : 30_000
  });
  const setup = useMutation({
    mutationFn: () => setupDeploymentHTTPS(domain.trim(), email.trim()),
    onSuccess: async () => {
      setSetupConfirmOpen(false);
      onNotice({ message: t("httpsSetupQueued"), tone: "success" });
      await queryClient.invalidateQueries({ queryKey: ["system-deployment"] });
    },
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("httpsActionFailed"), tone: "error" })
  });
  const renew = useMutation({
    mutationFn: renewDeploymentHTTPS,
    onSuccess: async () => {
      onNotice({ message: t("httpsRenewQueued"), tone: "success" });
      await queryClient.invalidateQueries({ queryKey: ["system-deployment"] });
    },
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("httpsActionFailed"), tone: "error" })
  });
  const data = deployment.data;
  const jobRunning = data?.job?.status === "running";
  const https = data?.https;
  const configured = Boolean(https?.configured);
  const domainValid = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/i.test(domain.trim());

  return (
    <>
    <Card className="overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-panel-line px-5 py-4 sm:flex-row sm:items-start sm:justify-between md:px-6">
        <div>
          <div className="flex flex-wrap items-center gap-2"><h2 className="font-semibold text-white">{t("httpsTitle")}</h2>{data ? <Badge className={configured ? "bg-panel-green/12 text-panel-green" : "bg-slate-800 text-slate-400"}>{configured ? "HTTPS" : "HTTP"}</Badge> : null}</div>
          <p className="mt-1 text-sm text-slate-400">{t("httpsDescription")}</p>
        </div>
        {configured ? <Button type="button" variant="secondary" disabled={!data?.capabilities.httpsRenew || renew.isPending || jobRunning} onClick={() => renew.mutate()}><RefreshCw className={cn("size-4", renew.isPending && "animate-spin")} />{t("renewCertificate")}</Button> : null}
      </div>

      {deployment.isLoading ? <div className="h-28 animate-pulse bg-slate-950/20" /> : deployment.isError ? <div className="px-5 py-5 text-sm text-panel-gold md:px-6">{t("deploymentUnavailable")}</div> : configured ? (
        <div className="grid border-b border-panel-line sm:grid-cols-2 xl:grid-cols-4">
          <UpdateValue label={t("accessMode")} value="HTTPS" />
          <UpdateValue label={t("httpsDomain")} value={https?.domain ?? "—"} />
          <UpdateValue label={t("certificateExpiry")} value={https?.expiresAt ? formatDateTime(https.expiresAt, locale) : t("certificateMissing")} />
          <UpdateValue
            label={t("automaticRenewal")}
            value={https?.autoRenewal.enabled ? t("autoRenewalEnabled") : t("autoRenewalNotDetected")}
            hint={https?.autoRenewal.lastCheckedAt
              ? t("autoRenewalLastCheck", { time: formatDateTime(https.autoRenewal.lastCheckedAt, locale), status: renewalStatusLabel(https.autoRenewal.lastStatus, t) })
              : https?.autoRenewal.enabled ? (https.autoRenewal.method === "systemd" ? t("autoRenewalSystemd") : t("autoRenewalUpdater")) : t("autoRenewalNotDetectedHint")}
          />
        </div>
      ) : (
        <form className="px-5 py-5 md:px-6" onSubmit={(event) => { event.preventDefault(); if (domainValid && !jobRunning) setSetupConfirmOpen(true); }}>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="text-sm text-slate-300"><span className="mb-1.5 block">{t("httpsDomain")}</span><Input className="w-full font-mono" placeholder="panel.example.com" value={domain} onChange={(event) => setDomain(event.target.value)} /></label>
            <label className="text-sm text-slate-300"><span className="mb-1.5 block">{t("httpsEmail")}</span><Input className="w-full" placeholder="admin@example.com" type="email" value={email} onChange={(event) => setEmail(event.target.value)} /></label>
          </div>
          <div className="mt-4 flex flex-col gap-3 border-t border-panel-line pt-4 sm:flex-row sm:items-center sm:justify-between">
            <p className={cn("max-w-2xl text-xs leading-5", data?.capabilities.httpsSetup ? "text-slate-500" : "text-panel-gold")}>{data?.capabilities.httpsSetup ? t("httpsSetupHint") : t("httpsDriverUnsupported")}</p>
            <Button type="submit" disabled={!data?.capabilities.httpsSetup || !domainValid || setup.isPending || jobRunning}><ShieldCheck className="size-4" />{setup.isPending ? t("httpsConfiguring") : t("configureHTTPS")}</Button>
          </div>
        </form>
      )}
      {data?.job?.status && (data.job.kind === "https-setup" || data.job.kind === "https-renew") ? <OperationStatus job={data.job} /> : null}
    </Card>
    <ConfirmDialog
      busy={setup.isPending}
      cancelLabel={t("cancel")}
      confirmLabel={t("confirmConfigureHTTPS")}
      confirmVariant="primary"
      description={t("httpsSetupConfirmDescription", { domain: domain.trim() })}
      eyebrow={t("httpsTitle")}
      eyebrowTone="gold"
      open={setupConfirmOpen}
      title={t("configureHTTPS")}
      onCancel={() => !setup.isPending && setSetupConfirmOpen(false)}
      onConfirm={() => setup.mutate()}
    />
    </>
  );
}

function OperationStatus({ job }: { job: { status?: string; kind?: string; message?: string } }) {
  const { t } = useI18n();
  const running = job.status === "running";
  const failed = job.status === "failed";
  return (
    <div className={cn("border-t border-panel-line px-5 py-3 text-xs md:px-6", failed ? "text-red-300" : running ? "text-panel-gold" : "text-slate-500")}>
      <div className="flex items-center gap-2">
        <span className={cn("size-1.5 rounded-full", failed ? "bg-red-400" : running ? "animate-pulse bg-panel-gold" : "bg-panel-green")} />
        <span>{running ? t("operationRunning") : failed ? t("operationFailed") : t("operationCompleted")}</span>
      </div>
      {failed && job.message ? (
        <details className="mt-2 pl-3.5 text-slate-400">
          <summary className="cursor-pointer select-none font-medium hover:text-slate-200">{t("viewErrorDetails")}</summary>
          <pre className="mt-2 max-h-44 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950/70 p-3 font-mono text-[11px] leading-5 text-red-200">{job.message}</pre>
        </details>
      ) : null}
    </div>
  );
}

function deploymentServiceLabel(name: string, t: ReturnType<typeof useI18n>["t"]) {
  const labels: Record<string, string> = { updater: t("serviceUpdater"), api: "API", web: "Web", nginx: "Nginx", "gamepanel-exporter": "GamePanel Exporter", prometheus: "Prometheus", cadvisor: "cAdvisor", "node-exporter": "Node Exporter" };
  return labels[name] ?? name;
}

function deploymentStateLabel(state: string, t: ReturnType<typeof useI18n>["t"]) {
  return state === "missing" ? t("statusMissing") : state === "unavailable" ? t("unavailable") : state === "exited" || state === "stopped" ? t("statusStopped") : state;
}

function formatDateTime(value: string, locale: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function renewalStatusLabel(status: string | undefined, t: ReturnType<typeof useI18n>["t"]) {
  return status === "success" ? t("renewalStatusSuccess") : status === "failed" ? t("renewalStatusFailed") : status === "running" ? t("renewalStatusRunning") : t("renewalStatusPending");
}

function PanelUpdateCard({ onNotice }: { onNotice: (notice: { message: string; tone: "success" | "error" } | null) => void }) {
  const { locale, t } = useI18n();
  const queryClient = useQueryClient();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const update = useQuery({
    queryKey: ["system-update"],
    queryFn: getSystemUpdateStatus,
    retry: false,
    refetchInterval: (query) => query.state.data?.job?.status === "running" ? 3_000 : 60_000
  });
  const check = useMutation({
    mutationFn: checkSystemUpdate,
    onSuccess: (data) => queryClient.setQueryData(["system-update"], data),
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("panelUpdateCheckFailed"), tone: "error" })
  });
  const preference = useMutation({
    mutationFn: updateSystemAutoCheck,
    onSuccess: (data) => queryClient.setQueryData(["system-update"], data),
    onError: () => onNotice({ message: t("panelUpdatePreferenceFailed"), tone: "error" })
  });
  const install = useMutation({
    mutationFn: (version: string) => applySystemUpdate(version),
    onSuccess: (job) => {
      queryClient.setQueryData(["system-update"], (current: typeof data) => current ? { ...current, job } : current);
      setConfirmOpen(false);
      onNotice({ message: t("panelUpdateQueued"), tone: "success" });
    },
    onError: (error) => onNotice({ message: error instanceof Error ? error.message : t("panelUpdateCheckFailed"), tone: "error" })
  });

  const data = update.data;
  const latestVersion = data?.latest?.version ?? "—";
  const jobRunning = data?.job?.status === "running";
  const updateJobRunning = jobRunning && data?.job?.kind === "update";
  const status = data?.checkError
    ? { label: t("panelUpdateCheckFailed"), className: "bg-red-500/12 text-red-300" }
    : data?.updateAvailable
      ? { label: t("panelUpdateAvailable"), className: "bg-panel-gold/12 text-panel-gold" }
      : data?.checkedAt
        ? { label: t("panelUpdateUpToDate"), className: "bg-panel-green/12 text-panel-green" }
        : null;
  const checkedAt = data?.checkedAt
    ? new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(new Date(data.checkedAt))
    : t("panelUpdateNeverChecked");

  return (
    <>
      <Card className="mt-5 overflow-hidden">
        <div className="flex flex-col gap-3 border-b border-panel-line px-5 py-4 sm:flex-row sm:items-start sm:justify-between md:px-6">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-semibold text-white">{t("panelUpdateTitle")}</h2>
              {status ? <Badge className={status.className}>{status.label}</Badge> : null}
            </div>
            <p className="mt-1 max-w-3xl text-sm text-slate-400">{t("panelUpdateDescription")}</p>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button type="button" variant="secondary" disabled={check.isPending || jobRunning} onClick={() => check.mutate()}>
              <RefreshCw aria-hidden="true" className={cn("size-4", check.isPending && "animate-spin")} />
              {check.isPending ? t("panelUpdateChecking") : t("panelUpdateCheck")}
            </Button>
            {data?.updateAvailable ? (
              <Button type="button" disabled={!data.updaterAvailable || jobRunning} onClick={() => setConfirmOpen(true)}>
                <Download aria-hidden="true" className="size-4" />
                {updateJobRunning ? t("panelUpdateInstalling") : t("panelUpdateInstall")}
              </Button>
            ) : null}
          </div>
        </div>

        <div className="grid border-b border-panel-line sm:grid-cols-3">
          <UpdateValue label={t("panelUpdateCurrentVersion")} value={data?.current.version ?? "—"} />
          <UpdateValue label={t("panelUpdateLatestVersion")} value={latestVersion} />
          <UpdateValue label={t("panelUpdateLastChecked")} value={checkedAt} />
        </div>

        <div className="flex flex-col gap-4 px-5 py-4 sm:flex-row sm:items-center sm:justify-between md:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <button
              aria-checked={Boolean(data?.autoCheckEnabled)}
              aria-label={t("panelUpdateAutoCheck")}
              className={cn(
                "relative h-6 w-11 shrink-0 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-50",
                data?.autoCheckEnabled ? "bg-panel-green" : "bg-slate-700"
              )}
              disabled={!data || preference.isPending}
              role="switch"
              type="button"
              onClick={() => data && preference.mutate(!data.autoCheckEnabled)}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "absolute left-1 top-1 size-4 rounded-full bg-white transition-transform",
                  data?.autoCheckEnabled && "translate-x-5"
                )}
              />
            </button>
            <div className="min-w-0">
              <p className="text-sm font-medium text-slate-200">{t("panelUpdateAutoCheck")}</p>
              <p className="mt-0.5 text-xs text-slate-500">{t("panelUpdateAutoCheckHint", { hours: data?.intervalHours ?? 24 })}</p>
            </div>
          </div>
          {data?.latest?.releaseNotesUrl ? (
            <a className="inline-flex shrink-0 items-center gap-1.5 text-sm text-panel-green hover:underline" href={data.latest.releaseNotesUrl} rel="noreferrer" target="_blank">
              {t("panelUpdateReleaseNotes")}<ExternalLink aria-hidden="true" className="size-3.5" />
            </a>
          ) : null}
        </div>
        {data?.updateAvailable && !data.updaterAvailable ? (
          <div className="border-t border-panel-line px-5 py-3 text-sm text-panel-gold md:px-6">{t("panelUpdateUpdaterUnavailable")}</div>
        ) : null}
      </Card>

      <ConfirmDialog
        busy={install.isPending}
        cancelLabel={t("cancel")}
        confirmLabel={install.isPending ? t("panelUpdateInstalling") : t("panelUpdateConfirm")}
        confirmVariant="primary"
        description={t("panelUpdateDialogDescription")}
        eyebrow={t("panelUpdateDialogEyebrow")}
        eyebrowTone="green"
        open={confirmOpen}
        title={t("panelUpdateDialogTitle", { version: latestVersion })}
        onCancel={() => !install.isPending && setConfirmOpen(false)}
        onConfirm={() => data?.latest?.version && install.mutate(data.latest.version)}
      />
    </>
  );
}

function UpdateValue({ hint, label, value }: { hint?: string; label: string; value: string }) {
  return (
    <div className="min-w-0 border-b border-panel-line px-5 py-4 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0 md:px-6">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate font-mono text-sm font-medium text-slate-200" title={value}>{value}</p>
      {hint ? <p className="mt-1 truncate text-xs text-slate-500" title={hint}>{hint}</p> : null}
    </div>
  );
}

function SettingRow({ badge, children, description, label }: { badge?: string; children: ReactNode; description: string; label: string }) {
  return (
    <div className="grid gap-4 border-b border-panel-line px-5 py-5 last:border-b-0 md:grid-cols-[minmax(220px,0.75fr)_minmax(360px,1.25fr)] md:gap-8 md:px-6">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-sm font-medium text-slate-200">{label}</h3>
          {badge ? <Badge className="bg-panel-gold/12 text-panel-gold">{badge}</Badge> : null}
        </div>
        <p className="mt-1 max-w-md text-sm leading-6 text-slate-500">{description}</p>
      </div>
      <div className="flex min-w-0 md:justify-end">{children}</div>
    </div>
  );
}

function RegionOption({ checked, description, label, name, onChange, value }: { checked: boolean; description: string; label: string; name: string; onChange: () => void; value: string }) {
  return (
    <label className={cn(
      "relative flex min-h-16 cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 transition",
      "hover:border-slate-600 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-panel-green/50",
      checked ? "border-panel-green/65 bg-panel-green/8" : "border-panel-line bg-slate-950/35"
    )}>
      <input className="sr-only" type="radio" checked={checked} name={name} value={value} onChange={onChange} />
      <span className={cn(
        "flex size-5 shrink-0 items-center justify-center rounded-full border",
        checked ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-600"
      )}>
        {checked ? <Check aria-hidden="true" className="size-3.5" strokeWidth={3} /> : null}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium text-slate-100">{label}</span>
        <span className="mt-0.5 block truncate text-xs text-slate-500">{description}</span>
      </span>
    </label>
  );
}

function validatePublicHost(value: string, message: string) {
  if (!value) return "";
  if (value.length > 253 || /[\s/]/.test(value) || value.includes("://")) return message;
  return "";
}

function formatRegistrySource(registry: string, region: ImageRegion) {
  const normalized = registry.trim().replace(/^https?:\/\//, "").replace(/\/$/, "");
  if (region === "global" && normalized && !normalized.includes(".")) {
    return `docker.io/${normalized}`;
  }
  return normalized;
}
