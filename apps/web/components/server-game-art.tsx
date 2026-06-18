"use client";

import Image from "next/image";
import { Box, Hammer } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { serverProviderDisplay } from "@/lib/server-display";
import { cn } from "@/lib/utils";
import type { Server } from "@/lib/types";

const gameArt = {
  terraria: {
    src: "/images/terraria-official-cover.jpg",
    accent: "from-panel-green/30 via-slate-950/0 to-slate-950/45"
  }
} as const;

export function ServerGameArt({ server, className }: { server: Pick<Server, "mode" | "providerKey">; className?: string }) {
  const { t } = useI18n();
  const art = gameArt.terraria;
  const provider = serverProviderDisplay(server);
  const showProviderMark = provider.label !== "Terraria";

  return (
    <div className={cn("group relative size-16 shrink-0 overflow-hidden rounded-md border border-panel-line bg-slate-950", className)}>
      <Image
        src={art.src}
        alt={t("terrariaCoverAlt")}
        fill
        sizes="64px"
        className="object-cover object-[50%_38%] transition duration-200 group-hover:scale-105"
      />
      <div className={cn("absolute inset-0 bg-gradient-to-br", art.accent)} />
      <div className="absolute inset-x-0 bottom-0 h-7 bg-gradient-to-t from-slate-950/85 to-transparent" />
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
