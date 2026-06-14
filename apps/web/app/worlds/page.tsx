"use client";

import { Download, Plus, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteWorld, duplicateWorld, importWorld, listWorlds, worldDownloadUrl } from "@/lib/api";
import { localizeDifficulty, localizeRelativeTime, localizeWorldSize, useI18n } from "@/lib/i18n";

export default function WorldsPage() {
  const { locale, t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const worlds = query.data ?? [];

  const upload = useMutation({
    mutationFn: (file: File) => importWorld(file),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["worlds"] });
      inputRef.current!.value = "";
    },
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableImportWorld"))
  });
  const duplicate = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => duplicateWorld(id, name),
    onSuccess: async () => client.invalidateQueries({ queryKey: ["worlds"] }),
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableDuplicateWorld"))
  });
  const remove = useMutation({
    mutationFn: deleteWorld,
    onSuccess: async () => client.invalidateQueries({ queryKey: ["worlds"] }),
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableDeleteWorld"))
  });

  return (
    <AppShell>
      <PageHeader
        title={t("worldsTitle")}
        description={t("worldsDescription")}
        action={
          <>
            <input
              ref={inputRef}
              className="hidden"
              type="file"
              accept=".wld"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) upload.mutate(file);
              }}
            />
            <Button variant="secondary" onClick={() => inputRef.current?.click()} disabled={upload.isPending}>
              <Upload aria-hidden="true" />
              {upload.isPending ? t("importing") : t("importWorld")}
            </Button>
          </>
        }
      />
      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p>}
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {worlds.map((world) => (
          <Card key={world.id} className="p-4">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="truncate font-semibold">{world.name}</h2>
                <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-300">
                  <span className="rounded bg-slate-800 px-2 py-1">{localizeWorldSize(world.size, locale)}</span>
                  <span className="rounded bg-slate-800 px-2 py-1">{localizeDifficulty(world.difficulty, locale)}</span>
                </div>
              </div>
              {world.server && <span className="shrink-0 rounded bg-panel-green/15 px-2 py-1 text-xs text-panel-green">{t("inUse")}</span>}
            </div>
            <p className="mt-4 text-sm text-slate-400">{t("modified")}: {localizeRelativeTime(world.modified, locale)}</p>
            <p className="text-sm text-slate-400">{t("usedBy")}: {world.server || t("notInUse")}</p>
            <p className="text-sm text-slate-400">{t("size")}: {world.bytes}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Button
                variant="secondary"
                onClick={() => duplicate.mutate({ id: world.id, name: `${world.name} ${t("duplicateSuffix")}` })}
                disabled={duplicate.isPending || query.isError}
              >
                <Plus aria-hidden="true" />
                {t("duplicate")}
              </Button>
              <a href={worldDownloadUrl(world.id)}>
                <Button variant="secondary" disabled={query.isError}>
                  <Download aria-hidden="true" />
                  {t("download")}
                </Button>
              </a>
              <Button
                variant="danger"
                onClick={() => {
                  if (window.confirm(t("deleteWorldConfirm", { name: world.name }))) remove.mutate(world.id);
                }}
                disabled={remove.isPending || query.isError}
              >
                <Trash2 aria-hidden="true" />
                {t("delete")}
              </Button>
            </div>
          </Card>
        ))}
        <Card className="flex min-h-52 items-center justify-center border-dashed p-4 text-slate-400">
          <button className="text-center" type="button" onClick={() => inputRef.current?.click()}>
            <Plus aria-hidden="true" className="mx-auto" />
            <p className="mt-2 text-sm">{t("importNewWorld")}</p>
          </button>
        </Card>
      </div>
      {!query.isLoading && worlds.length === 0 && <p className="mt-4 text-sm text-slate-400">{t("noWorldsYet")}</p>}
    </AppShell>
  );
}
