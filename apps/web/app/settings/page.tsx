"use client";

import { useMutation } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { Database, Globe, KeyRound, ShieldCheck } from "lucide-react";
import { useState, type FormEvent } from "react";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { changeAdminPassword, getDockerStatus, getSettings, updatePublicHost } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [passwordMessage, setPasswordMessage] = useState("");
  const [publicHost, setPublicHost] = useState("");
  const [publicHostMessage, setPublicHostMessage] = useState("");
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false, refetchInterval: 5000 });
  const settings = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });
  const configuredHost = settings.data?.dockerHost ?? docker.data?.host ?? "GAMEPANEL_DOCKER_HOST";
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
    mutationFn: () => updatePublicHost(publicHost),
    onSuccess: () => {
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

        <Card className="p-5">
          <div className="flex items-start gap-3">
            <Globe className="mt-1 shrink-0 text-panel-green" aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <h2 className="font-semibold">{t("publicHostTitle")}</h2>
              <p className="mt-2 text-sm text-slate-400">{t("publicHostDescription")}</p>
              <form className="mt-5 space-y-3" onSubmit={submitPublicHost}>
                <label className="block">
                  <span className="text-xs font-medium text-slate-500">{t("publicHostTitle")}</span>
                  <Input
                    className="mt-2 w-full"
                    placeholder={t("publicHostPlaceholder")}
                    value={publicHost || (settings.data?.publicHost ?? "")}
                    onChange={(event) => setPublicHost(event.target.value)}
                  />
                </label>
                {publicHostMessage ? <p className="text-sm text-slate-400">{publicHostMessage}</p> : null}
                <Button className="w-full" type="submit" disabled={publicHostMutation.isPending}>
                  {publicHostMutation.isPending ? t("saving") : t("saveButton")}
                </Button>
              </form>
            </div>
          </div>
        </Card>

        <Card className="p-5">
          <div className="flex items-start gap-3">
            <KeyRound className="mt-1 shrink-0 text-panel-green" aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <h2 className="font-semibold">{t("localAdmin")}</h2>
              <p className="mt-2 text-sm text-slate-400">{t("localAdminDescription")}</p>
              <form className="mt-5 space-y-3" onSubmit={submitPasswordChange}>
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
                {passwordMessage ? <p className="text-sm text-slate-400">{passwordMessage}</p> : null}
                <Button className="w-full" type="submit" disabled={passwordMutation.isPending}>
                  {passwordMutation.isPending ? t("saving") : t("changePassword")}
                </Button>
              </form>
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
