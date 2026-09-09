"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Server as ServerIcon,
  Archive,
  Box,
  Settings as SettingsIcon,
  AlertTriangle,
  FileText,
  Bell,
  Building2,
  Check,
  ChevronDown,
  Globe2,
  LogOut,
  X
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { logoutAdmin } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { useAuthBootstrap } from "@/lib/auth-session";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { useConsoleContext } from "@/lib/console-context";

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
  const { setTheme } = useTheme();
  const isZh = locale.startsWith("zh");
  const { platformRole } = usePermissions();
  const { scope, organizations, currentOrganization, selectPlatform, selectOrganization } = useConsoleContext();
  const account = useAuthBootstrap().data?.account;
  const queryClient = useQueryClient();

  const [profileOpen, setProfileOpen] = useState(false);
  const [contextOpen, setContextOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);

  useEffect(() => {
    if (!account?.preferences) return;
    setLocale(account.preferences.locale);
    setTheme(account.preferences.theme);
  }, [account?.preferences?.locale, account?.preferences?.theme, setLocale, setTheme]);

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
    <div className="min-h-screen bg-[var(--app-bg)] text-slate-800 antialiased font-sans p-3 md:p-5">
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

            <div className="relative">
              <button
                type="button"
                aria-expanded={contextOpen}
                onClick={() => setContextOpen((open) => !open)}
                className="flex h-8 min-w-0 items-center gap-2 rounded-lg border border-slate-200/80 bg-slate-50/70 px-2.5 text-left transition hover:bg-slate-100/90"
              >
                {scope.kind === "platform" ? <Globe2 className="size-3.5 text-slate-500" /> : <Building2 className="size-3.5 text-emerald-600" />}
                <span className="min-w-0">
                  <span className="block max-w-36 truncate text-xs font-semibold leading-3.5 text-slate-800">
                    {scope.kind === "platform" ? (isZh ? "平台管理" : "Platform") : currentOrganization?.name ?? (isZh ? "租户空间" : "Workspace")}
                  </span>
                  <span className="block text-[10px] font-normal leading-3 text-slate-400">
                    {scope.kind === "platform"
                      ? (isZh ? "全局控制台" : "Global console")
                      : membershipLabel(currentOrganization?.membershipRole, isZh)}
                  </span>
                </span>
                <ChevronDown className="size-3 text-slate-400" />
              </button>

              {contextOpen ? (
                <div className="absolute left-0 top-full z-50 mt-1.5 w-72 rounded-xl border border-slate-200/80 bg-white p-1.5 shadow-lg">
                  {platformRole === "platform_admin" ? (
                    <button
                      type="button"
                      onClick={() => { selectPlatform(); setContextOpen(false); }}
                      className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left hover:bg-slate-50"
                    >
                      <Globe2 className="size-4 text-slate-500" />
                      <span className="min-w-0 flex-1">
                        <span className="block text-xs font-semibold text-slate-800">{isZh ? "平台管理" : "Platform"}</span>
                        <span className="block text-[10px] text-slate-400">{isZh ? "租户、订单与基础设施" : "Tenants, orders, and infrastructure"}</span>
                      </span>
                      {scope.kind === "platform" ? <Check className="size-3.5 text-emerald-600" /> : null}
                    </button>
                  ) : null}

                  {organizations.length > 0 ? (
                    <div className="mt-1 border-t border-slate-100 pt-1">
                      <div className="px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider text-slate-400">
                        {isZh ? "租户空间" : "Workspaces"}
                      </div>
                      <div className="max-h-64 overflow-y-auto">
                        {organizations.map((organization) => (
                          <button
                            key={organization.id}
                            type="button"
                            onClick={() => { selectOrganization(organization.id); setContextOpen(false); }}
                            className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left hover:bg-slate-50"
                          >
                            <Building2 className="size-4 text-slate-400" />
                            <span className="min-w-0 flex-1">
                              <span className="block truncate text-xs font-semibold text-slate-800">{organization.name}</span>
                              <span className="block text-[10px] text-slate-400">{membershipLabel(organization.membershipRole, isZh)}</span>
                            </span>
                            {scope.kind === "organization" && scope.organizationId === organization.id ? <Check className="size-3.5 text-emerald-600" /> : null}
                          </button>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : null}
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

            <div className="h-3.5 w-px bg-slate-200 mx-1 shrink-0" />

            <div className="relative">
              <button
                type="button"
                onClick={() => setProfileOpen(!profileOpen)}
                className="h-7 flex items-center gap-1.5 px-1 rounded-md hover:bg-slate-100 cursor-pointer transition"
              >
                <div className="w-5 h-5 rounded-md bg-slate-100 border border-slate-200 text-slate-700 font-semibold flex items-center justify-center text-[9px] font-mono shrink-0">
                  {(account?.username ?? "GP").slice(0, 2).toUpperCase()}
                </div>
                <span className="text-xs font-medium text-slate-700 truncate max-w-[70px]">
                  {account?.username ?? (isZh ? "当前用户" : "Current user")}
                </span>
              </button>

              {profileOpen && (
                <div className="absolute right-0 top-full mt-1.5 w-44 rounded-xl border border-slate-200/80 bg-white p-1.5 shadow-lg z-50 animate-in fade-in zoom-in-95 duration-100 space-y-1">
                  <div className="px-2.5 py-1.5 border-b border-slate-100 text-[11px]">
                    <div className="font-bold text-slate-900">{account?.username ?? (isZh ? "当前用户" : "Current user")}</div>
                    <div className="text-slate-400 text-[10px] truncate">{platformRole === "platform_admin" ? (isZh ? "平台管理员账号" : "Platform administrator") : (isZh ? "平台用户账号" : "Platform user")}</div>
                  </div>
                  <Link
                    href="/settings"
                    onClick={() => setProfileOpen(false)}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-900 transition"
                  >
                    <SettingsIcon className="size-3.5" />
                    <span>{isZh ? "账号设置" : "Account settings"}</span>
                  </Link>
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
        <nav className="flex gap-1 overflow-x-auto rounded-xl border bg-white p-1.5 micro-border subtle-elevation md:hidden">
          <MobileNavLink active={pathname.startsWith("/servers")} href="/servers" icon={<ServerIcon className="size-3.5" />} label={isZh ? "实例" : "Instances"} />
          <MobileNavLink active={pathname.startsWith("/worlds")} href="/worlds" icon={<Archive className="size-3.5" />} label={isZh ? "存档" : "Saves"} />
          <MobileNavLink active={pathname.startsWith("/mods")} href="/mods" icon={<Box className="size-3.5" />} label={isZh ? "模组" : "Mods"} />
          <MobileNavLink active={pathname.startsWith("/settings")} href="/settings" icon={<SettingsIcon className="size-3.5" />} label={isZh ? "设置" : "Settings"} />
        </nav>

        <div className="flex flex-col items-stretch gap-3.5 md:min-h-[700px] md:flex-row">
          {/* SIDEBAR */}
          <aside className="relative hidden w-[210px] shrink-0 flex-col justify-between rounded-xl border bg-white p-2.5 transition-all duration-200 micro-border subtle-elevation md:flex">
            <div className="space-y-3.5">
              {/* Workspace Title Strip */}
              <div className="px-1 border-b micro-border pb-2">
                <span className="text-[9px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "工作空间" : "WORKSPACE"}
                </span>
              </div>

              {/* GROUP 2: COMPUTE */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">{isZh ? "计算" : "COMPUTE"}</div>
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
                  <span className="truncate">{isZh ? "实例" : "Instances"}</span>
                </Link>
              </div>

              {/* GROUP 3: STORAGE */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">{isZh ? "存储" : "STORAGE"}</div>
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
                  <span className="truncate">{isZh ? "世界与存档" : "Worlds & saves"}</span>
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
                  <span className="truncate">{isZh ? "模组工坊" : "Mod workshop"}</span>
                </Link>
              </div>

              {/* GROUP 4: OBSERVABILITY */}
              <div className="space-y-0.5">
                <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">{isZh ? "运维" : "OPERATIONS"}</div>
                <Link
                  href="/settings"
                  className={cn(
                    "h-7 flex items-center gap-2 px-2 rounded-lg text-xs font-medium transition",
                    pathname.startsWith("/settings")
                      ? "bg-slate-100 text-slate-900 font-semibold"
                      : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                  )}
                >
                  <SettingsIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">{isZh ? "设置" : "Settings"}</span>
                </Link>
                <button
                  type="button"
                  onClick={() => setDrawerOpen(true)}
                  className="w-full h-7 flex items-center gap-2 px-2 rounded-lg text-slate-600 hover:bg-slate-50 hover:text-slate-900 text-xs font-medium transition cursor-pointer"
                >
                  <AlertTriangle className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <span className="truncate">{isZh ? "事件与告警" : "Incidents & alerts"}</span>
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

function membershipLabel(role: string | undefined, isZh: boolean) {
  if (role === "platform_managed") return isZh ? "平台代管" : "Platform managed";
  if (role === "owner") return isZh ? "空间所有者" : "Workspace owner";
  if (role === "admin") return isZh ? "空间管理员" : "Workspace admin";
  if (role === "member") return isZh ? "成员" : "Member";
  if (role === "viewer") return isZh ? "只读成员" : "Viewer";
  return isZh ? "正在加载" : "Loading";
}

function MobileNavLink({
  active,
  href,
  icon,
  label
}: {
  active: boolean;
  href: string;
  icon: ReactNode;
  label: string;
}) {
  return (
    <Link
      href={href}
      className={cn(
        "flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium transition",
        active ? "bg-slate-100 text-slate-900" : "text-slate-500 hover:bg-slate-50 hover:text-slate-900"
      )}
    >
      {icon}
      <span>{label}</span>
    </Link>
  );
}
