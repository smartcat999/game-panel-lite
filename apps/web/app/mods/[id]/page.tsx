"use client";

import Image from "next/image";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQueries, useQuery } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Clock3, ExternalLink, Eye, Heart, Package, Users } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { PageHeader } from "@/components/page-header";
import { Badge, Card } from "@/components/ui";
import { listGameServers, listGames, listGlobalMods, listModPacks, listMods } from "@/lib/api";
import { gameServerMode } from "@/lib/game-server-resource";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { modDisplayName, modSourceLabel } from "@/lib/mod-display";
import type { GameServerResource, ModFile } from "@/lib/types";

type ModSource = {
  mod: ModFile;
  server?: GameServerResource;
  scope: "library" | "server";
};

export default function ModDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, staleTime: 5 * 60 * 1000, retry: false });
  const packsQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const modProviderKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const game of gamesQuery.data ?? []) {
      for (const provider of game.providers) {
        if (provider.capabilities.mods) {
          keys.add(provider.key);
        }
      }
    }
    return keys;
  }, [gamesQuery.data]);
  const modCapableServers = useMemo(
    () => (serversQuery.data ?? []).filter((server) => modProviderKeys.has(server.providerKey) || gameServerMode(server) === "tmodloader"),
    [modProviderKeys, serversQuery.data]
  );
  const serverModQueries = useQueries({
    queries: modCapableServers.map((server) => ({
      queryKey: ["mods", server.id],
      queryFn: () => listMods(server.id),
      retry: false,
      enabled: serversQuery.isSuccess && (gamesQuery.isSuccess || gameServerMode(server) === "tmodloader")
    }))
  });
  const allSources = useMemo<ModSource[]>(() => {
    const librarySources = (globalModsQuery.data ?? []).map((mod) => ({ mod, scope: "library" as const }));
    const serverSources = serverModQueries.flatMap((query, index) => {
      const server = modCapableServers[index];
      return (query.data ?? []).map((mod) => ({ mod, server, scope: "server" as const }));
    });
    return [...librarySources, ...serverSources];
  }, [globalModsQuery.data, serverModQueries, modCapableServers]);
  const source = allSources.find((item) => item.mod.id === id);
  const sources = useMemo(
    () => source ? allSources.filter((item) => sameModIdentity(source.mod, item.mod)) : [],
    [allSources, source]
  );
  const relatedPacks = useMemo(() => (packsQuery.data ?? []).filter((pack) => pack.modIds.includes(id)), [id, packsQuery.data]);
  const loading = globalModsQuery.isLoading || serversQuery.isLoading || gamesQuery.isLoading || serverModQueries.some((query) => query.isLoading);
  const errored = globalModsQuery.isError || serversQuery.isError || gamesQuery.isError || serverModQueries.some((query) => query.isError);

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
      <PageHeader
        title={modDisplayName(source.mod, locale)}
        description={source.mod.workshopId ? `${modSourceLabel(source.mod, locale)} ${source.mod.workshopId}` : t("modDetailDescription")}
      />
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="overflow-hidden p-0">
            <div className="flex flex-col gap-5 p-5 sm:flex-row">
              <div className="flex size-28 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-panel-line bg-slate-950/70 text-panel-green">
                {source.mod.previewUrl ? (
                  <Image src={source.mod.previewUrl} alt="" className="size-full object-cover" width={112} height={112} unoptimized />
                ) : (
                  <Package aria-hidden="true" className="size-7" />
                )}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge className="bg-slate-800 text-slate-300">{modSourceLabel(source.mod, locale)}</Badge>
                  <Badge className={source.mod.enabled ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400"}>
                    {source.mod.enabled ? t("enabled") : t("disabled")}
                  </Badge>
                </div>
                {source.mod.description ? (
                  <p className="mt-3 whitespace-pre-line text-sm leading-6 text-slate-300">{sanitizeModDescription(source.mod.description)}</p>
                ) : (
                  <p className="mt-3 text-sm text-slate-500">{t("modDescriptionUnavailable")}</p>
                )}
                {source.mod.tags && source.mod.tags.length > 0 ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {source.mod.tags.map((tag) => <span key={tag} className="rounded bg-slate-900 px-2 py-1 text-xs text-slate-300">{tag}</span>)}
                  </div>
                ) : null}
              </div>
            </div>
            {(source.mod.subscriptions || source.mod.favorited || source.mod.views || source.mod.updatedAtSteam) ? (
              <div className="grid grid-cols-2 border-t border-panel-line sm:grid-cols-4">
                {source.mod.subscriptions ? <Metric icon={<Users />} label={t("workshopSubscriptions")} value={source.mod.subscriptions.toLocaleString()} /> : null}
                {source.mod.favorited ? <Metric icon={<Heart />} label={t("workshopFavorites")} value={source.mod.favorited.toLocaleString()} /> : null}
                {source.mod.views ? <Metric icon={<Eye />} label={t("workshopViews")} value={source.mod.views.toLocaleString()} /> : null}
                {source.mod.updatedAtSteam ? <Metric icon={<Clock3 />} label={t("workshopUpdated")} value={formatSteamDate(source.mod.updatedAtSteam, locale)} /> : null}
              </div>
            ) : null}
          </Card>

          <Card className="p-5">
            <h2 className="font-semibold text-white">{t("modTechnicalDetails")}</h2>
            <dl className="mt-4 grid gap-x-8 border-t border-panel-line sm:grid-cols-2">
              <DetailRow label={t("fileName")} value={source.mod.fileName} />
              <DetailRow label={t("size")} value={source.mod.size} />
              {source.mod.workshopId && <DetailRow label={t("workshopIdLabel")} value={source.mod.workshopId} />}
              {source.mod.modName && <DetailRow label={t("internalModName")} value={source.mod.modName} />}
              {source.mod.modVersion && <DetailRow label={t("modVersion")} value={source.mod.modVersion} />}
              {source.mod.tmodVersion && <DetailRow label={t("tmodVersion")} value={source.mod.tmodVersion} />}
              {source.mod.creatorSteamId && <DetailRow label={t("creatorSteamId")} value={source.mod.creatorSteamId} />}
              <DetailRow label={t("created")} value={localizeRelativeTime(source.mod.created, locale)} />
            </dl>
          </Card>

          <Card className="p-5">
            <h2 className="font-semibold text-white">{t("dependencyRelations")}</h2>
            <div className="mt-4 flex flex-wrap gap-2">
              {source.mod.dependencies && source.mod.dependencies.length > 0 ? source.mod.dependencies.map((dependency) => (
                <span key={dependency} className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2 text-sm text-slate-200">{dependency}</span>
              )) : <p className="text-sm text-slate-500">{t("noDependencies")}</p>}
            </div>
          </Card>
        </div>

        <div className="space-y-4">
          {source.mod.workshopId ? (
            <Card className="p-4">
              <h2 className="font-semibold text-white">{t("modSource")}</h2>
              <a
                href={`https://steamcommunity.com/sharedfiles/filedetails/?id=${source.mod.workshopId}`}
                target="_blank"
                rel="noreferrer"
                className="mt-4 flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 text-sm font-medium text-slate-100 transition hover:border-panel-green/50 hover:text-panel-green"
              >
                <span>{t("openSteamWorkshop")}</span>
                <ExternalLink aria-hidden="true" className="size-4 shrink-0" />
              </a>
            </Card>
          ) : null}
          <Card className="p-4">
            <h2 className="font-semibold text-white">{t("modUsageLocations")}</h2>
            <div className="mt-4 space-y-2">
              {sources.map((item) => (
                <div key={`${item.scope}-${item.server?.id ?? "library"}`} className="flex items-center justify-between gap-3 border-b border-panel-line py-2 last:border-0">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-slate-100">{item.scope === "library" ? t("platformLibrary") : item.server?.name}</p>
                    <p className="mt-0.5 text-xs text-slate-500">{item.mod.enabled ? t("enabled") : t("disabled")}</p>
                  </div>
                  {item.server ? (
                    <Link href={`/servers/${item.server.id}`} className="shrink-0 text-xs font-medium text-panel-green hover:underline">
                      {t("manageOnServer")}
                    </Link>
                  ) : null}
                </div>
              ))}
            </div>
          </Card>
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

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-4 border-b border-panel-line py-3">
      <dt className="shrink-0 text-sm text-slate-500">{label}</dt>
      <dd className="truncate text-right text-sm font-medium text-slate-100" title={value}>{value}</dd>
    </div>
  );
}

function Metric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center gap-3 border-b border-panel-line px-4 py-3 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0">
      <span className="text-slate-500 [&>svg]:size-4">{icon}</span>
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold text-slate-100">{value}</p>
        <p className="truncate text-xs text-slate-500">{label}</p>
      </div>
    </div>
  );
}

function sameModIdentity(left: ModFile, right: ModFile) {
  if (left.providerKey !== right.providerKey) return false;
  if (left.workshopId || right.workshopId) return Boolean(left.workshopId && left.workshopId === right.workshopId);
  if (left.modName || right.modName) return Boolean(left.modName && left.modName === right.modName);
  return left.fileName === right.fileName;
}

function sanitizeModDescription(value: string) {
  return value
    .replace(/\[(\/)?[a-z0-9=:#/.\-_"' ]+\]/gi, "")
    .replace(/https?:\/\/\S+/gi, "")
    .replace(/[ \t]+/g, " ")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function formatSteamDate(timestamp: number, locale: string) {
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "short",
    day: "numeric"
  }).format(new Date(timestamp * 1000));
}
