"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import { Activity, Archive, Bookmark, Box, Gauge, Gamepad2, Globe2, HardDrive, KeyRound, Languages, LogOut, Plus, Search, Settings, ShieldCheck, UserCog, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useI18n, type Locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { changeAdminPassword, getApiHealth, getAuthBootstrap, getSettings, listGameServers, logoutAdmin, updateLocale } from "@/lib/api";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameServerJoinPort, gameServerMode, gameServerSearchText, gameServerWorldName } from "@/lib/game-server-resource";
import { serverProviderDisplay, serverResourceLabelKey } from "@/lib/server-display";
import type { GameServerResource } from "@/lib/types";

const nav = [
  { href: "/dashboard", labelKey: "navDashboard", icon: Gauge },
  { href: "/games", labelKey: "navGames", icon: Gamepad2 },
  { href: "/servers", labelKey: "navServers", icon: HardDrive },
  { href: "/worlds", labelKey: "navWorlds", icon: Globe2 },
  { href: "/mods", labelKey: "navMods", icon: Box },
  { href: "/presets", labelKey: "navPresets", icon: Bookmark },
  { href: "/backups", labelKey: "navBackups", icon: Archive },
  { href: "/activity", labelKey: "navActivity", icon: Activity },
  { href: "/settings", labelKey: "navSettings", icon: Settings }
] as const;

