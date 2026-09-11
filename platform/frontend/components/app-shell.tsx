"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Archive,
  Bell,
  Boxes,
  Building2,
  Check,
  ChevronsUpDown,
  CircleDollarSign,
  Database,
  Gauge,
  HardDrive,
  ListChecks,
  LogOut,
  Menu,
  Network,
  Server,
  Settings,
  ShieldCheck,
  UserRound,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";

import { AccountModal } from "@/components/account-modal";
import { api, type Workspace } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Area = "workspace" | "platform" | "region";
type NavItem = { label: string; href: string; icon: LucideIcon };
type Translate = ReturnType<typeof useI18n>["t"];

function workspaceNavigation(slug: string, t: Translate): Array<{ group: string; items: NavItem[] }> {
  return [
    {
      group: t("nav.compute"),
      items: [{ label: t("nav.instances"), href: `/w/${slug}/instances`, icon: Server }],
    },
    {
      group: t("nav.data"),
      items: [{ label: t("nav.backups"), href: `/w/${slug}/backups`, icon: Archive }],
    },
    {
      group: t("nav.workspace"),
      items: [{ label: t("nav.billing"), href: `/w/${slug}/billing`, icon: CircleDollarSign }],
    },
  ];
}

function platformNavigation(t: Translate): Array<{ group: string; items: NavItem[] }> {
  return [
    {
      group: t("nav.platform"),
      items: [
        { label: t("nav.regions"), href: "/platform/regions", icon: Network },
        { label: t("nav.instances"), href: "/platform/instances", icon: Boxes },
        { label: t("nav.workspaces"), href: "/platform/workspaces", icon: Building2 },
        { label: t("nav.users"), href: "/platform/users", icon: Users },
      ],
    },
    {
      group: t("nav.commerce"),
      items: [
        { label: t("nav.creditGrants"), href: "/platform/credit-grants", icon: CircleDollarSign },
        { label: t("nav.priceBooks"), href: "/platform/price-books", icon: Database },
      ],
    },
    { group: t("nav.security"), items: [{ label: t("nav.audit"), href: "/platform/audit", icon: ShieldCheck }] },
  ];
}

function regionNavigation(regionId: string, t: Translate): Array<{ group: string; items: NavItem[] }> {
  return [
    {
      group: t("nav.regions"),
      items: [
        { label: t("nav.nodes"), href: `/platform/regions/${regionId}/nodes`, icon: Server },
        { label: t("nav.deployments"), href: `/platform/regions/${regionId}/deployments`, icon: Boxes },
        { label: t("nav.tasks"), href: `/platform/regions/${regionId}/tasks`, icon: ListChecks },
        { label: t("nav.capacity"), href: `/platform/regions/${regionId}/capacity`, icon: Gauge },
        { label: t("nav.storage"), href: `/platform/regions/${regionId}/storage`, icon: HardDrive },
        { label: t("nav.monitoring"), href: `/platform/regions/${regionId}/monitoring`, icon: Bell },
      ],
    },
  ];
}

