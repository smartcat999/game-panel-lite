"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import { Activity, Archive, Bookmark, Box, Gauge, Gamepad2, Globe2, HardDrive, LogOut, Plus, Search, Settings, ShieldCheck } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { getApiHealth, getAuthBootstrap, listServers, logoutAdmin } from "@/lib/api";
import { serverProviderDisplay, serverResourceLabelKey } from "@/lib/server-display";
import { serverJoinPort } from "@/lib/server-join";

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
  const searchRef = useRef<HTMLFormElement>(null);
  const profileRef = useRef<HTMLDivElement>(null);
  const apiHealth = useQuery({ queryKey: ["api-health"], queryFn: getApiHealth, retry: false, refetchInterval: 10000 });
  const authQuery = useQuery({ queryKey: ["auth-bootstrap"], queryFn: getAuthBootstrap, retry: false, staleTime: 30000 });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false, enabled: searchOpen || search.trim().length > 0 });

  const logoutMutation = useMutation({
    mutationFn: logoutAdmin,
    onSuccess: async () => {
      setProfileOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
      router.push("/dashboard");
    }
  });
  const serviceAvailable = apiHealth.data?.status === "ok";
  const serviceLabel = serviceAvailable ? t("online") : apiHealth.isLoading ? t("dockerCheckingShort") : t("unavailable");
  const searchTerm = search.trim().toLowerCase();
  const searchResults = useMemo(() => {
    if (!searchTerm) return [];
    return (serversQuery.data ?? [])
      .filter((server) => {
        const provider = serverProviderDisplay(server);
        const resourceLabel = t(serverResourceLabelKey(server));
        return [server.name, server.world, String(serverJoinPort(server)), String(server.port), server.mode, provider.label, resourceLabel]
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
    nav.forEach((item) => router.prefetch(item.href));
    router.prefetch("/servers/new");
  }, [router]);

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
          {nav.map((item) => {
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
              <div
                className="hidden w-[104px] shrink-0 items-center gap-1 rounded-md border border-panel-line bg-slate-950/60 p-1 text-xs md:flex"
                aria-label={t("language")}
              >
                <button
                  className={cn(
                    "w-12 rounded px-2 py-1 text-center text-slate-300 transition-colors",
                    locale === "zh" && "bg-panel-green text-slate-950"
                  )}
                  type="button"
                  onClick={() => setLocale("zh")}
                >
                  {t("chinese")}
                </button>
                <button
                  className={cn(
                    "w-9 rounded px-2 py-1 text-center text-slate-300 transition-colors",
                    locale === "en" && "bg-panel-green text-slate-950"
                  )}
                  type="button"
                  onClick={() => setLocale("en")}
                >
                  {t("english")}
                </button>
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
        <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-panel-line bg-panel-bg/95 px-2 py-2 backdrop-blur lg:hidden" aria-label="Mobile navigation">
          <div className="grid grid-cols-5 gap-1">
            {nav.slice(0, 5).map((item) => {
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

function SearchServerResult({ server, onOpen }: { server: Awaited<ReturnType<typeof listServers>>[number]; onOpen: (id: string) => void }) {
  const { t } = useI18n();
  const provider = serverProviderDisplay(server);
  const resourceLabel = t(serverResourceLabelKey(server));
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition hover:bg-slate-800/80"
      onClick={() => onOpen(server.id)}
    >
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium text-white">{server.name}</span>
        <span className="block truncate text-xs text-slate-500">{resourceLabel}: {server.world} · {serverJoinPort(server)}</span>
      </span>
      <span className="shrink-0 rounded bg-slate-800 px-2 py-1 text-xs text-slate-300">{provider.label}</span>
    </button>
  );
}