const visibleNav = nav.filter((item) => showWorldAndBackupFeatures || (item.href !== "/worlds" && item.href !== "/backups"));

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
  const [createPending, setCreatePending] = useState(false);
  const [search, setSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [accountTab, setAccountTab] = useState<"language" | "password">("language");
  const [selectedLocale, setSelectedLocale] = useState<Locale>(locale);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [accountMessage, setAccountMessage] = useState("");
  const searchRef = useRef<HTMLFormElement>(null);
  const profileRef = useRef<HTMLDivElement>(null);
  const apiHealth = useQuery({ queryKey: ["api-health"], queryFn: getApiHealth, retry: false, refetchInterval: 10000 });
  const authQuery = useQuery({ queryKey: ["auth-bootstrap"], queryFn: getAuthBootstrap, retry: false, staleTime: 30000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 30000 });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, enabled: searchOpen || search.trim().length > 0 });

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
  const serviceAvailable = apiHealth.data?.status === "ok";
  const serviceLabel = serviceAvailable ? t("online") : apiHealth.isLoading ? t("dockerCheckingShort") : t("unavailable");
  const searchTerm = search.trim().toLowerCase();
  const searchResults = useMemo(() => {
    if (!searchTerm) return [];
    return (serversQuery.data ?? [])
      .filter((server) => {
        const provider = serverProviderDisplay({ providerKey: server.providerKey, mode: gameServerMode(server) });
        const resourceLabel = t(serverResourceLabelKey(server));
        return [...gameServerSearchText(server, provider.label), resourceLabel]
          .some((value) => value.toLowerCase().includes(searchTerm));
      })
      .slice(0, 5);
  }, [searchTerm, serversQuery.data, t]);

  useEffect(() => {
    setCreatePending(false);
    setProfileOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!createPending) return;
    const timeout = window.setTimeout(() => setCreatePending(false), 2000);
    return () => window.clearTimeout(timeout);
  }, [createPending]);

  useEffect(() => {
    visibleNav.forEach((item) => router.prefetch(item.href));
    router.prefetch("/servers/new");
  }, [router]);

  useEffect(() => {
    if (settingsQuery.data?.locale) {
      setLocale(settingsQuery.data.locale);
      setSelectedLocale(settingsQuery.data.locale);
    }
  }, [settingsQuery.data?.locale, setLocale]);

  useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (!searchRef.current?.contains(event.target as Node)) {
        setSearchOpen(false);
      }
      if (!profileRef.current?.contains(event.target as Node)) {
        setProfileOpen(false);
      }
    };
    window.addEventListener("pointerdown", handlePointerDown);
    return () => window.removeEventListener("pointerdown", handlePointerDown);
  }, []);

  useEffect(() => {
    if (!accountOpen) return;
    const originalOverflow = document.body.style.overflow;
    const originalPaddingRight = document.body.style.paddingRight;
    const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
    document.body.style.overflow = "hidden";
    if (scrollbarWidth > 0) {
      document.body.style.paddingRight = `${scrollbarWidth}px`;
    }
    return () => {
      document.body.style.overflow = originalOverflow;
      document.body.style.paddingRight = originalPaddingRight;
    };
  }, [accountOpen]);

  const openServer = (id: string) => {
    setSearchOpen(false);
    setSearch("");
    router.push(`/servers/${id}`);
  };

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (searchResults[0]) {
      openServer(searchResults[0].id);
      return;
    }
    if (searchTerm) {
      router.push(`/servers?search=${encodeURIComponent(search.trim())}`);
      setSearchOpen(false);
    }
  };

  const openAccountSettings = (tab: "language" | "password" = "language") => {
    setAccountTab(tab);
    setSelectedLocale(locale);
    setAccountMessage("");
    setProfileOpen(false);
    setAccountOpen(true);
  };

  const saveLocale = () => {
    setAccountMessage("");
    localeMutation.mutate(selectedLocale);
  };

  const submitPasswordChange = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setAccountMessage("");
    passwordMutation.mutate();
  };

  return (
    <div className="min-h-screen bg-panel-bg text-slate-100">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-line bg-panel-sidebar lg:flex lg:flex-col">
        <Link href="/dashboard" className="flex h-20 items-center gap-3 px-6 pt-2">
          <span className="flex size-9 items-center justify-center rounded-md bg-panel-green text-slate-950">
            <Gamepad2 aria-hidden="true" />
          </span>
          <span className="font-semibold">GamePanel Lite</span>
        </Link>
        <nav className="flex flex-1 flex-col gap-1 px-3 py-4">
          {visibleNav.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                onMouseEnter={() => router.prefetch(item.href)}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-3 text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white",
                  active && "bg-slate-800/80 text-white"
                )}
              >
                <Icon aria-hidden="true" />
                {t(item.labelKey)}
              </Link>
            );
          })}
        </nav>
        <div className="m-4 overflow-hidden rounded-lg border border-panel-line bg-slate-950/40 p-3">
          <div className="h-20 overflow-hidden rounded-md border border-panel-line bg-slate-950">
            <Image
              src="/images/terraria-official-cover.jpg"
              alt=""
              width={1200}
              height={1800}
              className="h-full w-full object-cover object-[50%_42%]"
              priority
            />
          </div>
        </div>
      </aside>
      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-panel-line bg-panel-bg/95 px-4 py-3 backdrop-blur md:px-8">
          <div className="flex items-center gap-4">
            <form ref={searchRef} className="relative max-w-md flex-1" onSubmit={submitSearch}>
              <Search className="pointer-events-none absolute left-3 top-2.5 text-slate-500" aria-hidden="true" />
              <Input
                aria-label={t("searchServers")}
                autoComplete="off"
                className="w-full pl-10"
                placeholder={t("searchServers")}
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setSearchOpen(true);
                }}
                onFocus={() => setSearchOpen(true)}
              />
              {searchOpen && search.trim().length > 0 && (
                <div className="absolute left-0 right-0 top-12 z-30 overflow-hidden rounded-lg border border-panel-line bg-panel-card shadow-[0_12px_32px_rgba(0,0,0,0.32)]">
                  {serversQuery.isLoading ? (
                    <p className="px-3 py-3 text-sm text-slate-400">{t("loading")}</p>
                  ) : serversQuery.isError ? (
                    <p className="px-3 py-3 text-sm text-panel-gold">{t("apiServersUnavailable")}</p>
                  ) : searchResults.length === 0 ? (
                    <p className="px-3 py-3 text-sm text-slate-400">{t("noSearchResults")}</p>
                  ) : (
                    <div className="py-1">
                      {searchResults.map((server) => (
                        <SearchServerResult key={server.id} server={server} onOpen={openServer} />
                      ))}
                    </div>
                  )}
                </div>
              )}
            </form>
            <div className="ml-auto flex shrink-0 items-center gap-3">
              <div className="hidden shrink-0 items-center gap-2 text-xs text-slate-300 sm:flex">
                <span
                  className={cn(
                    "inline-flex items-center gap-1 whitespace-nowrap rounded border border-panel-line bg-slate-950/45 px-2 py-1",
                    serviceAvailable ? "text-panel-green" : "text-panel-gold"
                  )}
                >
                  <ShieldCheck aria-hidden="true" className="size-4" />
                  <span>{serviceLabel}</span>
                </span>
              </div>
              <Link
                href="/servers/new"
                aria-label={t("createServer")}
                className="shrink-0"
                onClick={() => setCreatePending(true)}
                onMouseEnter={() => router.prefetch("/servers/new")}
              >
                <Button className="h-11 w-11 shrink-0 whitespace-nowrap sm:h-12 sm:w-44">
                  <Plus aria-hidden="true" />
                  <span className="hidden sm:inline">{createPending ? t("openingCreateServer") : t("createServer")}</span>
                </Button>
              </Link>
              <div ref={profileRef} className="relative hidden md:block">
                <button
                  type="button"
                  aria-expanded={profileOpen}
                  aria-label={t("userProfile")}
                  className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full border border-panel-line bg-slate-950/70 p-0.5 shadow-[0_0_0_1px_rgba(123,217,120,0.08)] transition hover:border-panel-green focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                  onClick={() => setProfileOpen((value) => !value)}
                >
                  <Image
                    src="/images/user-avatar.svg"
                    alt={t("userAvatarAlt")}
                    width={80}
                    height={80}
                    className="size-full rounded-full object-cover"
                  />
                </button>
                {profileOpen && (
                  <div className="absolute right-0 top-12 z-30 w-56 rounded-lg border border-panel-line bg-panel-card p-3 shadow-[0_12px_32px_rgba(0,0,0,0.32)]">
                    <div className="flex items-center gap-3">
                      <Image
                        src="/images/user-avatar.svg"
                        alt={t("userAvatarAlt")}
                        width={80}
                        height={80}
                        className="size-10 rounded-full border border-panel-line bg-slate-950"
                      />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-white">{authQuery.data?.account?.username ?? t("localUser")}</p>
                        <p className="text-xs text-slate-500">{t("localProfileDescription")}</p>
                      </div>
                    </div>
                    <div className="mt-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2 text-xs text-slate-400">
                      GamePanel Lite v1.0.0
                    </div>
                    <button
                      type="button"
                      className="mt-2 flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white"
                      onClick={() => openAccountSettings("language")}
                    >
                      <UserCog aria-hidden="true" className="size-4" />
                      {t("accountSettings")}
                    </button>
                    <button
                      type="button"
                      className="mt-2 flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white disabled:cursor-not-allowed disabled:opacity-60"
                      onClick={() => logoutMutation.mutate()}
                      disabled={logoutMutation.isPending}
                    >
                      <LogOut aria-hidden="true" className="size-4" />
                      {logoutMutation.isPending ? t("loggingOut") : t("logout")}
                    </button>
                  </div>
                )}
              </div>
            </div>
          </div>
        </header>
        <main className="px-4 py-6 pb-24 md:px-8 lg:pb-6">{children}</main>
        {accountOpen ? (
          <AccountSettingsDialog
            activeTab={accountTab}
            currentPassword={currentPassword}
            locale={locale}
            selectedLocale={selectedLocale}
            message={accountMessage}
            newPassword={newPassword}
            passwordPending={passwordMutation.isPending}
            localePending={localeMutation.isPending}
            onChangeLocale={setSelectedLocale}
            onChangeCurrentPassword={setCurrentPassword}
            onChangeNewPassword={setNewPassword}
            onClose={() => setAccountOpen(false)}
            onSaveLocale={saveLocale}
            onSubmitPassword={submitPasswordChange}
            onTabChange={(tab) => {
              setAccountTab(tab);
              setAccountMessage("");
            }}
          />
        ) : null}
        <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-panel-line bg-panel-bg/95 px-2 py-2 backdrop-blur lg:hidden" aria-label="Mobile navigation">
          <div className="grid grid-cols-5 gap-1">
            {visibleNav.slice(0, 5).map((item) => {
              const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "flex min-w-0 flex-col items-center justify-center gap-1 rounded-md px-1 py-2 text-[11px] font-medium text-slate-500 transition hover:bg-slate-800 hover:text-white",
                    active && "bg-panel-green/15 text-panel-green"
                  )}
                >
                  <Icon aria-hidden="true" className="size-5" />
                  <span className="max-w-full truncate">{t(item.labelKey)}</span>
                </Link>
              );
            })}
          </div>
        </nav>
      </div>
    </div>
  );
}