export function AppShell({
  area,
  scope,
  workspaceSlug = "ember",
  children,
}: {
  area: Area;
  scope?: string;
  workspaceSlug?: string;
  children: ReactNode;
}) {
  const { t } = useI18n();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [workspaceMenuOpen, setWorkspaceMenuOpen] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [accountModalOpen, setAccountModalOpen] = useState(false);
  const pathname = usePathname();

  const workspaceRef = useRef<HTMLDivElement>(null);
  const userRef = useRef<HTMLDivElement>(null);

  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/workspaces"),
    enabled: area === "workspace",
  });

  const currentWorkspace = workspaces.data?.find((w) => w.slug === workspaceSlug);
  const scopeName =
    area === "workspace"
      ? currentWorkspace?.name ?? workspaceSlug
      : area === "platform"
      ? t("shell.platformConsole")
      : scope ?? "cn-east-1";

  const groups =
    area === "workspace"
      ? workspaceNavigation(workspaceSlug, t)
      : area === "platform"
      ? platformNavigation(t)
      : regionNavigation(scope ?? "cn-east-1", t);

  // Close menus when clicking outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (workspaceRef.current && !workspaceRef.current.contains(e.target as Node)) {
        setWorkspaceMenuOpen(false);
      }
      if (userRef.current && !userRef.current.contains(e.target as Node)) {
        setUserMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);


  const sidebarContent = (
    <div className="flex flex-col h-full select-none">
      {/* 1. SIDEBAR TOP: Workspace Switcher Card */}
      <div className="p-3 border-b border-white/[0.06] relative" ref={workspaceRef}>
        <button
          type="button"
          onClick={() => setWorkspaceMenuOpen(!workspaceMenuOpen)}
          className="w-full flex items-center justify-between p-2 rounded-lg bg-white/[0.03] hover:bg-white/[0.06] border border-white/[0.08] transition-all group text-left"
        >
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="w-7 h-7 rounded-md bg-emerald-500/15 border border-emerald-500/30 flex items-center justify-center shrink-0 shadow-sm">
              <span className="font-mono text-xs font-bold text-emerald-400">
                {scopeName.slice(0, 1).toUpperCase()}
              </span>
            </div>
            <div className="min-w-0 flex flex-col">
              <div className="flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.8)]" />
                <span className="text-xs font-semibold text-zinc-100 truncate tracking-tight">
                  {scopeName}
                </span>
              </div>
              <span className="text-[10px] text-zinc-400 font-mono tracking-wider truncate">
                {area === "workspace" ? "GAMING CLOUD" : "PLATFORM CORE"}
              </span>
            </div>
          </div>
          <ChevronsUpDown className="w-3.5 h-3.5 text-zinc-400 group-hover:text-zinc-200 shrink-0 ml-1 transition-colors" />
        </button>

        {/* Workspace Dropdown Flyout */}
        {workspaceMenuOpen && (
          <div className="absolute top-[calc(100%+4px)] left-3 right-3 z-50 p-1.5 rounded-xl bg-[#12161f] border border-white/[0.12] shadow-2xl backdrop-blur-xl">
            <div className="px-2 py-1.5 text-[10px] font-semibold text-zinc-400 uppercase tracking-wider">
              {t("nav.workspaces")}
            </div>
            <div className="space-y-0.5 max-h-48 overflow-y-auto">
              {workspaces.data?.map((ws) => (
                <button
                  key={ws.id}
                  type="button"
                  onClick={() => {
                    setWorkspaceMenuOpen(false);
                    router.push(`/w/${ws.slug}/instances`);
                  }}
                  className={cn(
                    "w-full flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs font-medium transition-colors text-left",
                    ws.slug === workspaceSlug
                      ? "bg-emerald-500/10 text-emerald-300 font-semibold"
                      : "text-zinc-300 hover:bg-white/[0.05] hover:text-white"
                  )}
                >
                  <div className="flex items-center gap-2 truncate">
                    <span
                      className={cn(
                        "w-1.5 h-1.5 rounded-full",
                        ws.slug === workspaceSlug ? "bg-emerald-400" : "bg-zinc-600"
                      )}
                    />
                    <span className="truncate">{ws.name}</span>
                  </div>
                  {ws.slug === workspaceSlug && <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" />}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* 2. NAVIGATION GROUPS: Zero Number Badges, Clean Typography */}
      <div className="flex-1 overflow-y-auto px-3 py-4 space-y-6">
        {groups.map((group) => (
          <div key={group.group} className="space-y-1">
            <div className="px-2.5 text-[10.5px] font-semibold tracking-wider text-zinc-400 uppercase">
              {group.group}
            </div>
            <nav className="space-y-0.5">
              {group.items.map((item) => {
                const Icon = item.icon;
                const active =
                  pathname === item.href ||
                  (item.href.endsWith("/instances") && pathname.startsWith(`${item.href}/`));
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    onClick={() => setMobileOpen(false)}
                    className={cn(
                      "flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-xs font-medium transition-all group",
                      active
                        ? "bg-white/[0.08] text-white font-semibold shadow-sm"
                        : "text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.04]"
                    )}
                  >
                    <Icon
                      className={cn(
                        "w-4 h-4 transition-colors",
                        active ? "text-emerald-400" : "text-zinc-400 group-hover:text-zinc-300"
                      )}
                      strokeWidth={1.8}
                    />
                    <span className="truncate tracking-tight">{item.label}</span>
                  </Link>
                );
              })}
            </nav>
          </div>
        ))}
      </div>

      {/* 3. SIDEBAR BOTTOM: Integrated User Profile & System Preferences */}
      <div className="p-3 border-t border-white/[0.06] relative" ref={userRef}>
        <button
          type="button"
          onClick={() => setUserMenuOpen(!userMenuOpen)}
          className="w-full flex items-center justify-between p-2 rounded-lg hover:bg-white/[0.05] transition-colors group text-left"
        >
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="w-7 h-7 rounded-md bg-gradient-to-tr from-cyan-600 to-indigo-600 flex items-center justify-center text-[10px] font-bold text-white shadow-sm shrink-0">
              GP
            </div>
            <div className="min-w-0 flex flex-col">
              <span className="text-xs font-semibold text-zinc-200 group-hover:text-white truncate">
                Admin Operator
              </span>
              <span className="text-[10.5px] text-zinc-400 truncate">Pro Node</span>
            </div>
          </div>
          <Settings className="w-3.5 h-3.5 text-zinc-400 group-hover:text-zinc-200 transition-colors shrink-0" />
        </button>

        {/* User Flyout Menu */}
        {userMenuOpen && (
          <div className="absolute bottom-[calc(100%+4px)] left-3 right-3 z-50 p-1.5 rounded-xl bg-[#12161f] border border-white/[0.12] shadow-2xl backdrop-blur-xl">
            <div className="px-2.5 py-1.5 border-b border-white/[0.06] mb-1">
              <p className="text-xs font-semibold text-zinc-200">Operator Console</p>
              <p className="text-[10.5px] text-zinc-400 truncate">admin@gamepanel.internal</p>
            </div>

            <div className="space-y-0.5">
              <button
                type="button"
                onClick={() => {
                  setUserMenuOpen(false);
                  setAccountModalOpen(true);
                }}
                className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-zinc-300 hover:bg-white/[0.05] hover:text-white transition-colors text-left"
              >
                <UserRound className="w-3.5 h-3.5 text-zinc-400" />
                {t("shell.accountSettings")}
              </button>

              <Link
                href="/platform"
                onClick={() => setUserMenuOpen(false)}
                className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-zinc-300 hover:bg-white/[0.05] hover:text-white transition-colors"
              >
                <ShieldCheck className="w-3.5 h-3.5 text-zinc-400" />
                {t("shell.platformConsole")}
              </Link>

              <div className="my-1 border-t border-white/[0.06]" />

              <Link
                href="/login"
                onClick={() => setUserMenuOpen(false)}
                className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-rose-400 hover:bg-rose-500/10 hover:text-rose-300 transition-colors"
              >
                <LogOut className="w-3.5 h-3.5" />
                {t("shell.signOut")}
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  );

  return (
    <div className="min-h-screen bg-[#090c10] text-zinc-100 flex flex-col md:flex-row antialiased selection:bg-emerald-500/20 selection:text-emerald-300">
      {/* Mobile Top Header */}
      <header className="md:hidden h-14 border-b border-white/[0.08] bg-[#0c1017] px-4 flex items-center justify-between sticky top-0 z-30">
        <button
          type="button"
          onClick={() => setMobileOpen(true)}
          className="p-1.5 rounded-md hover:bg-white/[0.08] text-zinc-300"
        >
          <Menu className="w-5 h-5" />
        </button>
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-emerald-400" />
          <span className="text-xs font-semibold text-white">{scopeName}</span>
        </div>
        <div className="w-6 h-6 rounded bg-emerald-600/30 text-emerald-400 text-xs flex items-center justify-center font-bold font-mono">
          GP
        </div>
      </header>

      {/* Desktop Left Sidebar: Fixed Width, Borderless Top Integration */}
      <aside className="hidden md:flex w-60 shrink-0 border-r border-white/[0.07] bg-[#0c1017] flex-col h-screen sticky top-0">
        {sidebarContent}
      </aside>

      {/* Mobile Drawer */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm md:hidden"
          onClick={() => setMobileOpen(false)}
        >
          <aside
            className="w-64 max-w-[80vw] h-full bg-[#0c1017] border-r border-white/[0.08] flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex justify-end p-2">
              <button
                type="button"
                onClick={() => setMobileOpen(false)}
                className="p-1 text-zinc-400 hover:text-white"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            {sidebarContent}
          </aside>
        </div>
      )}

      {/* Main Content Area: Zero Top Header, Clean Heroic Full Canvas */}
      <main className="flex-1 min-w-0 overflow-y-auto h-screen p-6 md:p-10">
        <div className="max-w-[1440px] mx-auto space-y-6">
          {children}
        </div>
      </main>

      {/* Interactive Account Settings Modal (Profile, Security, Preferences/Language) */}
      <AccountModal open={accountModalOpen} onClose={() => setAccountModalOpen(false)} />
    </div>
  );
}
