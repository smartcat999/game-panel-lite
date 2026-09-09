"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Archive,
  BarChart3,
  Bell,
  Box,
  ChevronDown,
  ChevronsLeft,
  ChevronsRight,
  HardDrive,
  KeyRound,
  Languages,
  LayoutDashboard,
  LayoutGrid,
  LogOut,
  Server,
  ShieldAlert,
  Sliders,
  Users,
  X
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useI18n, type Locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { TopNav } from "@/components/top-nav";
import { AppsDrawer } from "@/components/apps-drawer";
import { IncidentDrawer } from "@/components/incident-drawer";
import { PermissionDenied } from "@/components/permission-denied";
import { useAuthBootstrap } from "@/lib/auth-session";
import { usePermissions } from "@/lib/permissions";
import { PerspectiveProvider, usePerspective } from "@/lib/perspective-context";
import {
  changeAdminPassword,
  getSettings,
  listGameServers,
  logoutAdmin,
  updateLocale
} from "@/lib/api";

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/")) {
    return <>{children}</>;
  }
  return (
    <PerspectiveProvider>
      <AppChrome>{children}</AppChrome>
    </PerspectiveProvider>
  );
}

function AppChrome({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale, setLocale, t } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canAccessGameAssets, canCreateServer, canEditSettings } = usePermissions();
  const { isMemberView, isWorkspaceAdminView, isSuperAdminView } = usePerspective();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [incidentOpen, setIncidentOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [accountTab, setAccountTab] = useState<"language" | "password">("language");
  const [selectedLocale, setSelectedLocale] = useState<Locale>(locale);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [accountMessage, setAccountMessage] = useState("");

  const profileRef = useRef<HTMLDivElement>(null);

  const authQuery = useAuthBootstrap();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 30000 });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, staleTime: 10000 });

  const servers = serversQuery.data ?? [];

  const logoutMutation = useMutation({
    mutationFn: logoutAdmin,
    onSuccess: async () => {
      setProfileOpen(false);
      await authQuery.refetch();
      router.push("/dashboard");
    }
  });

  const localeMutation = useMutation({
    mutationFn: (nextLocale: Locale) => updateLocale(nextLocale),
    onSuccess: async (result) => {
      setLocale(result.locale);
      window.localStorage.setItem("gamepanel.locale", result.locale);
      setSelectedLocale(result.locale);
      setAccountMessage(t("languageSaved"));
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (err) => setAccountMessage(err instanceof Error ? err.message : t("languageSaveFailed"))
  });

  const passwordMutation = useMutation({
    mutationFn: () => changeAdminPassword(currentPassword, newPassword),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setAccountMessage(t("passwordChanged"));
    },
    onError: (err) => setAccountMessage(err instanceof Error ? err.message : t("passwordChangeFailed"))
  });

  useEffect(() => {
    setProfileOpen(false);
    setDrawerOpen(false);
  }, [pathname]);

  useEffect(() => {
    const saved = window.localStorage.getItem("gamepanel.sidebar-collapsed");
    if (saved === "true") setSidebarCollapsed(true);
  }, []);

  const toggleSidebar = () => {
    setSidebarCollapsed((prev) => {
      const next = !prev;
      window.localStorage.setItem("gamepanel.sidebar-collapsed", String(next));
      return next;
    });
  };

  useEffect(() => {
    if (settingsQuery.data?.locale && !window.localStorage.getItem("gamepanel.locale")) {
      setLocale(settingsQuery.data.locale);
      setSelectedLocale(settingsQuery.data.locale);
    }
  }, [settingsQuery.data?.locale, setLocale]);

  useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (!profileRef.current?.contains(event.target as Node)) {
        setProfileOpen(false);
      }
    };
    window.addEventListener("pointerdown", handlePointerDown);
    return () => window.removeEventListener("pointerdown", handlePointerDown);
  }, []);

  const openAccountSettings = (tab: "language" | "password" = "language") => {
    setAccountTab(tab);
    setSelectedLocale(locale);
    setAccountMessage("");
    setProfileOpen(false);
    setAccountOpen(true);
  };

  const saveLocale = () => {
    setAccountMessage("");
    if (canEditSettings) {
      localeMutation.mutate(selectedLocale);
      return;
    }
    window.localStorage.setItem("gamepanel.locale", selectedLocale);
    setLocale(selectedLocale);
    setAccountMessage(t("languageSaved"));
  };

  const submitPasswordChange = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setAccountMessage("");
    if (newPassword !== confirmPassword) return;
    passwordMutation.mutate();
  };

  const passwordMismatch = confirmPassword.length > 0 && newPassword !== confirmPassword;

  return (
    <div className="min-h-screen bg-[#F8FAFC] text-slate-800 antialiased">
      {/* Top Global Command Header (52px / h-12) */}
      <header className="sticky top-0 z-40 h-12 border-b micro-border bg-white/95 backdrop-blur-md px-3 sm:px-6 subtle-elevation">
        <div className="flex h-full items-center justify-between gap-3">
          {/* Left: Brand + Inline Workspace Trigger */}
          <div className="flex items-center gap-2.5 shrink-0">
            <Link
              href="/dashboard"
              className="flex items-center gap-2 text-xs font-bold tracking-tight text-slate-900 hover:opacity-90 transition"
            >
              <div className="flex size-6 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 font-bold border border-emerald-500/25">
                GP
              </div>
              <span className="hidden sm:inline font-bold tracking-tight">
                GamePanel <span className="text-emerald-600 font-mono text-[11px]">Lite</span>
              </span>
            </Link>

            <div className="h-3.5 w-px bg-slate-200 shrink-0" />

            {/* Workspace Selector Pill */}
            <div className="flex items-center gap-1.5 rounded-md border border-slate-200/80 bg-slate-50/80 px-2 py-1 text-xs font-semibold text-slate-800 shadow-2xs">
              <span className="size-2 rounded-full bg-emerald-500 shrink-0" />
              <span className="truncate max-w-[110px] sm:max-w-none">Geek Guild</span>
              <span className="text-[9px] font-mono text-emerald-700 bg-emerald-100/60 border border-emerald-300/40 px-1 rounded font-semibold">
                Pro
              </span>
            </div>
          </div>

          {/* Right: 3-Role Perspective Controller + Icon Utility Toolbar */}
          <div className="flex items-center gap-1.5 sm:gap-2 shrink-0">
            {/* 3-Role Perspective Switcher */}
            <TopNav />

            <div className="h-3.5 w-px bg-slate-200 hidden sm:block shrink-0" />

            {/* Incidents & Alerts Trigger */}
            <button
              type="button"
              onClick={() => setIncidentOpen(true)}
              aria-label="Alerts and notifications"
              title={isZh ? "告警与通知中心" : "Incidents & Alerts"}
              className="relative flex size-7 items-center justify-center rounded-md text-slate-500 hover:bg-slate-100 hover:text-slate-900 transition"
            >
              <Bell className="size-3.5" />
              <span className="absolute top-1 right-1 size-1.5 rounded-full bg-amber-500 ring-2 ring-white" />
            </button>

            {/* Apps Drawer Button */}
            <button
              type="button"
              onClick={() => setDrawerOpen(true)}
              aria-label="All Apps and Navigation"
              title={isZh ? "所有功能组件 (⌘B)" : "All Apps (⌘B)"}
              className="flex size-7 items-center justify-center rounded-md text-slate-500 hover:bg-slate-100 hover:text-slate-900 transition"
            >
              <LayoutGrid className="size-3.5" />
            </button>

            <div className="h-3.5 w-px bg-slate-200 shrink-0" />

            {/* Profile Avatar & Menu */}
            <div ref={profileRef} className="relative">
              <button
                type="button"
                aria-expanded={profileOpen}
                aria-label={t("userProfile")}
                className="flex items-center gap-1.5 rounded-md p-1 hover:bg-slate-100 transition"
                onClick={() => setProfileOpen((value) => !value)}
              >
                <div className="flex size-5 shrink-0 items-center justify-center rounded bg-slate-100 border border-slate-200 text-slate-700 font-semibold text-[9px] font-mono">
                  {(authQuery.data?.account?.username ?? "GP").slice(0, 2).toUpperCase()}
                </div>
                <span className="hidden md:inline text-xs font-medium text-slate-700 max-w-[80px] truncate">
                  {authQuery.data?.account?.username ?? t("localUser")}
                </span>
                <ChevronDown className="size-3 text-slate-400" />
              </button>

              {profileOpen && (
                <div className="absolute right-0 top-9 z-50 w-56 rounded-xl border micro-border bg-white p-2.5 shadow-2xl space-y-2 animate-in fade-in zoom-in-95 duration-100">
                  <div className="flex items-center gap-2.5 border-b border-slate-100 pb-2.5">
                    <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-slate-100 font-mono text-xs font-bold text-slate-700 border border-slate-200">
                      {(authQuery.data?.account?.username ?? "GP").slice(0, 2).toUpperCase()}
                    </div>
                    <div className="min-w-0">
                      <p className="truncate text-xs font-bold text-slate-900">
                        {authQuery.data?.account?.username ?? t("localUser")}
                      </p>
                      <p className="text-[10px] text-emerald-600 font-mono font-bold uppercase">
                        {isSuperAdminView
                          ? (isZh ? "平台超级管理员" : "Superadmin")
                          : isWorkspaceAdminView
                          ? (isZh ? "工作区管理员" : "Workspace Admin")
                          : (isZh ? "开黑成员 (只读)" : "Member")}
                      </p>
                    </div>
                  </div>

                  <div className="space-y-0.5">
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                      onClick={() => openAccountSettings("language")}
                    >
                      <Languages className="size-3.5 text-sky-500" />
                      <span>{isZh ? "语言偏好" : "Language"}</span>
                    </button>
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                      onClick={() => openAccountSettings("password")}
                    >
                      <KeyRound className="size-3.5 text-amber-500" />
                      <span>{isZh ? "账号安全" : "Security"}</span>
                    </button>
                  </div>

                  <div className="border-t border-slate-100 pt-1">
                    <button
                      type="button"
                      disabled={logoutMutation.isPending}
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-rose-600 transition hover:bg-rose-50"
                      onClick={() => logoutMutation.mutate()}
                    >
                      <LogOut className="size-3.5" />
                      <span>{logoutMutation.isPending ? (isZh ? "正在退出..." : "Logging out...") : (isZh ? "退出登录" : "Log out")}</span>
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </header>

      {/* Main Workspace Layout (Sidebar + Content Container) */}
      <div className="mx-auto max-w-7xl p-3 sm:p-5">
        <div className="flex flex-col md:flex-row gap-4 items-stretch min-h-[calc(100vh-6rem)]">
          {/* Full-Height Cloud Sidebar */}
          <aside
            className={cn(
              "bg-white border micro-border rounded-xl p-2.5 subtle-elevation flex flex-col justify-between shrink-0 transition-all duration-200 select-none",
              sidebarCollapsed ? "w-full md:w-14 items-center" : "w-full md:w-[210px]"
            )}
          >
            {/* Top Navigation Groups */}
            <div className="w-full space-y-3.5">
              {/* Workspace Header & Collapse Toggle */}
              <div className="flex items-center justify-between px-1 border-b border-slate-100 pb-2">
                {!sidebarCollapsed && (
                  <span className="text-[9px] font-bold uppercase tracking-wider text-slate-400">
                    WORKSPACE
                  </span>
                )}
                <button
                  type="button"
                  onClick={toggleSidebar}
                  title={sidebarCollapsed ? (isZh ? "展开侧边栏" : "Expand Sidebar") : (isZh ? "折叠侧边栏" : "Collapse Sidebar")}
                  className="size-5 rounded hover:bg-slate-100 text-slate-400 hover:text-slate-700 flex items-center justify-center transition ml-auto"
                >
                  {sidebarCollapsed ? <ChevronsRight className="size-3.5" /> : <ChevronsLeft className="size-3.5" />}
                </button>
              </div>

              {/* GROUP 1: OVERVIEW */}
              <div className="space-y-0.5">
                {!sidebarCollapsed && (
                  <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">
                    WORKSPACE
                  </div>
                )}
                <SidebarItem
                  href="/dashboard"
                  active={pathname === "/dashboard"}
                  icon={<LayoutDashboard className="size-3.5" />}
                  label="Dashboard"
                  collapsed={sidebarCollapsed}
                />
              </div>

              {/* GROUP 2: COMPUTE (No Config, No +Deploy) */}
              <div className="space-y-0.5">
                {!sidebarCollapsed && (
                  <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">
                    COMPUTE
                  </div>
                )}
                <SidebarItem
                  href="/servers"
                  active={pathname.startsWith("/servers")}
                  icon={<HardDrive className="size-3.5" />}
                  label="Instances"
                  badge={servers.length > 0 ? String(servers.length) : undefined}
                  collapsed={sidebarCollapsed}
                />
              </div>

              {/* GROUP 3: STORAGE */}
              <div className="space-y-0.5">
                {!sidebarCollapsed && (
                  <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">
                    STORAGE
                  </div>
                )}
                <SidebarItem
                  href="/worlds"
                  active={pathname.startsWith("/worlds") || pathname.startsWith("/backups")}
                  icon={<Archive className="size-3.5" />}
                  label="Worlds & Saves"
                  collapsed={sidebarCollapsed}
                />
                <SidebarItem
                  href="/mods"
                  active={pathname.startsWith("/mods") || pathname.startsWith("/games") || pathname.startsWith("/presets")}
                  icon={<Box className="size-3.5" />}
                  label="Mod Workshop"
                  collapsed={sidebarCollapsed}
                />
              </div>

              {/* GROUP 4: OBSERVABILITY (Audit logs归位运维分类) */}
              <div className="space-y-0.5">
                {!sidebarCollapsed && (
                  <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">
                    OBSERVABILITY
                  </div>
                )}
                <SidebarItem
                  href="/activity"
                  active={pathname.startsWith("/activity")}
                  icon={<BarChart3 className="size-3.5" />}
                  label="Audit Logs"
                  collapsed={sidebarCollapsed}
                />
                <button
                  type="button"
                  onClick={() => setIncidentOpen(true)}
                  title="Incidents & Alerts"
                  className={cn(
                    "flex w-full items-center gap-2 rounded-lg px-2 text-xs font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900 h-7",
                    sidebarCollapsed && "justify-center px-0"
                  )}
                >
                  <ShieldAlert className="size-3.5 text-slate-400 shrink-0" />
                  {!sidebarCollapsed && (
                    <>
                      <span className="truncate text-left flex-1">Incidents & Alerts</span>
                      <span className="size-1.5 rounded-full bg-amber-500 shrink-0" />
                    </>
                  )}
                </button>
              </div>

              {/* GROUP 5: TEAM & BILLING (Workspace Admin only, strictly no hardware) */}
              {!isMemberView && (
                <div className="space-y-0.5">
                  {!sidebarCollapsed && (
                    <div className="px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-slate-400">
                      ORGANIZATION
                    </div>
                  )}
                  <SidebarItem
                    href="/settings"
                    active={pathname.startsWith("/settings")}
                    icon={<Users className="size-3.5" />}
                    label="Team Members"
                    badge="6"
                    collapsed={sidebarCollapsed}
                  />
                  <SidebarItem
                    href="/settings?tab=billing"
                    active={pathname.startsWith("/settings") && typeof window !== "undefined" && window.location.search.includes("billing")}
                    icon={<Sliders className="size-3.5" />}
                    label="Billing & Quota"
                    collapsed={sidebarCollapsed}
                  />
                </div>
              )}

              {/* GROUP 6: PLATFORM INFRASTRUCTURE (Visible ONLY to Platform Superadmin!) */}
              {isSuperAdminView && (
                <div className="space-y-0.5 pt-2 border-t border-purple-100">
                  {!sidebarCollapsed && (
                    <div className="px-2 py-0.5 text-[9px] font-bold uppercase tracking-wider text-purple-600">
                      PLATFORM HARDWARE
                    </div>
                  )}
                  <SidebarItem
                    href="/settings?tab=nodes"
                    active={false}
                    icon={<Server className="size-3.5 text-purple-600" />}
                    label="Cluster Nodes"
                    badge="3 Live"
                    badgeTone="purple"
                    collapsed={sidebarCollapsed}
                  />
                </div>
              )}
            </div>

            {/* Bottom Quota Gauge (Single Metric Display - No Slash Policy!) */}
            {!sidebarCollapsed ? (
              <div className="w-full pt-3 border-t border-slate-100 mt-4 space-y-2">
                <div className="flex items-center justify-between text-[11px] font-semibold text-slate-700">
                  <span>Workspace Quota</span>
                  <span className="font-mono text-emerald-700 font-bold">{servers.length} Slots In Use</span>
                </div>
                <div className="w-full bg-slate-100 h-1.5 rounded-full overflow-hidden">
                  <div
                    className="bg-emerald-500 h-full rounded-full transition-all"
                    style={{ width: `${Math.min(100, (servers.length / 8) * 100)}%` }}
                  />
                </div>
                <div className="flex items-center justify-between text-[10px] text-slate-400 font-mono">
                  <span>Cap 8 Slots · RAM 4.2 GB</span>
                  <span className="font-bold text-slate-600">PRO</span>
                </div>
              </div>
            ) : (
              <div className="w-full pt-3 border-t border-slate-100 flex justify-center text-[10px] font-mono font-bold text-emerald-600">
                {servers.length}
              </div>
            )}
          </aside>

          {/* Main Content Area */}
          <main className="flex-1 min-w-0">
            {pageAllowed(pathname, canAccessGameAssets, canCreateServer, canEditSettings) ? children : <PermissionDenied />}
          </main>
        </div>
      </div>

      {/* Incident & Notification Center Drawer */}
      <IncidentDrawer open={incidentOpen} onClose={() => setIncidentOpen(false)} />

      {/* Global Apps & Tools Drawer */}
      <AppsDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />

      {/* Account Settings Dialog */}
      {accountOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-xs p-4 animate-in fade-in duration-150">
          <div className="w-full max-w-md rounded-2xl border micro-border bg-white shadow-2xl p-6 space-y-5 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <h3 className="text-sm font-bold text-slate-900">{isZh ? "账号设置" : "Account Settings"}</h3>
              <button
                type="button"
                onClick={() => setAccountOpen(false)}
                className="text-slate-400 hover:text-slate-700 transition"
              >
                <X className="size-4" />
              </button>
            </div>

            {/* Tab switch */}
            <div className="flex border-b border-slate-100">
              <button
                type="button"
                className={cn(
                  "flex-1 pb-2.5 text-xs font-semibold transition border-b-2",
                  accountTab === "language"
                    ? "border-emerald-500 text-emerald-600 font-bold"
                    : "border-transparent text-slate-400 hover:text-slate-700"
                )}
                onClick={() => setAccountTab("language")}
              >
                {isZh ? "语言偏好" : "Language Preference"}
              </button>
              <button
                type="button"
                className={cn(
                  "flex-1 pb-2.5 text-xs font-semibold transition border-b-2",
                  accountTab === "password"
                    ? "border-emerald-500 text-emerald-600 font-bold"
                    : "border-transparent text-slate-400 hover:text-slate-700"
                )}
                onClick={() => setAccountTab("password")}
              >
                {isZh ? "账号安全" : "Account Security"}
              </button>
            </div>

            {accountMessage && (
              <p className="text-xs text-emerald-700 bg-emerald-50 border border-emerald-200 px-3 py-2 rounded-lg">
                {accountMessage}
              </p>
            )}

            {accountTab === "language" ? (
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-700">{isZh ? "语言选择" : "Select Language"}</label>
                  <select
                    value={selectedLocale}
                    onChange={(e) => setSelectedLocale(e.target.value as Locale)}
                    className="w-full rounded-lg border micro-border bg-slate-50/60 px-3 py-2 text-xs text-slate-900 focus:border-emerald-500 focus:bg-white focus:outline-hidden"
                  >
                    <option value="zh">简体中文 (Chinese Simplified)</option>
                    <option value="en">English (US)</option>
                  </select>
                </div>
                <div className="flex justify-end gap-2 pt-2">
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => setAccountOpen(false)}
                  >
                    {t("cancel")}
                  </Button>
                  <Button
                    type="button"
                    disabled={localeMutation.isPending}
                    onClick={saveLocale}
                  >
                    {localeMutation.isPending ? t("saving") : t("saveButton")}
                  </Button>
                </div>
              </div>
            ) : (
              <form onSubmit={submitPasswordChange} className="space-y-4">
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-700">{t("currentPassword")}</label>
                  <Input
                    type="password"
                    required
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    className="h-8 text-xs bg-slate-50 border-slate-200"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-700">{t("newPassword")}</label>
                  <Input
                    type="password"
                    required
                    minLength={8}
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="h-8 text-xs bg-slate-50 border-slate-200"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-700">{t("confirmNewPassword")}</label>
                  <Input
                    type="password"
                    required
                    minLength={8}
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    aria-invalid={passwordMismatch}
                    className={cn(
                      "h-8 text-xs bg-slate-50 border-slate-200",
                      passwordMismatch && "border-rose-500 focus-visible:ring-rose-500/30"
                    )}
                  />
                  {passwordMismatch && <p className="text-xs text-rose-500">{t("passwordsDoNotMatch")}</p>}
                </div>
                <div className="flex justify-end gap-2 pt-2">
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => setAccountOpen(false)}
                  >
                    {t("cancel")}
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      passwordMutation.isPending ||
                      !currentPassword ||
                      !newPassword ||
                      !confirmPassword ||
                      newPassword !== confirmPassword
                    }
                  >
                    {passwordMutation.isPending ? t("saving") : t("saveButton")}
                  </Button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function SidebarItem({
  href,
  active,
  icon,
  label,
  badge,
  badgeTone = "default",
  collapsed
}: {
  href: string;
  active: boolean;
  icon: ReactNode;
  label: string;
  badge?: string;
  badgeTone?: "default" | "purple";
  collapsed: boolean;
}) {
  return (
    <Link
      href={href}
      title={label}
      className={cn(
        "flex h-7 items-center gap-2 rounded-lg px-2 text-xs font-medium transition",
        active
          ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs"
          : "text-slate-600 hover:bg-slate-50 hover:text-slate-900",
        collapsed && "justify-center px-0"
      )}
    >
      <span className={cn(active ? "text-slate-900" : "text-slate-400 shrink-0")}>{icon}</span>
      {!collapsed && (
        <>
          <span className="truncate flex-1">{label}</span>
          {badge && (
            <span
              className={cn(
                "rounded px-1 py-0.2 font-mono text-[10px] font-medium border",
                badgeTone === "purple"
                  ? "bg-purple-50 text-purple-700 border-purple-200"
                  : "bg-white text-slate-700 border-slate-200"
              )}
            >
              {badge}
            </span>
          )}
        </>
      )}
    </Link>
  );
}

function pageAllowed(pathname: string, canAccessGameAssets: boolean, canCreateServer: boolean, canEditSettings: boolean) {
  if (pathname.startsWith("/servers/new")) return canCreateServer;
  if (pathname.startsWith("/settings") || pathname.startsWith("/versions")) {
    return canEditSettings;
  }
  if (["/games", "/mods", "/presets", "/worlds", "/backups"].some((path) => pathname.startsWith(path))) {
    return canAccessGameAssets;
  }
  if (pathname.startsWith("/activity")) return canAccessGameAssets;
  return true;
}
