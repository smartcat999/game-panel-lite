"use client";

import Image from "next/image";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Download, Loader2, Plus } from "lucide-react";
import { Button, Card } from "@/components/ui";
import { PageHeader } from "@/components/page-header";
import { getGameArt } from "@/lib/game-art";
import { gameDescription, gameDisplayName } from "@/lib/game-display";
import { useI18n } from "@/lib/i18n";
import { listGames, prepareRuntimeImage } from "@/lib/api";
import { providerDescription, providerDisplayName } from "@/lib/provider-display";
import { isRuntimeImagePreparing, isRuntimeImageReady, runtimeImageLabelKey, runtimeImageTone } from "@/lib/runtime-image";
import { cn } from "@/lib/utils";
import type { GameCatalogEntry, ProviderCatalog, ProviderKey, RuntimeImageStatus } from "@/lib/types";

export default function GamesPage() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const gamesQuery = useQuery({
    queryKey: ["games"],
    queryFn: listGames,
    retry: false,
    refetchInterval: (query) => hasPreparingRuntime(query.state.data) ? 1000 : false
  });
  const install = useMutation({
    mutationFn: ({ providerKey, version }: { providerKey: ProviderKey; version?: string }) => prepareRuntimeImage(providerKey, version),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["games"] });
    }
  });
  const games = gamesQuery.data ?? [];

  return (
    <>
      <PageHeader title={t("gameLibraryTitle")} description={t("gameLibraryDescription")} />
      {gamesQuery.isError && <p className="mb-4 text-sm text-panel-gold">{t("dockerStatusUnavailable")}</p>}
      <div className="grid gap-4">
        {games.map((game) => (
          <GameRuntimeCard
            key={game.key}
            game={game}
            installError={install.error instanceof Error ? install.error.message : ""}
            installingProvider={install.variables?.providerKey}
            isInstalling={install.isPending}
            onInstall={(provider) => install.mutate({ providerKey: provider.key, version: provider.recommendedVersion })}
          />
        ))}
      </div>
      {gamesQuery.isLoading && <p className="mt-4 text-sm text-slate-400">{t("loading")}</p>}
    </>
  );
}

function GameRuntimeCard({
  game,
  installError,
  installingProvider,
  isInstalling,
  onInstall
}: {
  game: GameCatalogEntry;
  installError: string;
  installingProvider?: ProviderKey;
  isInstalling: boolean;
  onInstall: (provider: ProviderCatalog) => void;
}) {
  const { t } = useI18n();
  const art = getGameArt(game.coverImage ?? game.key);
  const Icon = art.icon;
  return (
    <Card className="overflow-hidden p-0">
      <div className="grid gap-0 lg:grid-cols-[260px_1fr]">
        <div className="relative min-h-44 overflow-hidden border-b border-panel-line bg-slate-950 lg:border-b-0 lg:border-r">
          <div className={cn("absolute inset-0 bg-gradient-to-br", art.gradient)} />
          {art.imageSrc ? (
            <>
              <Image
                src={art.imageSrc}
                alt={art.alt}
                fill
                sizes="(min-width: 1024px) 260px, 100vw"
                className="object-cover object-center"
              />
              <div className={cn("absolute inset-0 bg-gradient-to-br opacity-55", art.gradient)} />
            </>
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-white/65">
              <Icon aria-hidden="true" className="size-16" />
            </div>
          )}
        </div>
        <div className="p-4 md:p-5">
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-white">{gameDisplayName(game.key, game.name, t)}</h2>
              <p className="mt-1 max-w-3xl text-sm text-slate-400">{gameDescription(game.key, game.description, t)}</p>
            </div>
            <span className="w-fit rounded border border-panel-line bg-slate-950/60 px-2.5 py-1 text-xs text-slate-400">
              {t("gameLibraryProviderCount", { count: game.providers.length })}
            </span>
          </div>
          <div className="mt-4 divide-y divide-panel-line overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
            {game.providers.map((provider) => (
              <ProviderRuntimeRow
                key={provider.key}
                game={game}
                installError={installError}
                installingProvider={installingProvider}
                isInstalling={isInstalling}
                onInstall={onInstall}
                provider={provider}
              />
            ))}
            {game.providers.length === 0 && <p className="p-4 text-sm text-slate-500">{t("plannedGameHint")}</p>}
          </div>
        </div>
      </div>
    </Card>
  );
}

