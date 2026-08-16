"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, Globe2, RotateCcw, Save } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card, Input, ToastNotice } from "@/components/ui";
import { getSettings, updateImageRegion, updatePublicHost } from "@/lib/api";
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
    </>
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
