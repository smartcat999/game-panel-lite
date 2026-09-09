"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Server as ServerIcon,
  Archive,
  Box,
  Sliders,
  ChevronsLeft,
  ChevronsRight,
  Languages,
  LogOut
} from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { listGameServers, logoutAdmin } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/")) {
    return <>{children}</>;
  }
  return <AppChrome>{children}</AppChrome>;
}

function AppChrome({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { locale, setLocale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { role } = usePermissions();
  const queryClient = useQueryClient();

  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);

  const serversQuery = useQuery({
    queryKey: ["game-servers"],
    queryFn: listGameServers,
    retry: false
  });

  const servers = serversQuery.data ?? [];

  const handleLogout = async () => {
    try {
      await logoutAdmin();
      queryClient.clear();
      window.location.reload();
    } catch {
      window.location.reload();
    }
  };

  return (
    <div className="min-h-screen bg-[#F8FAFC] text-slate-800 antialiased">
      {/* Top Navbar */}
      <header className="sticky top-0 z-40 h-12 border-b micro-border bg-white/95 backdrop-blur-md px-3 sm:px-6 subtle-elevation">
        <div className="flex h-full items-center justify-between gap-3">
          {/* Brand */}
          <div className="flex items-center gap-2.5 shrink-0">
            <Link
              href="/servers"
              className="flex items-center gap-2 text-xs font-bold tracking-tight text-slate-900 hover:opacity-90 transition"
            >
              <div className="flex size-6 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 font-bold border border-emerald-500/25">
                GP
              </div>
              <span className="font-bold tracking-tight">
                GamePanel <span className="text-emerald-600 font-mono text-[11px]">Lite</span>
              </span>
            </Link>
          </div>

          {/* Right Tools */}
          <div className="flex items-center gap-2 shrink-0">
            {/* 1-Click Language Switcher */}
            <button
              type="button"
              onClick={() => setLocale(locale === "zh" ? "en" : "zh")}
              title={isZh ? "切换至英文 (Switch to English)" : "切换至中文 (Switch to Chinese)"}
              className="flex h-7 items-center gap-1 rounded-md px-2 text-xs font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900 transition border border-slate-200/60 cursor-pointer"
            >
              <Languages className="size-3.5 text-slate-500" />
              <span className="font-mono text-[11px] font-semibold">{locale === "zh" ? "EN" : "中"}</span>
            </button>

            <div className="h-3.5 w-px bg-slate-200 shrink-0" />

            {/* User Profile */}
            <div className="relative">
              <button
                type="button"
                onClick={() => setProfileOpen(!profileOpen)}
                className="flex items-center gap-1.5 rounded-md p-1 hover:bg-slate-100 transition cursor-pointer"
              >
                <div className="flex size-6 shrink-0 items-center justify-center rounded-md bg-slate-100 border border-slate-200 text-slate-700 font-semibold text-[10px] font-mono">
                  {role === "admin" ? "AD" : "MB"}
                </div>
              </button>

              {profileOpen && (
                <div className="absolute right-0 top-full mt-1.5 w-40 rounded-xl border border-slate-200/80 bg-white p-1 shadow-lg z-50 animate-in fade-in zoom-in-95 duration-100">
                  <button
                    onClick={handleLogout}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-rose-600 hover:bg-rose-50 transition cursor-pointer"
                  >
                    <LogOut className="size-3.5" />
                    <span>{isZh ? "退出登录" : "Sign Out"}</span>
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </header>

      {/* Main Layout Body */}
      <div className="flex min-h-[calc(100vh-3rem)]">
        {/* Sidebar */}
        <aside
          className={cn(
            "bg-white border-r micro-border flex flex-col justify-between shrink-0 p-2.5 transition-all duration-200",
            sidebarCollapsed ? "w-14" : "w-48"
          )}
        >
          <div className="space-y-1">
            {/* Collapse toggle */}
            <div className="flex items-center justify-end px-1 border-b border-slate-100 pb-2 mb-1">
              <button
                onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
                className="size-6 rounded hover:bg-slate-100 text-slate-400 hover:text-slate-700 flex items-center justify-center transition cursor-pointer"
                title={sidebarCollapsed ? (isZh ? "展开" : "Expand") : (isZh ? "收起" : "Collapse")}
              >
                {sidebarCollapsed ? <ChevronsRight className="size-3.5" /> : <ChevronsLeft className="size-3.5" />}
              </button>
            </div>

            <SidebarLink
              href="/servers"
              active={pathname.startsWith("/servers")}
              icon={<ServerIcon className="size-4" />}
              label={isZh ? "服务器" : "Servers"}
              badge={servers.length > 0 ? String(servers.length) : undefined}
              collapsed={sidebarCollapsed}
            />

            <SidebarLink
              href="/worlds"
              active={pathname.startsWith("/worlds") || pathname.startsWith("/backups")}
              icon={<Archive className="size-4" />}
              label={isZh ? "世界存档" : "Worlds"}
              collapsed={sidebarCollapsed}
            />

            <SidebarLink
              href="/mods"
              active={pathname.startsWith("/mods")}
              icon={<Box className="size-4" />}
              label={isZh ? "模组工坊" : "Mods"}
              collapsed={sidebarCollapsed}
            />

            <SidebarLink
              href="/settings"
              active={pathname.startsWith("/settings")}
              icon={<Sliders className="size-4" />}
              label={isZh ? "系统设置" : "Settings"}
              collapsed={sidebarCollapsed}
            />
          </div>
        </aside>

        {/* Main Content Area */}
        <main className="flex-1 p-4 sm:p-6 max-w-7xl w-full mx-auto overflow-y-auto">
          {children}
        </main>
      </div>
    </div>
  );
}

function SidebarLink({
  href,
  active,
  icon,
  label,
  badge,
  collapsed
}: {
  href: string;
  active: boolean;
  icon: ReactNode;
  label: string;
  badge?: string;
  collapsed: boolean;
}) {
  return (
    <Link
      href={href}
      title={label}
      className={cn(
        "flex h-8 items-center gap-2.5 rounded-lg px-2 text-xs font-medium transition cursor-pointer select-none",
        active
          ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs"
          : "text-slate-600 hover:bg-slate-50 hover:text-slate-900",
        collapsed && "justify-center px-0"
      )}
    >
      <span className={cn("shrink-0", active ? "text-slate-900" : "text-slate-400")}>{icon}</span>
      {!collapsed && <span className="truncate">{label}</span>}
      {!collapsed && badge && (
        <span className="ml-auto text-[10px] font-mono font-semibold bg-white border micro-border px-1.5 py-0.2 rounded text-slate-600 shadow-2xs">
          {badge}
        </span>
      )}
    </Link>
  );
}
