"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Package, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteModPack, listModPacks } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";

export default function ModPackDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const client = useQueryClient();
  const id = params.id;
  const packsQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const [pendingDelete, setPendingDelete] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const pack = useMemo(() => (packsQuery.data ?? []).find((item) => item.id === id), [id, packsQuery.data]);
  const remove = useMutation({
    mutationFn: deleteModPack,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["mod-packs"] });
      router.push("/mods");
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableDeleteModPack"))
  });

  if (packsQuery.isLoading) {
    return <p className="text-sm text-slate-400">{t("loading")}</p>;
  }

  if (packsQuery.isError || !pack) {
    return (
      <>
        <BackLink />
        <Card className="p-6">
          <p className="text-sm text-panel-gold">{packsQuery.isError ? t("modsApiUnavailable") : t("modPackNotFound")}</p>
        </Card>
      </>
    );
  }

  return (
    <>
      <BackLink />
      <PageHeader title={pack.name} description={t("modPackDetailDescription")} />
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-11 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                <Package aria-hidden="true" className="size-5" />
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold text-white">{pack.name}</h2>
                <p className="mt-1 text-sm text-slate-500">{pack.description || t("modPacksHint")}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <DetailTile label={t("modPacks")} value={pack.name} />
              <DetailTile label={t("modsTitle")} value={String(pack.mods.length)} />
              <DetailTile label={t("created")} value={pack.created} />
              <DetailTile label={t("type")} value={t("modPacks")} />
            </div>
          </Card>

          <Card className="p-4">
            <h2 className="font-semibold">{t("modLibrary")}</h2>
            <div className="mt-4 grid gap-2">
              {pack.mods.map((mod) => (
                <Link key={mod.id} href={`/mods/${mod.id}`} className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60">
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-slate-100">{modDisplayName(mod, locale)}</span>
                    <span className="mt-0.5 block truncate text-xs text-slate-500">{mod.size}</span>
                  </span>
                  <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
                </Link>
              ))}
              {pack.mods.length === 0 && <p className="text-sm text-slate-500">{t("noGlobalMods")}</p>}
            </div>
          </Card>
        </div>

        <Card className="h-fit p-4">
          <h2 className="font-semibold">{t("actions")}</h2>
          <Button className="mt-4 w-full" variant="danger" onClick={() => setPendingDelete(true)} disabled={remove.isPending}>
            <Trash2 aria-hidden="true" />
            {t("delete")}
          </Button>
        </Card>
      </div>

      <ConfirmDialog
        open={pendingDelete}
        eyebrow={t("destructiveAction")}
        title={t("deleteModPackConfirm", { name: pack.name })}
        description={t("confirmDeleteModPackDescription")}
        detail={<DetailLine label={t("modPacks")} value={pack.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        confirmVariant="danger"
        busy={remove.isPending}
        onCancel={() => setPendingDelete(false)}
        onConfirm={() => remove.mutate(pack.id)}
      />
    </>
  );
}

function BackLink() {
  const { t } = useI18n();
  return (
    <Link href="/mods" className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-slate-400 transition hover:text-white">
      <ArrowLeft aria-hidden="true" className="size-4" />
      {t("backToMods")}
    </Link>
  );
}

function DetailTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-slate-100">{value}</p>
    </div>
  );
}

function DetailLine({ label, value }: { label: string; value: string }) {
  return (
    <>
      <span className="text-slate-500">{label}: </span>
      <span className="font-medium text-white">{value}</span>
    </>
  );
}
