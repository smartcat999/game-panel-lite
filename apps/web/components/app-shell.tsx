"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import {
  Gamepad2,
  KeyRound,
  Languages,
  LayoutGrid,
  LogOut,
  UserCog,
  X
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useI18n, type Locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { TopNav } from "@/components/top-nav";
import { AppsDrawer } from "@/components/apps-drawer";
import { ClusterStatusPill } from "@/components/cluster-status-pill";
import { ClusterFleetPopover } from "@/components/cluster-fleet-popover";
import { PermissionDenied } from "@/components/permission-denied";
import { usePermissions } from "@/lib/permissions";
import {
  changeAdminPassword,
  getAuthBootstrap,
  getSettings,
  logoutAdmin,
  updateLocale
} from "@/lib/api";

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/")) {
    return <>{children}</>;
  }
  return <AppChrome>{children}</AppChrome>;
}

function AppChrome({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale, setLocale, t } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canAccessGameAssets, canCreateServer, canEditSettings } = usePermissions();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [accountTab, setAccountTab] = useState<"language" | "password">("language");
  const [selectedLocale, setSelectedLocale] = useState<Locale>(locale);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [accountMessage, setAccountMessage] = useState("");

  const profileRef = useRef<HTMLDivElement>(null);

  const authQuery = useQuery({ queryKey: ["auth-bootstrap"], queryFn: getAuthBootstrap, retry: false, staleTime: 30000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 30000 });

  const logoutMutation = useMutation({
    mutationFn: logoutAdmin,
    onSuccess: async () => {
      setProfileOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
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
      setAccountMessage(t("passwordChanged"));
    },
    onError: (err) => setAccountMessage(err instanceof Error ? err.message : t("passwordChangeFailed"))
  });

  useEffect(() => {
    setProfileOpen(false);
    setDrawerOpen(false);
  }, [pathname]);

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
    passwordMutation.mutate();
  };

  return (
    <div className="min-h-screen bg-[#070b12] text-slate-100 selection:bg-panel-green/30">
      {/* Top Global Command Header */}
      <header className="sticky top-0 z-50 h-14 border-b border-slate-800/80 bg-[#090d16]/95 backdrop-blur-xl px-4 sm:px-6 lg:px-8">
        <div className="flex h-full items-center justify-between gap-4">
          {/* Left: Brand + Standalone Host Badge */}
          <div className="flex items-center gap-3 shrink-0">
            <Link
              href="/dashboard"
              className="flex items-center gap-2 text-sm font-bold tracking-tight text-white hover:opacity-90 transition"
            >
              <div className="flex size-7 items-center justify-center rounded-lg bg-panel-green/15 text-panel-green border border-panel-green/30 shadow-xs">
                <Gamepad2 className="size-4" />
              </div>
              <span className="font-bold tracking-tight">GamePanel <span className="text-panel-green font-mono text-xs">Lite</span></span>
              <span className="relative flex size-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-panel-green opacity-75" />
                <span className="relative inline-flex size-2 rounded-full bg-panel-green" />
              </span>
            </Link>

            {/* Cluster Fleet Interactive Popover Hub */}
            <ClusterFleetPopover />
          </div>

          {/* Right: Pure Icon TopNav + Cluster Status Pill + Apps Drawer + Profile */}
          <div className="flex items-center gap-2 sm:gap-3 shrink-0">
            {/* Main Icon Navigation (Positioned on the right) */}
            <TopNav />

            <div className="h-4 w-px bg-slate-800/80 hidden sm:block" />

            {/* Cluster Real-Time Metrics Mini-Pill */}
            <ClusterStatusPill />

            {/* Apps & Tools Drawer Button */}
            <button
              type="button"
              onClick={() => setDrawerOpen(true)}
              aria-label="All Apps and Navigation"
              title="All Apps (⌘B / [)"
              className="flex size-8 items-center justify-center rounded-lg border border-slate-800 bg-slate-950/80 text-slate-400 hover:border-slate-600 hover:text-slate-100 transition focus:outline-none focus:ring-1 focus:ring-panel-green/50"
            >
              <LayoutGrid className="size-4" />
            </button>

            {/* Profile Menu */}
            <div ref={profileRef} className="relative">
              <button
                type="button"
                aria-expanded={profileOpen}
                aria-label={t("userProfile")}
                className="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-slate-800 bg-slate-950 p-0.5 transition hover:border-panel-green focus:outline-none focus:ring-1 focus:ring-panel-green/50"
                onClick={() => setProfileOpen((value) => !value)}
              >
                <Image
                  src="/images/user-avatar.svg"
                  alt={t("userAvatarAlt")}
                  width={40}
                  height={40}
                  className="size-full rounded-full object-cover"
                />
              </button>

              {profileOpen && (
                <div className="absolute right-0 top-10 z-40 w-60 rounded-xl border border-slate-700 bg-slate-900 p-3 shadow-2xl ring-1 ring-white/10 backdrop-blur-xl">
                  <div className="flex items-center gap-3 border-b border-slate-800 pb-3">
                    <Image
                      src="/images/user-avatar.svg"
                      alt={t("userAvatarAlt")}
                      width={40}
                      height={40}
                      className="size-9 rounded-full border border-slate-700 bg-slate-950"
                    />
                    <div className="min-w-0">
                      <p className="truncate text-xs font-bold text-white">
                        {authQuery.data?.account?.username ?? t("localUser")}
                      </p>
                      <p className="text-[10px] text-panel-green font-mono mt-0.5 font-bold uppercase">
                        {authQuery.data?.account?.role === "admin"
                          ? (isZh ? "超级管理员 (Admin)" : "Administrator")
                          : authQuery.data?.account?.role === "viewer"
                          ? (isZh ? "只读访客 (Viewer)" : "Viewer")
                          : (isZh ? "开黑成员 (Member)" : "Member")}
                      </p>
                    </div>
                  </div>

                  <div className="mt-2 space-y-1">
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 transition hover:bg-slate-800 hover:text-white"
                      onClick={() => openAccountSettings("language")}
                    >
                      <Languages className="size-3.5 text-sky-400" />
                      <span>{isZh ? "语言偏好" : "Language Preference"}</span>
                    </button>
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 transition hover:bg-slate-800 hover:text-white"
                      onClick={() => openAccountSettings("password")}
                    >
                      <KeyRound className="size-3.5 text-panel-gold" />
                      <span>{isZh ? "账号安全" : "Account Security"}</span>
                    </button>
                  </div>

                  <div className="mt-2 border-t border-slate-800 pt-2">
                    <button
                      type="button"
                      disabled={logoutMutation.isPending}
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-rose-400 transition hover:bg-rose-950/40 hover:text-rose-300"
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

      {/* Main Content Area */}
      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        {pageAllowed(pathname, canAccessGameAssets, canCreateServer, canEditSettings) ? children : <PermissionDenied />}
      </main>

      {/* Apps and Quick Navigation Drawer */}
      <AppsDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />

      {/* Account Settings Dialog */}
      {accountOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4">
          <div className="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 shadow-2xl p-6 space-y-5">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <UserCog className="size-5 text-panel-green" />
                <h3 className="text-sm font-bold text-white">{isZh ? "账号设置" : "Account Settings"}</h3>
              </div>
              <button
                type="button"
                onClick={() => setAccountOpen(false)}
                className="text-slate-400 hover:text-white transition"
              >
                <X className="size-4" />
              </button>
            </div>

            {/* Tab switch */}
            <div className="flex border-b border-slate-800">
              <button
                type="button"
                className={cn(
                  "flex-1 pb-2.5 text-xs font-semibold transition border-b-2",
                  accountTab === "language"
                    ? "border-panel-green text-panel-green"
                    : "border-transparent text-slate-400 hover:text-slate-200"
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
                    ? "border-panel-green text-panel-green"
                    : "border-transparent text-slate-400 hover:text-slate-200"
                )}
                onClick={() => setAccountTab("password")}
              >
                {isZh ? "账号安全" : "Account Security"}
              </button>
            </div>

            {accountMessage && (
              <p className="text-xs text-panel-green bg-panel-green/10 border border-panel-green/30 px-3 py-2 rounded-lg">
                {accountMessage}
              </p>
            )}

            {accountTab === "language" ? (
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-300">{isZh ? "语言选择" : "Select Language"}</label>
                  <select
                    value={selectedLocale}
                    onChange={(e) => setSelectedLocale(e.target.value as Locale)}
                    className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-white focus:border-panel-green focus:outline-none"
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
                    {localeMutation.isPending ? t("saving") : t("save")}
                  </Button>
                </div>
              </div>
            ) : (
              <form onSubmit={submitPasswordChange} className="space-y-4">
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-300">{t("currentPassword")}</label>
                  <Input
                    type="password"
                    required
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    className="h-8 text-xs bg-slate-950 border-slate-700"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-300">{t("newPassword")}</label>
                  <Input
                    type="password"
                    required
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="h-8 text-xs bg-slate-950 border-slate-700"
                  />
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
                    disabled={passwordMutation.isPending}
                  >
                    {passwordMutation.isPending ? t("saving") : t("save")}
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