function AccountSettingsDialog({
  activeTab,
  currentPassword,
  locale,
  localePending,
  message,
  newPassword,
  passwordPending,
  selectedLocale,
  onChangeLocale,
  onChangeCurrentPassword,
  onChangeNewPassword,
  onClose,
  onSaveLocale,
  onSubmitPassword,
  onTabChange
}: {
  activeTab: "language" | "password";
  currentPassword: string;
  locale: Locale;
  localePending: boolean;
  message: string;
  newPassword: string;
  passwordPending: boolean;
  selectedLocale: Locale;
  onChangeLocale: (locale: Locale) => void;
  onChangeCurrentPassword: (value: string) => void;
  onChangeNewPassword: (value: string) => void;
  onClose: () => void;
  onSaveLocale: () => void;
  onSubmitPassword: (event: FormEvent<HTMLFormElement>) => void;
  onTabChange: (tab: "language" | "password") => void;
}) {
  const { t } = useI18n();
  const localeChanged = selectedLocale !== locale;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-slate-950/72 px-4 py-6 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="account-settings-title">
      <div className="w-[min(720px,calc(100vw-32px))] overflow-hidden rounded-lg border border-panel-line bg-panel-card shadow-[0_24px_72px_rgba(0,0,0,0.55)]">
        <div className="flex min-h-[81px] items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
          <div className="min-w-0">
            <h2 id="account-settings-title" className="font-semibold text-white">{t("accountSettings")}</h2>
            <p className="mt-1 line-clamp-2 text-sm text-slate-400">{t("accountSettingsDescription")}</p>
          </div>
          <button
            aria-label={t("close")}
            className="flex size-9 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            type="button"
            onClick={onClose}
          >
            <X aria-hidden="true" className="size-5" />
          </button>
        </div>
        <div className="grid min-h-[360px] md:grid-cols-[192px_minmax(0,1fr)]">
          <nav className="border-b border-panel-line bg-slate-950/25 p-3 md:border-b-0 md:border-r" aria-label={t("accountSettings")}>
            <AccountTabButton active={activeTab === "language"} icon={<Languages aria-hidden="true" className="size-4" />} label={t("appearanceTitle")} onClick={() => onTabChange("language")} />
            <AccountTabButton active={activeTab === "password"} icon={<KeyRound aria-hidden="true" className="size-4" />} label={t("changePassword")} onClick={() => onTabChange("password")} />
          </nav>
          <div className="min-w-0 p-5">
            {activeTab === "language" ? (
              <div>
                <h3 className="font-semibold text-white">{t("appearanceTitle")}</h3>
                <p className="mt-1 min-h-10 max-w-xl text-sm text-slate-400">{t("appearanceDescription")}</p>
                <div className="mt-5 inline-flex rounded-md border border-panel-line bg-slate-950/60 p-1 text-sm" aria-label={t("language")}>
                  <button
                    className={cn("min-w-24 rounded px-3 py-2 text-center text-slate-300 transition-colors", selectedLocale === "zh" && "bg-panel-green text-slate-950")}
                    type="button"
                    disabled={localePending}
                    onClick={() => onChangeLocale("zh")}
                  >
                    {t("chinese")}
                  </button>
                  <button
                    className={cn("min-w-24 rounded px-3 py-2 text-center text-slate-300 transition-colors", selectedLocale === "en" && "bg-panel-green text-slate-950")}
                    type="button"
                    disabled={localePending}
                    onClick={() => onChangeLocale("en")}
                  >
                    {t("languageEnglish")}
                  </button>
                </div>
                <div className="mt-5 flex min-h-10 flex-wrap items-center gap-3">
                  {localeChanged ? (
                    <>
                      <Button className="px-4" type="button" disabled={localePending} onClick={onSaveLocale}>
                        {localePending ? t("saving") : t("saveButton")}
                      </Button>
                      <button
                        className="rounded-md px-3 py-2 text-sm text-slate-400 transition hover:bg-slate-800 hover:text-white"
                        type="button"
                        disabled={localePending}
                        onClick={() => onChangeLocale(locale)}
                      >
                        {t("cancel")}
                      </button>
                    </>
                  ) : null}
                </div>
              </div>
            ) : (
              <form className="max-w-md space-y-3" onSubmit={onSubmitPassword}>
                <div>
                  <h3 className="font-semibold text-white">{t("changePassword")}</h3>
                  <p className="mt-1 text-sm text-slate-400">{t("localAdminDescription")}</p>
                </div>
                <label className="block">
                  <span className="text-xs font-medium text-slate-500">{t("currentPassword")}</span>
                  <Input
                    className="mt-2 w-full"
                    type="password"
                    value={currentPassword}
                    onChange={(event) => onChangeCurrentPassword(event.target.value)}
                    autoComplete="current-password"
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-medium text-slate-500">{t("newPassword")}</span>
                  <Input
                    className="mt-2 w-full"
                    type="password"
                    value={newPassword}
                    onChange={(event) => onChangeNewPassword(event.target.value)}
                    autoComplete="new-password"
                  />
                </label>
                <Button className="px-4" type="submit" disabled={passwordPending}>
                  {passwordPending ? t("saving") : t("changePassword")}
                </Button>
              </form>
            )}
            {message ? <p className="mt-5 rounded-md border border-panel-line bg-slate-950/40 px-3 py-2 text-sm text-slate-300">{message}</p> : null}
          </div>
        </div>
      </div>
    </div>
  );
}

