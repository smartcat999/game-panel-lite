"use client";

import Link from "next/link";
import { Download, FileArchive, Server as ServerIcon, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { Button, Card } from "@/components/ui";
import { deleteWorld, downloadWorldFile, listGameServers, listGames, listWorlds } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameFilterOptions, gameKeyFromProvider } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import type { MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { getWorldSourceServerId } from "@/lib/server-detail-resources";
import type { GameServerResource } from "@/lib/types";
import type { World } from "@/lib/types";

type PendingWorldAction =
  | { kind: "download"; world: World }
  | { kind: "delete"; world: World };

type WorldGameFilter = "all" | string;
type WorldProviderFilter = "all" | string;

function worldModeLabel(world: World, vanillaLabel: string) {
  if (world.providerKey === "terraria-tmodloader") return "tModLoader";
  return vanillaLabel;
}

function serversUsingWorld(world: World, servers: GameServerResource[]) {
  return servers.filter((server) => server.spec.sourceWorldId === world.id);
}

export default function WorldsPage() {
  if (!showWorldAndBackupFeatures) return <HiddenFeaturePage />;
  return <EnabledWorldsPage />;
}

function EnabledWorldsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingAction, setPendingAction] = useState<PendingWorldAction | null>(null);
  const [downloadingWorldId, setDownloadingWorldId] = useState("");
  const [gameFilter, setGameFilter] = useState<WorldGameFilter>("all");
  const [providerFilter, setProviderFilter] = useState<WorldProviderFilter>("all");
  const [search, setSearch] = useState("");
  const worlds = query.data ?? [];
  const servers = serversQuery.data ?? [];
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), worlds.map((world) => world.gameKey ?? gameKeyFromProvider(world.providerKey)), t),
    [gamesQuery.data, t, worlds]
  );
  const providerFilters = useMemo(
    () => providerFilterOptions(gamesQuery.data ?? [], t("filterAll"), worlds.map((world) => world.providerKey), gameFilter),
    [gameFilter, gamesQuery.data, t, worlds]
  );
  useEffect(() => {
    if (providerFilter !== "all" && !providerFilters.some((option) => option.key === providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerFilters]);
  const filteredWorlds = useMemo(() => {
    const term = search.trim().toLowerCase();
    return worlds.filter((world) => {
      const usingServers = serversUsingWorld(world, servers);
      const matchesSearch = !term || [world.name, world.size, worldModeLabel(world, "vanilla"), ...usingServers.map((server) => server.name)].some((value) => value.toLowerCase().includes(term));
      const worldGame = world.gameKey ?? gameKeyFromProvider(world.providerKey);
      const matchesGame = gameFilter === "all" || worldGame === gameFilter;
      const matchesProvider = providerFilter === "all" || world.providerKey === providerFilter;
      return matchesSearch && matchesGame && matchesProvider;
    });
  }, [gameFilter, providerFilter, search, servers, worlds]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : ""
  ].filter(Boolean);
  const remove = useMutation({
    mutationFn: deleteWorld,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("worldDeleted"));
      setPendingAction(null);
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      const message = error instanceof Error ? error.message : "";
      setErrorMessage(message.includes("world template is used") ? t("unableDeleteWorldInUse") : message.includes("active world") ? t("unableDeleteActiveWorld") : message || t("unableDeleteWorld"));
    }
  });

  const downloadWorld = async (world: World) => {
    setDownloadingWorldId(world.id);
    setErrorMessage("");
    setSuccessMessage("");
    try {
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
      setSuccessMessage(t("downloadStarted"));
      setPendingAction(null);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : t("unableDownloadWorld"));
    } finally {
      setDownloadingWorldId("");
    }
  };
  const pendingBusy = Boolean(
    pendingAction && (
      (pendingAction.kind === "download" && downloadingWorldId === pendingAction.world.id) ||
      (pendingAction.kind === "delete" && remove.isPending)
    )
  );

  const confirmPendingAction = () => {
    if (!pendingAction) return;
    if (pendingAction.kind === "download") {
      void downloadWorld(pendingAction.world);
      return;
    }
    if (pendingAction.kind === "delete") {
      remove.mutate(pendingAction.world.id);
    }
  };

  return (
    <>
      <PageHeader title={t("worldsTitle")} />
      <ResourceFilterBar
        activeChips={activeFilterChips}
        clearLabel={t("clearFilters")}
        density="compact"
        filters={[
          { label: t("filterGame"), options: gameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) },
          { label: t("filterType"), options: providerFilters, value: providerFilter, onChange: (value) => setProviderFilter(value) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setProviderFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        resultLabel={t("filteredResultsCount", { count: filteredWorlds.length })}
        search={search}
        searchPlaceholder={t("searchWorlds")}
      />
      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {filteredWorlds.map((world) => {
          const sourceServerId = getWorldSourceServerId(world);
          const sourceServerName = sourceServerId ? serverNameById.get(sourceServerId) ?? sourceServerId : "";
          const usingServers = serversUsingWorld(world, servers);
          return (
            <Card key={world.id} className="group p-4 transition hover:border-panel-green/40">
              <div className="flex items-start justify-between gap-4">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="relative flex size-10 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                    <FileArchive aria-hidden="true" className="size-5" />
                  </span>
                  <div className="min-w-0">
                    <Link href={`/worlds/${world.id}`} className="block min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
                      <h2 className="truncate font-semibold text-white transition group-hover:text-panel-green">{world.name}</h2>
                    </Link>
                  </div>
                </div>
                <span className={usingServers.length > 0 ? "shrink-0 rounded bg-panel-green/15 px-2 py-1 text-xs font-medium text-panel-green" : "shrink-0 rounded bg-slate-800 px-2 py-1 text-xs font-medium text-slate-400"}>
                  {usingServers.length > 0 ? t("inUse") : t("notInUse")}
                </span>
              </div>

              <div className="mt-4 space-y-2 text-sm">
                <div className="flex items-center gap-2">
                  <span className="w-20 shrink-0 text-xs text-slate-500">{t("serverType")}</span>
                  <span className="truncate font-medium text-slate-100">{worldModeLabel(world, t("modeVanilla"))}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-20 shrink-0 text-xs text-slate-500">{t("sourceServer")}</span>
                  {sourceServerId ? (
                    <Link href={`/servers/${sourceServerId}`} className="truncate font-medium text-panel-green hover:underline">
                      {sourceServerName}
                    </Link>
                  ) : (
                    <span className="truncate text-slate-500">{t("imported")}</span>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-20 shrink-0 text-xs text-slate-500">{t("activeServer")}</span>
                  {usingServers.length > 0 ? (
                    <Link href={`/worlds/${world.id}`} className="truncate font-medium text-panel-green hover:underline">
                      {t("usingServersCount", { count: usingServers.length })}
                    </Link>
                  ) : (
                    <span className="truncate text-slate-500">{t("notInUse")}</span>
                  )}
                </div>
              </div>

              <p className="mt-3 truncate text-sm text-slate-400">
                {world.bytes} · {t("modified")}: {localizeRelativeTime(world.modified, locale)}
              </p>

              <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-panel-line pt-3">
                <Link
                  className="inline-flex h-8 items-center gap-2 rounded-md border border-panel-green/40 bg-panel-green/10 px-3 text-sm font-medium text-panel-green transition hover:bg-panel-green/15 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                  href={`/servers/new?worldId=${encodeURIComponent(world.id)}`}
                >
                  <ServerIcon aria-hidden="true" className="size-4" />
                  <span>{t("createServerFromWorld")}</span>
                </Link>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    className="h-8 px-2 text-xs"
                    variant="ghost"
                    onClick={() => setPendingAction({ kind: "download", world })}
                    disabled={query.isError || downloadingWorldId === world.id}
                    aria-label={downloadingWorldId === world.id ? t("downloading") : t("download")}
                    title={downloadingWorldId === world.id ? t("downloading") : t("download")}
                  >
                    <Download aria-hidden="true" className="size-4" />
                  </Button>
                  <Button
                    className="h-8 px-2 text-xs text-red-200 hover:bg-red-500/15"
                    variant="ghost"
                    onClick={() => setPendingAction({ kind: "delete", world })}
                    disabled={remove.isPending || query.isError}
                    aria-label={t("delete")}
                    title={t("delete")}
                  >
                    <Trash2 aria-hidden="true" className="size-4" />
                  </Button>
                </div>
              </div>
            </Card>
          );
        })}
      </div>
      {!query.isLoading && filteredWorlds.length === 0 && <p className="mt-4 text-sm text-slate-400">{worlds.length === 0 ? t("noWorldsYet") : t("noWorldsMatch")}</p>}
      {pendingAction && (
        <ConfirmDialog
          open={Boolean(pendingAction)}
          eyebrow={pendingAction.kind === "delete" ? t("destructiveAction") : t("confirmActionEyebrow")}
          title={
            pendingAction.kind === "download"
                ? t("downloadWorldConfirm", { name: pendingAction.world.name })
                : t("deleteWorldConfirm", { name: pendingAction.world.name })
          }
          description={
            pendingAction.kind === "download"
                ? t("confirmDownloadWorldDescription", { name: pendingAction.world.name })
                : t("confirmDeleteWorldDescription", { name: pendingAction.world.name })
          }
          detail={(
            <div className="space-y-1">
              <p>
                <span className="text-slate-500">{t("world")}: </span>
                <span className="font-medium text-white">{pendingAction.world.name}</span>
              </p>
            </div>
          )}
          cancelLabel={t("cancel")}
          confirmLabel={
            pendingBusy
              ? t("actionWorking")
              : pendingAction.kind === "download"
                  ? t("download")
                  : t("delete")
          }
          confirmVariant={pendingAction.kind === "delete" ? "danger" : "gold"}
          busy={pendingBusy}
          onCancel={() => setPendingAction(null)}
          onConfirm={confirmPendingAction}
        />
      )}
    </>
  );
}

function HiddenFeaturePage() {
  return (
    <Card className="p-6">
      <h1 className="text-xl font-semibold text-white">Page not found</h1>
      <p className="mt-2 text-sm text-slate-400">The requested GamePanel Lite page does not exist.</p>
      <Link className="mt-4 inline-flex text-sm font-medium text-panel-green hover:underline" href="/dashboard">
        Back to dashboard
      </Link>
    </Card>
  );
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey, params?: Record<string, string | number>) => string
) {
  const option = options.find((item) => item.key === value);
  if (!option) return value;
  return option.labelKey ? t(option.labelKey) : option.label ?? value;
}
