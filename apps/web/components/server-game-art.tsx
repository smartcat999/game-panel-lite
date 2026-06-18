"use client";

import Image from "next/image";
import { Box, Hammer } from "lucide-react";
import { getGameArt } from "@/lib/game-art";
import { serverProviderDisplay } from "@/lib/server-display";
import { cn } from "@/lib/utils";
import type { Server } from "@/lib/types";

export function ServerGameArt({ server, className }: { server: Pick<Server, "mode" | "providerKey" | "gameKey">; className?: string }) {
  const art = getGameArt(server.gameKey ?? server.providerKey);
  const provider = serverProviderDisplay(server);
  const showProviderMark = provider.label !== "Terraria";
  const Icon = art.icon;

  return (
    <div className={cn("group relative size-16 shrink-0 overflow-hidden rounded-md border border-panel-line bg-slate-950", className)}>
      {art.imageSrc ? (
        <>
          <Image
            src={art.imageSrc}
            alt={art.alt}
            fill
            sizes="64px"
            className="object-cover object-[50%_38%] transition duration-200 group-hover:scale-105"
          />
          <div className={cn("absolute inset-0 bg-gradient-to-br", art.gradient)} />
          <div className="absolute inset-x-0 bottom-0 h-7 bg-gradient-to-t from-slate-950/85 to-transparent" />
        </>
      ) : (
        <>
          <div className={cn("absolute inset-0 bg-gradient-to-br transition duration-200 group-hover:scale-110", art.gradient)} />
          <div className="absolute inset-0 flex items-center justify-center">
            <Icon aria-hidden="true" className="size-8 text-white/85 drop-shadow-lg" />
          </div>
          <div className="absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-slate-950/90 to-transparent" />
        </>
      )}
      {showProviderMark && (
        <span
          aria-label={provider.label}
          className={cn(
            "absolute bottom-1 right-1 flex size-5 items-center justify-center rounded text-slate-950 shadow-[0_0_0_1px_rgba(255,255,255,0.18)]",
            provider.tone === "purple" ? "bg-purple-400/95" : "",
            provider.tone === "sky" ? "bg-sky-300/95" : "",
            provider.tone === "amber" ? "bg-panel-gold/95" : "",
            provider.tone === "green" ? "bg-panel-green/95" : "",
            provider.tone === "slate" ? "bg-slate-300/95" : ""
          )}
          title={provider.label}
        >
          {provider.tone === "purple" ? <Hammer aria-hidden="true" className="size-3" /> : <Box aria-hidden="true" className="size-3" />}
        </span>
      )}
    </div>
  );
}
