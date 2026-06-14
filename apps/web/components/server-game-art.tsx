"use client";

import Image from "next/image";
import { Hammer } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { ServerMode } from "@/lib/types";

const gameArt = {
  terraria: {
    src: "/images/terraria-official-cover.jpg",
    accent: "from-panel-green/30 via-slate-950/0 to-slate-950/45"
  }
} as const;

export function ServerGameArt({ mode, className }: { mode: ServerMode; className?: string }) {
  const { t } = useI18n();
  const art = gameArt.terraria;
  const modded = mode === "tmodloader";

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
      {modded && (
        <span
          aria-label="tModLoader"
          className="absolute bottom-1 right-1 flex size-5 items-center justify-center rounded bg-purple-400/95 text-slate-950 shadow-[0_0_0_1px_rgba(255,255,255,0.18)]"
          title="tModLoader"
        >
          <Hammer aria-hidden="true" className="size-3" />
        </span>
      )}
    </div>
  );
}
