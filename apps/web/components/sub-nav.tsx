"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Archive, Bookmark, Box, Gamepad2, Globe2, PackageCheck, Settings } from "lucide-react";
import type { ComponentType } from "react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export interface SubNavItem {
  href: string;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  badge?: string;
}

export function SubNav({ items, className }: { items: SubNavItem[]; className?: string }) {
  const pathname = usePathname();

  return (
    <div className={cn("flex items-center gap-1.5 overflow-x-auto border-b border-slate-800/80 pb-3 mb-6 scrollbar-none", className)}>
      {items.map((item) => {
        const Icon = item.icon;
        const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href + "/"));

        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium transition whitespace-nowrap",
              isActive
                ? "bg-slate-800 text-slate-100 shadow-xs ring-1 ring-white/10 font-semibold"
                : "text-slate-400 hover:bg-slate-900/60 hover:text-slate-200"
            )}
          >
            {Icon ? <Icon className={cn("size-3.5", isActive ? "text-panel-green" : "text-slate-400")} /> : null}
            <span>{item.label}</span>
            {item.badge ? (
              <span className="rounded-full bg-slate-700/80 px-1.5 py-0.2 text-[10px] font-mono text-slate-300">
                {item.badge}
              </span>
            ) : null}
          </Link>
        );
      })}
    </div>
  );
}

export function GameAssetsSubNav() {
  const { locale } = useI18n();
  const isZh = locale === "zh";

  const items: SubNavItem[] = [
    { href: "/games", label: isZh ? "游戏服务端库" : "Game Library", icon: Gamepad2 },
    { href: "/mods", label: isZh ? "创意工坊模组" : "Mod Workshop", icon: Box },
    { href: "/presets", label: isZh ? "配置预设模版" : "Presets", icon: Bookmark },
    { href: "/worlds", label: isZh ? "世界地图存档" : "Worlds", icon: Globe2 },
    { href: "/backups", label: isZh ? "快照与备份中心" : "Backups", icon: Archive }
  ];

  return <SubNav items={items} />;
}

export function SettingsSubNav() {
  const { locale } = useI18n();
  const isZh = locale === "zh";

  const items: SubNavItem[] = [
    { href: "/settings", label: isZh ? "控制台基本设置" : "General Settings", icon: Settings },
    { href: "/versions", label: isZh ? "系统版本与更新" : "Version & Updates", icon: PackageCheck }
  ];

  return <SubNav items={items} />;
}