function ProviderRuntimeRow({
  game,
  installError,
  installingProvider,
  isInstalling,
  onInstall,
  provider
}: {
  game: GameCatalogEntry;
  installError: string;
  installingProvider?: ProviderKey;
  isInstalling: boolean;
  onInstall: (provider: ProviderCatalog) => void;
  provider: ProviderCatalog;
}) {
  const { t } = useI18n();
  const status = provider.runtimeImage;
  const ready = isRuntimeImageReady(status);
  const preparing = isRuntimeImagePreparing(status) || (isInstalling && installingProvider === provider.key);
  const unsupported = status?.status === "unsupported";
  const failed = status?.status === "failed";
  const displayStatus: RuntimeImageStatus | undefined = preparing
    ? { image: status?.image ?? provider.key, message: status?.message, progress: status?.progress, status: "preparing", updatedAt: status?.updatedAt }
    : status;
  const progress = preparing ? normalizedRuntimeProgress(displayStatus?.progress) : 0;
  const statusHint = preparing
    ? progress > 0
      ? t("gameLibraryInstallProgress", { progress })
      : t("gameLibraryInstallStarting")
    : ready
      ? t("gameLibraryReadyHint")
      : t("gameLibraryInstallHint");
  return (
    <div className="flex flex-col gap-4 p-4 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-slate-100">{providerDisplayName(provider.key, provider.name, t)}</p>
          <RuntimeStatusBadge status={displayStatus} />
          {provider.recommended && <span className="rounded bg-panel-green/15 px-2 py-0.5 text-xs text-panel-green">{t("recommended")}</span>}
        </div>
        <p className="mt-1 max-w-2xl text-sm text-slate-400">{providerDescription(provider.key, provider.description, t)}</p>
        <p className="mt-2 text-xs text-slate-500">{statusHint}</p>
        {preparing ? <RuntimeInstallProgress progress={progress} /> : null}
        {failed && status?.message ? <p className="mt-2 text-xs text-panel-gold">{status.message}</p> : null}
        {failed && installError ? <p className="mt-1 text-xs text-panel-gold">{installError}</p> : null}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {ready ? (
          <Link
            href={`/servers/new?game=${encodeURIComponent(game.key)}&provider=${encodeURIComponent(provider.key)}`}
            className="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-panel-green px-4 text-sm font-semibold text-slate-950 transition hover:bg-panel-green/90 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
          >
            <Plus aria-hidden="true" className="size-4" />
            {t("gameLibraryCreate")}
          </Link>
        ) : (
          <Button
            type="button"
            variant={failed ? "secondary" : "primary"}
            className="h-10"
            disabled={preparing || unsupported}
            onClick={() => onInstall(provider)}
          >
            {preparing ? <Loader2 aria-hidden="true" className="size-4 animate-spin" /> : <Download aria-hidden="true" className="size-4" />}
            {preparing ? t("gameLibraryInstalling") : t("gameLibraryInstall")}
          </Button>
        )}
      </div>
    </div>
  );
}

function RuntimeInstallProgress({ progress }: { progress: number }) {
  const hasProgress = progress > 0;
  return (
    <div className="mt-3 h-1.5 w-full max-w-2xl overflow-hidden rounded-full bg-slate-800/80">
      <div
        className={cn(
          "h-full rounded-full bg-sky-300 transition-all duration-500",
          hasProgress ? "" : "w-1/3 animate-pulse"
        )}
        style={hasProgress ? { width: `${progress}%` } : undefined}
      />
    </div>
  );
}

function RuntimeStatusBadge({ status }: { status?: RuntimeImageStatus }) {
  const { t } = useI18n();
  const tone = runtimeImageTone(status);
  const Icon = status?.status === "ready" ? CheckCircle2 : status?.status === "preparing" ? Loader2 : AlertTriangle;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium",
        tone === "success" && "bg-panel-green/15 text-panel-green",
        tone === "info" && "bg-sky-500/15 text-sky-300",
        tone === "warning" && "bg-panel-gold/15 text-panel-gold",
        tone === "neutral" && "bg-slate-800 text-slate-400"
      )}
    >
      <Icon aria-hidden="true" className={cn("size-3.5", status?.status === "preparing" && "animate-spin")} />
      {t(runtimeImageLabelKey(status))}
    </span>
  );
}

function hasPreparingRuntime(games?: GameCatalogEntry[]) {
  return Boolean(games?.some((game) => game.providers.some((provider) => provider.runtimeImage?.status === "preparing")));
}

function normalizedRuntimeProgress(progress?: number) {
  if (typeof progress !== "number" || Number.isNaN(progress)) return 0;
  return Math.max(0, Math.min(100, Math.round(progress)));
}
