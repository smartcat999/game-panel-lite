"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Gauge, Gamepad2, HardDrive, Settings } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { listGameServers } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function TopNav() {
  const pathname = usePathname();
  const { locale } = useI18n();
  const isZh = locale === "zh";

  const serversQuery = useQuery({
    queryKey: ["game-servers"],
    queryFn: listGameServers,
    retry: false,
    staleTime: 10000
  });

  const servers = serversQuery.data ?? [];
  const runningCount = servers.filter((s) => s.status?.actualState === "running").length;

  const navItems = [
    {
      href: "/dashboard",
      title: isZh ? "仪表盘" : "Dashboard",
      icon: Gauge,
      active: pathname === "/dashboard"
    },
    {
      href: "/servers",
      title: isZh ? `游戏服务器 (${runningCount} 运行中)` : `Servers (${runningCount} Running)`,
      icon: HardDrive,
      badge: runningCount > 0 ? `${runningCount}` : undefined,
      active: pathname.startsWith("/servers")
    },
    {
      href: "/games",
      title: isZh ? "游戏、模组与世界资产" : "Games, Mods & World Assets",
      icon: Gamepad2,
      active:
        pathname.startsWith("/games") ||
        pathname.startsWith("/mods") ||
        pathname.startsWith("/presets") ||
        pathname.startsWith("/worlds") ||
        pathname.startsWith("/backups")
    },
    {
      href: "/activity",
      title: isZh ? "监控与活动事件" : "Monitoring & Activity",
      icon: Activity,
      active: pathname.startsWith("/activity")
    },
    {
      href: "/settings",
      title: isZh ? "系统、节点与集群设置" : "System & Cluster Settings",
      icon: Settings,
      active: pathname.startsWith("/settings") || pathname.startsWith("/versions")
    }
  ];

  return (
    <nav className="flex items-center gap-1">
      {navItems.map((item) => {
        const Icon = item.icon;
        return (
          <Link
            key={item.href}
            href={item.href}
            title={item.title}
            aria-label={item.title}
            className={cn(
              "group relative flex size-9 items-center justify-center rounded-lg transition-all",
              item.active
                ? "bg-slate-800/90 text-panel-green shadow-xs ring-1 ring-white/10"
                : "text-slate-400 hover:bg-slate-800/50 hover:text-slate-200"
            )}
          >
            <Icon className={cn("size-4 transition-transform group-hover:scale-110", item.active ? "text-panel-green" : "text-slate-400 group-hover:text-slate-200")} />

            {/* Active Glow Indicator */}
            {item.active && (
              <span className="absolute -bottom-1 left-2 right-2 h-[2px] rounded-full bg-panel-green shadow-[0_0_8px_rgba(34,197,94,0.6)]" />
            )}

            {/* Top Right Mini Badge */}
            {item.badge && (
              <span className="absolute -right-1 -top-1 flex size-4 items-center justify-center rounded-full bg-panel-green text-[9px] font-black text-black ring-2 ring-slate-950">
                {item.badge}
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