function AccountTabButton({ active, icon, label, onClick }: { active: boolean; icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      className={cn(
        "flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm text-slate-400 transition hover:bg-slate-800 hover:text-white",
        active && "bg-panel-green/15 text-panel-green"
      )}
      type="button"
      onClick={onClick}
    >
      <span className="shrink-0">{icon}</span>
      <span className="truncate">{label}</span>
    </button>
  );
}

function SearchServerResult({ server, onOpen }: { server: GameServerResource; onOpen: (id: string) => void }) {
  const { t } = useI18n();
  const provider = serverProviderDisplay({ providerKey: server.providerKey, mode: gameServerMode(server) });
  const resourceLabel = t(serverResourceLabelKey(server));
  const joinPort = gameServerJoinPort(server);
  const meta = showWorldAndBackupFeatures ? `${resourceLabel}: ${gameServerWorldName(server)} · ${joinPort}` : `${provider.label} · ${joinPort}`;
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition hover:bg-slate-800/80"
      onClick={() => onOpen(server.id)}
    >
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium text-white">{server.name}</span>
        <span className="block truncate text-xs text-slate-500">{meta}</span>
      </span>
      <span className="shrink-0 rounded bg-slate-800 px-2 py-1 text-xs text-slate-300">{provider.label}</span>
    </button>
  );
}
