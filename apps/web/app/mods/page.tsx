"use client";

import { Package, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteMod, listMods, listServers, uploadMod } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import type { ModFile } from "@/lib/types";

export default function ModsPage() {
  const { locale, t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const moddedServers = useMemo(() => (serversQuery.data ?? []).filter((server) => server.mode === "tmodloader"), [serversQuery.data]);
  const [selectedServerId, setSelectedServerId] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModFile | null>(null);
  const activeServerId = selectedServerId || moddedServers[0]?.id || "";
  const modsQuery = useQuery({ queryKey: ["mods", activeServerId], queryFn: () => listMods(activeServerId), enabled: Boolean(activeServerId), retry: false });

  const upload = useMutation({
    mutationFn: (file: File) => uploadMod(activeServerId, file),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modUploaded"));
      await client.invalidateQueries({ queryKey: ["mods", activeServerId] });
      if (inputRef.current) inputRef.current.value = "";
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

  return (
    <>
      <PageHeader
        title={t("modsTitle")}
        description={t("modsDescription")}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <select
              className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
              value={activeServerId}
              onChange={(event) => setSelectedServerId(event.target.value)}
              disabled={moddedServers.length === 0}
            >
              {moddedServers.length === 0 ? <option>{t("noTmodServers")}</option> : moddedServers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
            </select>
            <input
              ref={inputRef}
              className="hidden"
              type="file"
              accept=".tmod,.txt,.json"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) upload.mutate(file);
              }}
            />
            <Button variant="secondary" onClick={() => inputRef.current?.click()} disabled={!activeServerId || upload.isPending}>
              <Upload aria-hidden="true" />
              {upload.isPending ? t("uploading") : t("uploadMod")}
            </Button>
          </div>
        }
      />
      {serversQuery.isError && <p className="mb-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <Card className="p-6 text-sm text-slate-400">
        {t("supportedModFiles")}
      </Card>
      <div className="mt-4 grid gap-3">
        {(modsQuery.data ?? []).map((item) => (
          <Card key={item.id} className="flex items-center justify-between gap-4 p-4">
            <div>
              <h2 className="font-semibold text-white">{item.fileName}</h2>
              <p className="mt-1 text-sm text-slate-400">{item.size} · {item.enabled ? t("enabled") : t("disabled")} · {localizeRelativeTime(item.created, locale)}</p>
            </div>
            <Button
              variant="danger"
              onClick={() => setPendingDelete(item)}
              disabled={remove.isPending}
            >
              <Trash2 aria-hidden="true" />
              {t("delete")}
            </Button>
          </Card>
        ))}
        {activeServerId && !modsQuery.isLoading && (modsQuery.data ?? []).length === 0 && (
          <Card className="flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400">
            <div>
              <Package aria-hidden="true" className="mx-auto" />
              <p className="mt-2 text-sm">{t("noModsUploaded")}</p>
            </div>
          </Card>
        )}
      </div>
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
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        busy={remove.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => pendingDelete && remove.mutate(pendingDelete.id)}
      />
    </>
  );
}
