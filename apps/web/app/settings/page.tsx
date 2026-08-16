"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Download, ExternalLink, Globe2, RefreshCw, RotateCcw, Save } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card, Input, ToastNotice } from "@/components/ui";
import {
  applySystemUpdate,
  checkSystemUpdate,
  getSettings,
  getSystemUpdateStatus,
  updateImageRegion,
  updatePublicHost,
  updateSystemAutoCheck
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type ImageRegion = "global" | "cn";

export default function SettingsPage() {
  const { t } = useI18n();
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const [publicHost, setPublicHost] = useState<string | null>(null);
  const [imageRegion, setImageRegion] = useState<ImageRegion | null>(null);
  const [notice, setNotice] = useState<{ message: string; tone: "success" | "error" } | null>(null);

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

      <form onSubmit={submit}>
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
      </form>

      <PanelUpdateCard onNotice={setNotice} />
    </>
  );
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
                {jobRunning ? t("panelUpdateInstalling") : t("panelUpdateInstall")}
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

function UpdateValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 border-b border-panel-line px-5 py-4 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0 md:px-6">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate font-mono text-sm font-medium text-slate-200" title={value}>{value}</p>
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
