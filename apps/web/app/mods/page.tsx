"use client";

import { CheckCircle2, Library, Package, Power, ServerIcon, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card } from "@/components/ui";
import { assignMod, deleteGlobalMod, deleteMod, listGlobalMods, listMods, listServers, setModEnabled, uploadGlobalMod, uploadMod } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { describeResourceAction } from "@/lib/server-detail-actions";
import { cn } from "@/lib/utils";
import type { ModFile } from "@/lib/types";

export default function ModsPage() {
  const { locale, t } = useI18n();
  const globalInputRef = useRef<HTMLInputElement>(null);
  const serverInputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const moddedServers = useMemo(() => (serversQuery.data ?? []).filter((server) => server.mode === "tmodloader"), [serversQuery.data]);
  const [selectedServerId, setSelectedServerId] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModFile | null>(null);
  const activeServerId = selectedServerId || moddedServers[0]?.id || "";
  const activeServer = useMemo(() => moddedServers.find((server) => server.id === activeServerId), [activeServerId, moddedServers]);
  const modAction = describeResourceAction({ kind: "modifyMods", serverStatus: activeServer?.status });
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modsQuery = useQuery({ queryKey: ["mods", activeServerId], queryFn: () => listMods(activeServerId), enabled: Boolean(activeServerId), retry: false });

  const globalUpload = useMutation({
    mutationFn: (file: File) => uploadGlobalMod(file),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modUploaded"));
      await client.invalidateQueries({ queryKey: ["global-mods"] });
      if (globalInputRef.current) globalInputRef.current.value = "";
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableUploadMod"));
    }
  });
  const serverUpload = useMutation({
    mutationFn: (file: File) => uploadMod(activeServerId, file),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modUploaded"));
      await client.invalidateQueries({ queryKey: ["mods", activeServerId] });
      if (serverInputRef.current) serverInputRef.current.value = "";
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableUploadMod"));
    }
  });
  const remove = useMutation({
    mutationFn: ({ serverId, modId }: { serverId: string; modId: string }) => deleteMod(serverId, modId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["mods", activeServerId] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteMod"));
    }
  });
  const removeGlobal = useMutation({
    mutationFn: (modId: string) => deleteGlobalMod(modId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["global-mods"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteMod"));
    }
  });
  const toggle = useMutation({
    mutationFn: ({ serverId, modId, enabled }: { serverId: string; modId: string; enabled: boolean }) => setModEnabled(serverId, modId, enabled),
    onSuccess: async (mod) => {
      setErrorMessage("");
      setSuccessMessage(mod.enabled ? t("modEnabled") : t("modDisabled"));
      await client.invalidateQueries({ queryKey: ["mods", activeServerId] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableUpdateMod"));
    }
  });
  const assign = useMutation({
    mutationFn: ({ modId }: { modId: string }) => assignMod(modId, activeServerId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modAssigned"));
      await client.invalidateQueries({ queryKey: ["mods", activeServerId] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableAssignMod"));
    }
  });
  const globalMods = globalModsQuery.data ?? [];
  const serverMods = modsQuery.data ?? [];

  return (
    <>
      <PageHeader
        title={t("modsTitle")}
        description={t("modsDescription")}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <input
              ref={globalInputRef}
              className="hidden"
              type="file"
              accept=".tmod"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) globalUpload.mutate(file);
              }}
            />
            <Button variant="secondary" onClick={() => globalInputRef.current?.click()} disabled={globalUpload.isPending}>
              <Upload aria-hidden="true" />
              {globalUpload.isPending ? t("uploading") : t("uploadMod")}
            </Button>
          </div>
        }
      />
      {(serversQuery.isError || globalModsQuery.isError || modsQuery.isError) && (
        <p className="mb-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p>
      )}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <Card className="p-4 text-sm text-slate-400">
        {t("supportedModFiles")}
      </Card>

      <section className="mt-6">
        <PanelHeading icon={<Library aria-hidden="true" />} title={t("modLibrary")} hint={t("modLibraryHint")} count={globalMods.length} />
        <div className="mt-3 grid gap-3 xl:grid-cols-2">
          {globalMods.map((item) => (
            <Card key={item.id} className="p-4 transition hover:border-panel-purple/40">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <ModIdentity item={item} detail={`${item.size} · ${localizeRelativeTime(item.created, locale)}`} />
                <Badge className="shrink-0 bg-panel-purple/15 text-panel-purple">.tmod</Badge>
              </div>
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-panel-line pt-3">
                <span className="text-xs text-slate-500">{moddedServers.length > 0 ? t("assignToServer") : t("noTmodServers")}</span>
                <div className="flex shrink-0 flex-wrap gap-2">
                  {moddedServers.length > 0 && (
                    <Button
                      variant="secondary"
                      onClick={() => assign.mutate({ modId: item.id })}
                      disabled={assign.isPending || !activeServerId || modAction.disabled}
                      title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}
                    >
                      <ServerIcon aria-hidden="true" />
                      {t("assignToServer")}
                    </Button>
                  )}
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={removeGlobal.isPending}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
          {!globalModsQuery.isLoading && globalMods.length === 0 && (
            <Card className="flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
              <div>
                <Package aria-hidden="true" className="mx-auto" />
                <p className="mt-2 text-sm">{t("noGlobalMods")}</p>
              </div>
            </Card>
          )}
        </div>
      </section>

      {moddedServers.length > 0 && (
        <section className="mt-6">
          <div className="mb-3 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
            <PanelHeading icon={<ServerIcon aria-hidden="true" />} title={t("serverMods")} hint={t("serverModsHint")} count={serverMods.length} />
            <div className="flex flex-wrap items-center gap-2">
              <select
                className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
                value={activeServerId}
                onChange={(event) => setSelectedServerId(event.target.value)}
              >
                {moddedServers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
              </select>
              <Button
                variant="secondary"
                onClick={() => serverInputRef.current?.click()}
                disabled={!activeServerId || serverUpload.isPending || modAction.disabled}
                title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}
              >
                <Upload aria-hidden="true" />
                {serverUpload.isPending ? t("uploading") : t("uploadMod")}
              </Button>
            </div>
            <input
              ref={serverInputRef}
              className="hidden"
              type="file"
              accept=".tmod"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) serverUpload.mutate(file);
              }}
            />
          </div>
          {modAction.reasonKey ? <p className="mb-3 text-sm text-panel-gold">{t(modAction.reasonKey)}</p> : null}
          <div className="grid gap-3 xl:grid-cols-2">
            {serverMods.map((item) => (
              <Card key={item.id} className={cn("p-4 transition", item.enabled ? "border-panel-purple/35" : "opacity-75")}>
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <ModIdentity item={item} detail={`${item.size} · ${localizeRelativeTime(item.created, locale)}`} />
                  <Badge className={cn("shrink-0", item.enabled ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>
                    {item.enabled ? t("enabled") : t("disabled")}
                  </Badge>
                </div>
                <p className="mt-3 text-xs text-slate-500">{t("modAppliesAfterRestart")}</p>
                <div className="mt-4 flex flex-wrap justify-end gap-2 border-t border-panel-line pt-3">
                  <Button
                    variant="secondary"
                    onClick={() => toggle.mutate({ serverId: item.instanceId, modId: item.id, enabled: !item.enabled })}
                    disabled={toggle.isPending || modAction.disabled}
                    title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}
                  >
                    <Power aria-hidden="true" />
                    {item.enabled ? t("disable") : t("enable")}
                  </Button>
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={remove.isPending || modAction.disabled} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </Card>
            ))}
            {activeServerId && !modsQuery.isLoading && serverMods.length === 0 && (
              <Card className="flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
                <div>
                  <Package aria-hidden="true" className="mx-auto" />
                  <p className="mt-2 text-sm">{t("noModsUploaded")}</p>
                </div>
              </Card>
            )}
          </div>
        </section>
      )}

      {moddedServers.length === 0 && globalMods.length > 0 && (
        <Card className="mt-4 flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400">
          <div>
            <Package aria-hidden="true" className="mx-auto" />
            <p className="mt-2 text-sm">{t("noTmodServers")}</p>
          </div>
        </Card>
      )}

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfirm", { name: pendingDelete?.fileName ?? "" })}
        description={t("confirmDeleteModDescription", { name: pendingDelete?.fileName ?? "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("modsTitle")}: </span>
            <span className="font-medium text-white">{pendingDelete.fileName}</span>
          </>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={(pendingDelete?.instanceId === "unassigned" ? removeGlobal : remove).isPending ? t("actionWorking") : t("delete")}
        busy={(pendingDelete?.instanceId === "unassigned" ? removeGlobal : remove).isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          if (!pendingDelete) return;
          if (pendingDelete.instanceId === "unassigned") {
            removeGlobal.mutate(pendingDelete.id);
            return;
          }
          remove.mutate({ serverId: pendingDelete.instanceId, modId: pendingDelete.id });
        }}
      />
    </>
  );
}

function PanelHeading({ count, hint, icon, title }: { count: number; hint: string; icon: ReactNode; title: string }) {
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-panel-purple">
          {icon}
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-base font-semibold text-white">{title}</h2>
            <Badge className="bg-slate-800 text-slate-300">{count}</Badge>
          </div>
          <p className="mt-1 text-sm text-slate-500">{hint}</p>
        </div>
      </div>
    </div>
  );
}

function ModIdentity({ detail, item }: { detail: string; item: ModFile }) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-panel-line bg-slate-950/55 text-panel-purple">
        <Package aria-hidden="true" className="size-5" />
      </span>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <h3 className="truncate font-semibold text-white">{item.fileName}</h3>
          {item.enabled && <CheckCircle2 aria-hidden="true" className="size-4 shrink-0 text-panel-green" />}
        </div>
        <p className="mt-1 truncate text-sm text-slate-400">{detail}</p>
      </div>
    </div>
  );
}
