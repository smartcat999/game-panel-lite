"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Gamepad2, Gauge, HardDrive, Settings } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { listGameServers } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { usePermissions } from "@/lib/permissions";

export function MobileBottomNav() {
  const pathname = usePathname();
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const { canAccessGameAssets, canEditSettings } = usePermissions();

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
      label: isZh ? "仪表盘" : "Dashboard",
      icon: Gauge,
      active: pathname === "/dashboard"
    },
    {
      href: "/servers",
      label: isZh ? "服务器" : "Servers",
      icon: HardDrive,
      badge: runningCount > 0 ? `${runningCount}` : undefined,
      active: pathname.startsWith("/servers")
    },
    ...(canAccessGameAssets
      ? [
          {
            href: "/games",
            label: isZh ? "游戏资产" : "Games",
            icon: Gamepad2,
            active:
              pathname.startsWith("/games") ||
              pathname.startsWith("/mods") ||
              pathname.startsWith("/presets") ||
              pathname.startsWith("/worlds") ||
              pathname.startsWith("/backups") ||
              pathname.startsWith("/versions")
          }
        ]
      : []),
    ...(canAccessGameAssets
      ? [
          {
            href: "/activity",
            label: isZh ? "系统监控" : "Activity",
            icon: Activity,
            active: pathname.startsWith("/activity")
          }
        ]
      : []),
    ...(canEditSettings
      ? [
          {
            href: "/settings",
            label: isZh ? "系统设置" : "Settings",
            icon: Settings,
            active: pathname.startsWith("/settings")
          }
        ]
      : [])
  ];

  return (
    <nav
      aria-label="Mobile Navigation"
      className="fixed inset-x-0 bottom-0 z-40 flex h-14 items-center justify-around border-t border-slate-800/80 bg-[#090d16]/95 px-2 pb-[env(safe-area-inset-bottom)] backdrop-blur-xl md:hidden"
    >
      {navItems.map((item) => {
        const Icon = item.icon;
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "group relative flex flex-1 flex-col items-center justify-center py-1.5 transition-all",
              item.active ? "text-panel-green font-semibold" : "text-slate-400 hover:text-slate-200"
            )}
          >
            <div className="relative">
              <Icon
                className={cn(
                  "size-4.5 transition-transform group-active:scale-90",
                  item.active ? "text-panel-green" : "text-slate-400 group-hover:text-slate-200"
                )}
              />
              {item.badge && (
                <span className="absolute -right-2.5 -top-1 flex size-3.5 items-center justify-center rounded-full bg-panel-green text-[8px] font-black text-black">
                  {item.badge}
                </span>
              )}
            </div>
            <span className="mt-1 text-[10px] tracking-tight leading-none">
              {item.label}
            </span>
            {item.active && (
              <span className="absolute -bottom-1 h-0.5 w-6 rounded-full bg-panel-green shadow-[0_0_8px_rgba(34,197,94,0.6)]" />
            )}
          </Link>
        );
      })}
    </nav>
  );
}
