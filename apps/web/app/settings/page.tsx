"use client";

import { useMutation } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { Database, Network, Package } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { getSettings, updatePublicHost } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function SettingsPage() {
  const { t } = useI18n();
  const [publicHost, setPublicHost] = useState<string | null>(null);
  const [publicHostMessage, setPublicHostMessage] = useState("");
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const configuredHost = settings.data?.dockerHost ?? "GAMEPANEL_DOCKER_HOST";
  const publicHostValue = publicHost ?? settings.data?.publicHost ?? "";
  const imageRegion = settings.data?.imageRegion ?? "global";
  const imageRegistry = settings.data?.imageRegistry ?? "smartcat99999";
  const imageTag = settings.data?.imageTag ?? "v0.1.0";
  const providerCatalogPath = settings.data?.providerCatalogPath ?? "GAMEPANEL_PROVIDER_CATALOG_PATH";
  const publicHostMutation = useMutation({
    mutationFn: () => updatePublicHost(publicHostValue),
    onSuccess: () => {
      setPublicHost(null);
      setPublicHostMessage(t("publicHostSaved"));
      settings.refetch();
    },
    onError: (err) => setPublicHostMessage(err instanceof Error ? err.message : t("publicHostSaveFailed"))
  });

  const submitPublicHost = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    publicHostMutation.mutate();
  };

  return (
    <>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />

      <div className="grid gap-5">
        <SettingsSection
          id="connection"
          icon={<Network aria-hidden="true" className="size-5 text-panel-green" />}
          title={t("connectionSettings")}
          description={t("connectionSettingsDescription")}
        >
          <div className="grid gap-4">
            <SettingValue label={t("configuredDockerHost")} value={configuredHost} />
            <form className="grid gap-3 lg:grid-cols-[minmax(260px,520px)_auto] lg:items-end" onSubmit={submitPublicHost}>
              <label className="block">
                <span className="text-xs font-medium text-slate-500">{t("publicHostTitle")}</span>
                <Input
                  className="mt-2 w-full"
                  placeholder={t("publicHostPlaceholder")}
                  value={publicHostValue}
                  onChange={(event) => setPublicHost(event.target.value)}
                />
              </label>
              <Button className="h-10 px-4" type="submit" disabled={publicHostMutation.isPending}>
                {publicHostMutation.isPending ? t("saving") : t("saveButton")}
              </Button>
              {publicHostMessage ? <p className="text-sm text-slate-400 lg:col-span-2">{publicHostMessage}</p> : null}
            </form>
          </div>
        </SettingsSection>

        <SettingsSection
          id="images"
          icon={<Package aria-hidden="true" className="size-5 text-panel-green" />}
          title={t("imageSourceTitle")}
          description={t("imageSourceDescription")}
        >
          <div className="grid gap-3 md:grid-cols-2">
            <SettingValue label={t("imageRegion")} value={imageRegion} />
            <SettingValue label={t("panelImageTag")} value={imageTag} />
            <SettingValue label={t("imageRegistry")} value={imageRegistry} />
            <SettingValue label={t("providerCatalogPath")} value={providerCatalogPath} />
          </div>
          <p className="mt-3 text-sm text-slate-500">{t("imageSourceHint")}</p>
        </SettingsSection>

        <SettingsSection
          id="storage"
          icon={<Database aria-hidden="true" className="size-5 text-slate-400" />}
          title={t("dataDirectories")}
          description={t("dataDirectoriesDescription")}
        >
          {settings.data ? (
            <div className="grid gap-3">
              <SettingValue label={t("dataDir")} value={settings.data.dataDir} />
              <SettingValue label={t("dbPath")} value={settings.data.dbPath} />
            </div>
          ) : (
            <p className="text-sm text-slate-400">{t("loading")}</p>
          )}
        </SettingsSection>
      </div>
    </>
  );
}

function SettingsSection({
  children,
  description,
  icon,
  id,
  title
}: {
  children: ReactNode;
  description: string;
  icon: ReactNode;
  id: string;
  title: string;
}) {
  return (
    <section id={id}>
      <Card className="p-5 md:p-6">
        <div className="mb-5 flex items-start gap-3">
          <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/45">
            {icon}
          </span>
          <div className="min-w-0">
            <h2 className="font-semibold text-white">{title}</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-400">{description}</p>
          </div>
        </div>
        {children}
      </Card>
    </section>
  );
}

function SettingValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 break-all font-mono text-sm text-slate-200">{value}</p>
    </div>
  );
}
