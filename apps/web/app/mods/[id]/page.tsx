"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQueries, useQuery } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Package } from "lucide-react";
import { useMemo } from "react";
import { PageHeader } from "@/components/page-header";
import { Badge, Card } from "@/components/ui";
import { listGlobalMods, listModPacks, listMods, listServers } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { modDisplayName, modSourceLabel } from "@/lib/mod-display";
import type { ModFile, Server } from "@/lib/types";

type ModSource = {
  mod: ModFile;
  server?: Server;
  scope: "library" | "server";
};

export default function ModDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const packsQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const tmodServers = useMemo(() => (serversQuery.data ?? []).filter((server) => server.mode === "tmodloader"), [serversQuery.data]);
  const serverModQueries = useQueries({
    queries: tmodServers.map((server) => ({
      queryKey: ["mods", server.id],
      queryFn: () => listMods(server.id),
      retry: false,
      enabled: serversQuery.isSuccess
    }))
  });
  const sources = useMemo<ModSource[]>(() => {
    const librarySource = (globalModsQuery.data ?? []).find((mod) => mod.id === id);
    const serverSources = serverModQueries.flatMap((query, index) => {
      const server = tmodServers[index];
      return (query.data ?? []).filter((mod) => mod.id === id).map((mod) => ({ mod, server, scope: "server" as const }));
    });
    return librarySource ? [{ mod: librarySource, scope: "library" }, ...serverSources] : serverSources;
  }, [globalModsQuery.data, id, serverModQueries, tmodServers]);
  const source = sources[0];
  const relatedPacks = useMemo(() => (packsQuery.data ?? []).filter((pack) => pack.modIds.includes(id)), [id, packsQuery.data]);
  const loading = globalModsQuery.isLoading || serversQuery.isLoading || serverModQueries.some((query) => query.isLoading);
  const errored = globalModsQuery.isError || serversQuery.isError || serverModQueries.some((query) => query.isError);

  if (loading) {
    return <p className="text-sm text-slate-400">{t("loading")}</p>;
  }

  if (errored || !source) {
    return (
      <>
        <BackLink />
        <Card className="p-6">
          <p className="text-sm text-panel-gold">{errored ? t("modsApiUnavailable") : t("modNotFound")}</p>
        </Card>
      </>
    );
  }

  return (
    <>
      <BackLink />
      <PageHeader title={modDisplayName(source.mod, locale)} description={t("modDetailDescription")} />
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-11 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                <Package aria-hidden="true" className="size-5" />
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold text-white">{modDisplayName(source.mod, locale)}</h2>
                <p className="mt-1 truncate text-sm text-slate-500">{modSourceLabel(source.mod, locale)}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <DetailTile label={t("size")} value={source.mod.size} />
              <DetailTile label={t("created")} value={localizeRelativeTime(source.mod.created, locale)} />
              <DetailTile label={t("type")} value={source.scope === "library" ? t("platformLibrary") : t("serverMods")} />
              <DetailTile label={t("type")} value={source.mod.enabled ? t("enabled") : t("disabled")} />
            </div>
          </Card>

          <Card className="p-4">
            <h2 className="font-semibold">{t("relatedMods")}</h2>
            <div className="mt-4 grid gap-2">
              {sources.map((item) => (
                <div key={`${item.scope}-${item.server?.id ?? "library"}`} className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-slate-100">{item.scope === "library" ? t("platformLibrary") : item.server?.name}</p>
                    <p className="mt-0.5 text-xs text-slate-500">{item.mod.enabled ? t("enabled") : t("disabled")}</p>
                  </div>
                  {item.server && (
                    <Link href={`/servers/${item.server.id}`} className="shrink-0 text-sm font-medium text-panel-green hover:underline">
                      {t("manageOnServer")}
                    </Link>
                  )}
                </div>
              ))}
            </div>
          </Card>
        </div>

        <div className="space-y-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("modPacks")}</h2>
            <div className="mt-4 space-y-2">
              {relatedPacks.map((pack) => (
                <Link key={pack.id} href={`/mods/packs/${pack.id}`} className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60">
                  <span className="truncate text-sm font-medium text-slate-100">{pack.name}</span>
                  <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
                </Link>
              ))}
              {!packsQuery.isLoading && relatedPacks.length === 0 && <p className="text-sm text-slate-500">{t("noModPacks")}</p>}
            </div>
          </Card>
          <Card className="p-4">
            <h2 className="font-semibold">{t("type")}</h2>
            <Badge className="mt-4 bg-slate-800 text-slate-300">{source.scope === "library" ? t("platformLibrary") : t("installedOnServer")}</Badge>
          </Card>
        </div>
      </div>
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
