"use client";

import { Package, Power, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { assignMod, deleteGlobalMod, deleteMod, listGlobalMods, listMods, listServers, setModEnabled, uploadGlobalMod, uploadMod } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
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
    mutationFn: (modId: string) => deleteMod(activeServerId, modId),
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
    mutationFn: ({ modId, enabled }: { modId: string; enabled: boolean }) => setModEnabled(activeServerId, modId, enabled),
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
              accept=".tmod,.txt,.json"
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
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <Card className="p-6 text-sm text-slate-400">
        {t("supportedModFiles")}
      </Card>

      {globalMods.length > 0 && (
        <div className="mt-6">
          <h2 className="mb-3 text-base font-semibold">{t("modLibrary")}</h2>
          <div className="grid gap-3">
            {globalMods.map((item) => (
              <Card key={item.id} className="flex items-center justify-between gap-4 p-4">
                <div>
                  <h3 className="font-semibold text-white">{item.fileName}</h3>
                  <p className="mt-1 text-sm text-slate-400">{item.size} · {localizeRelativeTime(item.created, locale)}</p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  {moddedServers.length > 0 && (
                    <Button
                      variant="secondary"
                      onClick={() => assign.mutate({ modId: item.id })}
                      disabled={assign.isPending || !activeServerId}
                    >
                      {t("assignToServer")}
                    </Button>
                  )}
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={removeGlobal.isPending}>
                    <Trash2 aria-hidden="true" />
                    {t("delete")}
                  </Button>
                </div>
              </Card>
            ))}
          </div>
        </div>
      )}

      {moddedServers.length > 0 && (
        <div className="mt-6">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <h2 className="text-base font-semibold">{t("serverMods")}</h2>
            <select
              className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
              value={activeServerId}
              onChange={(event) => setSelectedServerId(event.target.value)}
            >
              {moddedServers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
            </select>
            <input
              ref={serverInputRef}
              className="hidden"
              type="file"
              accept=".tmod,.txt,.json"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) serverUpload.mutate(file);
              }}
            />
            <Button variant="secondary" onClick={() => serverInputRef.current?.click()} disabled={!activeServerId || serverUpload.isPending}>
              <Upload aria-hidden="true" />
              {serverUpload.isPending ? t("uploading") : t("uploadMod")}
            </Button>
          </div>
          <div className="grid gap-3">
            {serverMods.map((item) => (
              <Card key={item.id} className="flex items-center justify-between gap-4 p-4">
                <div>
                  <h3 className="font-semibold text-white">{item.fileName}</h3>
                  <p className="mt-1 text-sm text-slate-400">{item.size} · {item.enabled ? t("enabled") : t("disabled")} · {localizeRelativeTime(item.created, locale)}</p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button
                    variant="secondary"
                    onClick={() => toggle.mutate({ modId: item.id, enabled: !item.enabled })}
                    disabled={toggle.isPending}
                  >
                    <Power aria-hidden="true" />
                    {item.enabled ? t("disable") : t("enable")}
                  </Button>
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={remove.isPending}>
                    <Trash2 aria-hidden="true" />
                    {t("delete")}
                  </Button>
                </div>
              </Card>
            ))}
            {activeServerId && !modsQuery.isLoading && serverMods.length === 0 && (
              <Card className="flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400">
                <div>
                  <Package aria-hidden="true" className="mx-auto" />
                  <p className="mt-2 text-sm">{t("noModsUploaded")}</p>
                </div>
              </Card>
            )}
          </div>
        </div>
      )}

      {moddedServers.length === 0 && globalMods.length === 0 && (
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
        onConfirm={() => pendingDelete && (pendingDelete.instanceId === "unassigned" ? removeGlobal : remove).mutate(pendingDelete.id)}
      />
    </>
  );
}
