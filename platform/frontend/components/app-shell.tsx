"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Archive,
  Bell,
  BookOpen,
  Boxes,
  Building2,
  ChevronDown,
  CircleDollarSign,
  Database,
  Gauge,
  HardDrive,
  ListChecks,
  Menu,
  Network,
  Server,
  ShieldCheck,
  UserRound,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { api, type Workspace } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Area = "workspace" | "platform" | "region";
type NavItem = { label: string; href: string; icon: LucideIcon };
type Translate = ReturnType<typeof useI18n>["t"];

function workspaceNavigation(slug: string, t: Translate): Array<{ group: string; items: NavItem[] }> {
  return [
    { group: t("nav.compute"), items: [{ label: t("nav.instances"), href: `/w/${slug}/instances`, icon: Server }] },
    { group: t("nav.data"), items: [{ label: t("nav.backups"), href: `/w/${slug}/backups`, icon: Archive }] },
    { group: t("nav.workspace"), items: [{ label: t("nav.billing"), href: `/w/${slug}/billing`, icon: CircleDollarSign }] },
  ];
}

function platformNavigation(t: Translate): Array<{ group: string; items: NavItem[] }> { return [
  { group: t("nav.platform"), items: [
    { label: t("nav.regions"), href: "/platform/regions", icon: Network },
    { label: t("nav.instances"), href: "/platform/instances", icon: Boxes },
    { label: t("nav.workspaces"), href: "/platform/workspaces", icon: Building2 },
    { label: t("nav.users"), href: "/platform/users", icon: Users },
  ] },
  { group: t("nav.commerce"), items: [
    { label: t("nav.creditGrants"), href: "/platform/credit-grants", icon: CircleDollarSign },
    { label: t("nav.priceBooks"), href: "/platform/price-books", icon: Database },
  ] },
  { group: t("nav.security"), items: [{ label: t("nav.audit"), href: "/platform/audit", icon: ShieldCheck }] },
]; }

function regionNavigation(regionId: string, t: Translate): Array<{ group: string; items: NavItem[] }> {
  return [
    { group: t("nav.regions"), items: [
      { label: t("nav.nodes"), href: `/platform/regions/${regionId}/nodes`, icon: Server },
      { label: t("nav.deployments"), href: `/platform/regions/${regionId}/deployments`, icon: Boxes },
      { label: t("nav.tasks"), href: `/platform/regions/${regionId}/tasks`, icon: ListChecks },
      { label: t("nav.capacity"), href: `/platform/regions/${regionId}/capacity`, icon: Gauge },
      { label: t("nav.storage"), href: `/platform/regions/${regionId}/storage`, icon: HardDrive },
      { label: t("nav.monitoring"), href: `/platform/regions/${regionId}/monitoring`, icon: Bell },
    ] },
  ];
}

export function AppShell({ area, scope, workspaceSlug = "ember", children }: { area: Area; scope?: string; workspaceSlug?: string; children: ReactNode }) {
  const { t } = useI18n();
  const [mobileOpen, setMobileOpen] = useState(false);
  const pathname = usePathname();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => api<Workspace[]>("/workspaces"), enabled: area === "workspace" });
  const scopeName = area === "workspace" ? workspaces.data?.find((workspace) => workspace.slug === workspaceSlug)?.name ?? workspaceSlug : area === "platform" ? t("shell.platformConsole") : scope ?? "cn-east-1";
  const groups = area === "workspace" ? workspaceNavigation(workspaceSlug, t) : area === "platform" ? platformNavigation(t) : regionNavigation(scope ?? "cn-east-1", t);

  const sidebar = (
    <>
      <div className="sidebar-title">
        <span>{area === "workspace" ? t("shell.workspace") : area === "platform" ? t("shell.platform") : t("shell.region")}</span>
        <button aria-label={t("shell.collapseNavigation")} type="button"><Menu size={15} /></button>
      </div>
      <div className="sidebar-groups">
        {groups.map((group) => (
          <div className="nav-group" key={group.group}>
            <span className="nav-group-label">{group.group}</span>
            {group.items.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href || (item.href.endsWith("/instances") && pathname.startsWith(`${item.href}/`));
              return <Link className={cn("nav-item", active && "nav-item-active")} href={item.href} key={item.href} onClick={() => setMobileOpen(false)}><Icon size={17} strokeWidth={1.8} />{item.label}</Link>;
            })}
          </div>
        ))}
      </div>
    </>
  );

  return (
    <div className="prototype-shell">
      <header className="prototype-topbar">
        <div className="topbar-brand">
          <button aria-label={t("shell.navigation")} className="mobile-nav-trigger" onClick={() => setMobileOpen(true)} type="button"><Menu size={18} /></button>
          <Image alt="GamePanel" height={28} src="/icon.svg" width={28} />
          <span className="topbar-divider" />
          <button className="workspace-switcher" type="button"><span className="online-dot" /><span className="workspace-name">{scopeName}</span><ChevronDown size={14} /></button>
        </div>
        <div className="topbar-tools">
          <button aria-label={t("shell.documentation")} type="button"><BookOpen size={17} /></button>
          <button aria-label={t("shell.notifications")} className="notification" type="button"><Bell size={18} /><span /></button>
          <span className="topbar-divider" />
          <details className="account-menu">
            <summary><span className="avatar">GP</span><span>{t("shell.account")}</span><ChevronDown size={14} /></summary>
            <div className="menu-popover">
              <Link href="/account"><UserRound size={15} />{t("shell.accountSettings")}</Link>
              <Link href="/platform"><ShieldCheck size={15} />{t("shell.platformConsole")}</Link>
              <Link href="/login">{t("shell.signOut")}</Link>
            </div>
          </details>
        </div>
      </header>

      <aside className="prototype-sidebar">{sidebar}</aside>
      {mobileOpen ? <div className="mobile-nav-overlay" onClick={() => setMobileOpen(false)}><aside onClick={(event) => event.stopPropagation()}><Button aria-label={t("shell.closeNavigation")} className="mobile-nav-close" onClick={() => setMobileOpen(false)} size="icon" variant="quiet"><X size={18} /></Button>{sidebar}</aside></div> : null}
      <main className="prototype-main">{children}</main>
    </div>
  );
}
