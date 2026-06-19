"use client";

import { useMutation } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { Database, Globe, HardDrive, KeyRound, Network } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { changeAdminPassword, getSettings, updatePublicHost } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function SettingsPage() {
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [passwordMessage, setPasswordMessage] = useState("");
  const [publicHost, setPublicHost] = useState<string | null>(null);
  const [publicHostMessage, setPublicHostMessage] = useState("");
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const configuredHost = settings.data?.dockerHost ?? "GAMEPANEL_DOCKER_HOST";
  const publicHostValue = publicHost ?? settings.data?.publicHost ?? "";
  const passwordMutation = useMutation({
    mutationFn: () => changeAdminPassword(currentPassword, newPassword),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setPasswordMessage(t("passwordChanged"));
    },
    onError: (err) => setPasswordMessage(err instanceof Error ? err.message : t("passwordChangeFailed"))
  });
  const publicHostMutation = useMutation({
    mutationFn: () => updatePublicHost(publicHostValue),
    onSuccess: () => {
      setPublicHost(null);
      setPublicHostMessage(t("publicHostSaved"));
      settings.refetch();
    },
    onError: (err) => setPublicHostMessage(err instanceof Error ? err.message : t("publicHostSaveFailed"))
  });

  const submitPasswordChange = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPasswordMessage("");
    passwordMutation.mutate();
  };

  const submitPublicHost = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    publicHostMutation.mutate();
  };

  return (
    <>
      <PageHeader title={t("settingsTitle")} description={t("settingsDescription")} />

      <div className="grid gap-5 xl:grid-cols-[220px_minmax(0,1fr)]">
        <Card className="h-fit p-2 xl:sticky xl:top-24">
          <nav aria-label={t("settingsTitle")} className="flex gap-1 overflow-x-auto xl:flex-col xl:overflow-visible">
            <SettingsNavItem href="#docker-host" icon={<Network aria-hidden="true" className="size-4" />} label={t("dockerSockTitle")} />
            <SettingsNavItem href="#network" icon={<Network aria-hidden="true" className="size-4" />} label={t("publicHostTitle")} />
            <SettingsNavItem href="#storage" icon={<HardDrive aria-hidden="true" className="size-4" />} label={t("dataDirectories")} />
            <SettingsNavItem href="#security" icon={<KeyRound aria-hidden="true" className="size-4" />} label={t("localAdmin")} />
          </nav>
        </Card>

        <Card className="overflow-hidden">
          <SettingsSection
            id="docker-host"
            icon={<Network aria-hidden="true" className="size-5 text-slate-400" />}
            title={t("dockerSockTitle")}
            description={t("dockerSockDescription")}
          >
            <div className="grid gap-3">
              <SettingValue label={t("configuredDockerHost")} value={configuredHost} />
            </div>
          </SettingsSection>

          <SettingsSection
            id="network"
            icon={<Globe aria-hidden="true" className="size-5 text-panel-green" />}
            title={t("publicHostTitle")}
            description={t("publicHostDescription")}
          >
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

          <SettingsSection
            id="security"
            icon={<KeyRound aria-hidden="true" className="size-5 text-panel-green" />}
            title={t("localAdmin")}
            description={t("localAdminDescription")}
          >
            <form className="max-w-xl space-y-3" onSubmit={submitPasswordChange}>
              <label className="block">
                <span className="text-xs font-medium text-slate-500">{t("currentPassword")}</span>
                <Input
                  className="mt-2 w-full"
                  type="password"
                  value={currentPassword}
                  onChange={(event) => setCurrentPassword(event.target.value)}
                  autoComplete="current-password"
                />
              </label>
              <label className="block">
                <span className="text-xs font-medium text-slate-500">{t("newPassword")}</span>
                <Input
                  className="mt-2 w-full"
                  type="password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                  autoComplete="new-password"
                />
              </label>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <Button className="w-fit px-4" type="submit" disabled={passwordMutation.isPending}>
                  {passwordMutation.isPending ? t("saving") : t("changePassword")}
                </Button>
                {passwordMessage ? <p className="text-sm text-slate-400">{passwordMessage}</p> : null}
              </div>
            </form>
          </SettingsSection>
        </Card>
      </div>
    </>
  );
}

function SettingsNavItem({ href, icon, label }: { href: string; icon: ReactNode; label: string }) {
  return (
    <a
      className="inline-flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-sm text-slate-400 transition hover:bg-slate-900 hover:text-slate-100 focus:outline-none focus:ring-2 focus:ring-panel-green/40"
      href={href}
    >
      {icon}
      {label}
    </a>
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
    <section className="border-b border-panel-line p-5 last:border-b-0 md:p-6" id={id}>
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
