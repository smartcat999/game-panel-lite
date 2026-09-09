"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Server as ServerIcon,
  Archive,
  Box,
  LayoutDashboard,
  Activity,
  AlertTriangle,
  FileText,
  Bell,
  ChevronDown,
  LogOut,
  X
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { logoutAdmin } from "@/lib/api";
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

  const [profileOpen, setProfileOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);

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
    <div className="min-h-screen bg-[#F8FAFC] text-slate-800 antialiased font-sans p-3 md:p-5">
      <div className="max-w-6xl mx-auto space-y-3">
        {/* ULTRA-CLEAN TOP NAVBAR */}
        <header className="h-12 bg-white border micro-border rounded-xl px-3.5 flex items-center justify-between subtle-elevation select-none">
          {/* LEFT: LOGO + UNIFIED WORKSPACE TRIGGER */}
          <div className="flex items-center gap-2.5 shrink-0">
            <Link
              href="/servers"
              className="w-6 h-6 rounded-md bg-emerald-500/10 border border-emerald-500/25 flex items-center justify-center text-emerald-600 font-bold text-xs shrink-0 hover:opacity-80 transition"
            >
              GP
            </Link>

            <div className="h-3.5 w-px bg-slate-200 shrink-0" />

            <div className="h-7 px-2 rounded-md border border-slate-200/80 bg-slate-50/70 hover:bg-slate-100/90 cursor-pointer transition flex items-center gap-1.5 text-xs font-semibold text-slate-800">
              <span className="w-2 h-2 rounded-full bg-emerald-500 shrink-0" />
              <span>Geek Guild</span>
              <span className="text-[9px] font-mono text-emerald-700 bg-emerald-100/60 border border-emerald-300/40 px-1 py-0.2 rounded font-semibold ml-0.5">
                {role === "admin" ? "Pro" : "Member"}
              </span>
              <ChevronDown className="w-3 h-3 text-slate-400 shrink-0" />
            </div>
          </div>

          {/* RIGHT: UTILITIES & USER (ICON BUTTONS ONLY) */}
          <div className="flex items-center gap-1 text-xs shrink-0">
            <a
              href="https://terraria.wiki.gg"
              target="_blank"
              rel="noreferrer"
              title={isZh ? "文档" : "Documentation"}
              className="w-7 h-7 rounded-md text-slate-500 hover:text-slate-900 hover:bg-slate-100 flex items-center justify-center transition"
            >
              <FileText className="w-3.5 h-3.5" />
            </a>

            <button
              type="button"
              onClick={() => setDrawerOpen(true)}
              title={isZh ? "告警与通知" : "Alerts & Incidents"}
              className="w-7 h-7 rounded-md hover:bg-slate-100 text-slate-500 hover:text-slate-900 flex items-center justify-center relative transition cursor-pointer"
            >
              <Bell className="w-3.5 h-3.5" />
              <span className="w-1.5 h-1.5 rounded-full bg-amber-500 absolute top-1.5 right-1.5 ring-2 ring-white" />
            </button>

            <button
              type="button"
              onClick={() => setLocale(locale === "zh" ? "en" : "zh")}
              title={locale === "zh" ? "Switch to English" : "切换至中文"}
              className="h-7 px-1.5 rounded-md hover:bg-slate-100 text-slate-500 hover:text-slate-900 flex items-center justify-center font-mono text-[11px] font-semibold transition cursor-pointer"
            >
              {locale === "zh" ? "EN" : "中"}
            </button>

            <div className="h-3.5 w-px bg-slate-200 mx-1 shrink-0" />

            <div className="relative">
              <button
                type="button"
                onClick={() => setProfileOpen(!profileOpen)}
                className="h-7 flex items-center gap-1.5 px-1 rounded-md hover:bg-slate-100 cursor-pointer transition"
              >
                <div className="w-5 h-5 rounded-md bg-slate-100 border border-slate-200 text-slate-700 font-semibold flex items-center justify-center text-[9px] font-mono shrink-0">
                  {role === "admin" ? "AD" : "DM"}
                </div>
                <span className="text-xs font-medium text-slate-700 truncate max-w-[70px]">
                  {role === "admin" ? "Alex M." : "David M."}
                </span>
              </button>

              {profileOpen && (
                <div className="absolute right-0 top-full mt-1.5 w-44 rounded-xl border border-slate-200/80 bg-white p-1.5 shadow-lg z-50 animate-in fade-in zoom-in-95 duration-100 space-y-1">
                  <div className="px-2.5 py-1.5 border-b border-slate-100 text-[11px]">
                    <div className="font-bold text-slate-900">{role === "admin" ? "Alex M. (Admin)" : "David M. (Member)"}</div>
                    <div className="text-slate-400 font-mono text-[10px] truncate">gamepanel@localhost</div>
                  </div>
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
        </header>

        {/* MAIN WORKSPACE LAYOUT: SIDEBAR + CONTENT */}
        <div className="flex flex-col md:flex-row gap-3.5 items-stretch min-h-[700px]">
          {/* SIDEBAR */}
          <aside className="w-full md:w-[210px] bg-white border micro-border rounded-xl p-2.5 subtle-elevation flex flex-col justify-between shrink-0 relative transition-all duration-200">
            <div className="space-y-3.5">
              {/* Workspace Title Strip */}
              <div className="px-1 border-b micro-border pb-2">
                <span className="text-[9px] font-bold uppercase tracking-wider text-slate-400">
                  COLLABORATOR
                </span>
              </div>

              {/* GROUP 1: WORKSPACE */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">WORKSPACE</div>
                <Link
                  href="/servers"
                  className={cn(
                    "w-full h-7 flex items-center gap-2 px-2 rounded-lg text-xs font-medium transition text-left",
                    pathname === "/dashboard"
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                  )}
                >
                  <LayoutDashboard className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">Dashboard</span>
                </Link>
              </div>

              {/* GROUP 2: COMPUTE */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">COMPUTE</div>
                <Link
                  href="/servers"
                  className={cn(
                    "w-full h-7 flex items-center gap-2 px-2 rounded-lg text-xs transition text-left",
                    pathname.startsWith("/servers")
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900 font-medium"
                  )}
                >
                  <ServerIcon className="w-3.5 h-3.5 text-slate-800 shrink-0" />
                  <span className="truncate">Instances</span>
                </Link>
              </div>

              {/* GROUP 3: STORAGE */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">STORAGE</div>
                <Link
                  href="/worlds"
                  className={cn(
                    "h-7 flex items-center gap-2 px-2 rounded-lg text-xs font-medium transition",
                    pathname.startsWith("/worlds") || pathname.startsWith("/backups")
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                  )}
                >
                  <Archive className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">Worlds & Saves</span>
                </Link>
                <Link
                  href="/mods"
                  className={cn(
                    "h-7 flex items-center gap-2 px-2 rounded-lg text-xs font-medium transition",
                    pathname.startsWith("/mods")
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                  )}
                >
                  <Box className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">Mod Workshop</span>
                </Link>
              </div>

              {/* GROUP 4: OBSERVABILITY */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">OBSERVABILITY</div>
                <Link
                  href="/settings"
                  className={cn(
                    "h-7 flex items-center gap-2 px-2 rounded-lg text-xs font-medium transition",
                    pathname.startsWith("/settings")
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                  )}
                >
                  <Activity className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">Audit Logs</span>
                </Link>
                <button
                  type="button"
                  onClick={() => setDrawerOpen(true)}
                  className="w-full h-7 flex items-center gap-2 px-2 rounded-lg text-slate-600 hover:bg-slate-50 hover:text-slate-900 text-xs font-medium transition cursor-pointer"
                >
                  <AlertTriangle className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">Incidents & Alerts</span>
                  <span className="ml-auto w-1.5 h-1.5 rounded-full bg-amber-500" />
                </button>
              </div>
            </div>
          </aside>

          {/* MAIN CONTENT AREA */}
          <main className="flex-1 w-full min-w-0">
            {children}
          </main>
        </div>
      </div>

      {/* Incident Drawer */}
      {drawerOpen && (
        <div className="fixed inset-0 z-50 flex justify-end animate-in fade-in duration-150">
          <div
            onClick={() => setDrawerOpen(false)}
            className="fixed inset-0 bg-slate-900/20 backdrop-blur-xs"
          />
          <aside className="relative w-80 md:w-96 bg-white border-l micro-border shadow-2xl z-10 flex flex-col h-full animate-in slide-in-from-right duration-200">
            <div className="h-12 px-4 border-b micro-border flex items-center justify-between shrink-0">
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
                <span className="text-xs font-bold text-slate-900">
                  {isZh ? "告警与通知中心" : "Incidents & Alerts"}
                </span>
              </div>
              <button
                type="button"
                onClick={() => setDrawerOpen(false)}
                className="w-6 h-6 rounded-md hover:bg-slate-100 flex items-center justify-center text-slate-400 hover:text-slate-600 cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-3 space-y-2.5 text-xs">
              <div className="p-3 rounded-xl border border-rose-200 bg-rose-50/40 space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-rose-900 text-xs flex items-center gap-1.5">
                    <span className="w-1.5 h-1.5 rounded-full bg-rose-500" />
                    <span>{isZh ? "实例资源告警" : "Resource Alert"}</span>
                  </span>
                  <span className="text-[10px] font-mono text-rose-600 font-semibold">P1</span>
                </div>
                <p className="text-[11px] text-slate-600 leading-relaxed">
                  {isZh
                    ? "实例运行正常，全节点网络健康度 100%。"
                    : "All instances are healthy. Fleet cluster latency is nominal."}
                </p>
                <div className="flex items-center gap-2 pt-1">
                  <button
                    type="button"
                    onClick={() => setDrawerOpen(false)}
                    className="flex-1 h-6 bg-slate-900 hover:bg-slate-800 text-white rounded-md text-[11px] font-semibold transition cursor-pointer"
                  >
                    {isZh ? "确认" : "Acknowledge"}
                  </button>
                </div>
              </div>
            </div>
          </aside>
        </div>
      )}
    </div>
  );
}
