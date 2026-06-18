"use client";

import Link from "next/link";
import Image from "next/image";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { getGameArt } from "@/lib/game-art";
import { gameDescription, gameDisplayName } from "@/lib/game-display";
import { listGames } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { isRuntimeImageReady } from "@/lib/runtime-image";
import { cn } from "@/lib/utils";
import type { GameCatalogEntry } from "@/lib/types";

export function GameLibrary() {
  const { t } = useI18n();
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false });
  const games = (gamesQuery.data ?? []).filter((game) => game.status === "available");

  if (games.length === 0) {
    return null;
  }

  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{t("gameLibraryTitle")}</h2>
          <p className="text-xs text-slate-500">{t("gameLibraryDescription")}</p>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {games.map((game) => (
          <GameCard key={game.key} game={game} />
        ))}
      </div>
    </section>
  );
}

function GameCard({ game }: { game: GameCatalogEntry }) {
  const { t } = useI18n();
  const art = getGameArt(game.coverImage ?? game.key);
  const Icon = art.icon;
  const count = game.serverCount ?? 0;
  const readyProviders = game.providers.filter((provider) => isRuntimeImageReady(provider.runtimeImage)).length;

  return (
    <Link
      href="/games"
      className="group relative flex flex-col overflow-hidden rounded-lg border border-panel-line bg-panel-card transition hover:border-panel-green/50 hover:bg-slate-900/70"
    >
      <div className="relative h-24 w-full overflow-hidden bg-slate-950">
        <div className={cn("absolute inset-0 bg-gradient-to-br", art.gradient)} />
        {art.imageSrc ? (
          <>
            <Image
              src={art.imageSrc}
              alt={art.alt}
              fill
              sizes="(min-width: 1280px) 25vw, (min-width: 640px) 50vw, 100vw"
              className="object-cover object-center transition duration-200 group-hover:scale-105"
            />
            <div className={cn("absolute inset-0 bg-gradient-to-br opacity-60", art.gradient)} />
          </>
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-white/60">
            <Icon aria-hidden="true" className="size-12" />
          </div>
        )}
        <div className="absolute inset-x-0 bottom-0 h-10 bg-gradient-to-t from-panel-card to-transparent" />
      </div>
      <div className="flex flex-1 flex-col gap-2 p-3">
        <div className="flex items-start justify-between gap-2">
          <h3 className="text-sm font-semibold text-white">{gameDisplayName(game.key, game.name, t)}</h3>
          <ArrowRight aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-slate-500 transition group-hover:translate-x-0.5 group-hover:text-panel-green" />
        </div>
        <p className="line-clamp-2 text-xs text-slate-400">{gameDescription(game.key, game.description, t)}</p>
        <div className="mt-auto flex items-center gap-2 text-xs text-slate-500">
          <span className="rounded border border-panel-line bg-slate-950/50 px-1.5 py-0.5">
            {t("gameLibraryServers", { count })}
          </span>
          <span className={cn("rounded border px-1.5 py-0.5", readyProviders > 0 ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-line bg-slate-950/50 text-slate-500")}>
            {readyProviders > 0 ? t("gameLibraryInstalled") : t("gameLibraryNotInstalled")}
          </span>
        </div>
      </div>
    </Link>
  );
}
