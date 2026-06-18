import { Bird, Blocks, Flame, Leaf, type LucideIcon } from "lucide-react";

export type GameArtConfig = {
  /** Image src when available, otherwise uses gradient + icon */
  imageSrc?: string;
  /** Tailwind gradient classes for the art background */
  gradient: string;
  /** Icon for non-image art */
  icon: LucideIcon;
  /** Alt label for accessibility */
  alt: string;
};

const terrariaArt: GameArtConfig = {
  imageSrc: "/images/terraria-official-cover.jpg",
  gradient: "from-panel-green/30 via-slate-950/0 to-slate-950/45",
  icon: Leaf,
  alt: "Terraria",
};

export const gameArtConfig: Record<string, GameArtConfig> = {
  terraria: terrariaArt,
  "terraria-vanilla": terrariaArt,
  "terraria-tmodloader": terrariaArt,
  palworld: {
    imageSrc: "/images/palworld-cover.jpg",
    gradient: "from-sky-500/45 via-sky-900/20 to-slate-950/50",
    icon: Bird,
    alt: "Palworld",
  },
  "dont-starve-together": {
    imageSrc: "/images/dst-cover.jpg",
    gradient: "from-amber-500/45 via-orange-900/20 to-slate-950/50",
    icon: Flame,
    alt: "Don't Starve Together",
  },
  minecraft: {
    imageSrc: "/images/minecraft-cover.jpg",
    gradient: "from-emerald-600/45 via-green-900/20 to-slate-950/50",
    icon: Blocks,
    alt: "Minecraft",
  },
};

export function getGameArt(key: string | undefined): GameArtConfig {
  return gameArtConfig[key ?? ""] ?? {
    gradient: "from-panel-green/25 via-slate-950/0 to-slate-950/45",
    icon: Leaf,
    alt: "Game",
  };
}
