"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  Activity,
  Archive,
  Boxes,
  Building2,
  ChevronDown,
  CircleDollarSign,
  Database,
  Gauge,
  HardDrive,
  LayoutDashboard,
  ListChecks,
  Menu,
  Network,
  Search,
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
import { type ReactNode, useState } from "react";

import { usePreferences } from "@/components/providers";
import { Button } from "@/components/ui/button";
import { controlPlane, type Workspace } from "@/lib/control-plane";
import type { MessageKey } from "@/lib/messages";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";

type Area = "workspace" | "platform" | "region";

type NavItem = {
  label: MessageKey;
  icon: LucideIcon;
};

const navigation: Record<Area, NavItem[]> = {
  workspace: [
    { label: "workspace.nav.overview", icon: LayoutDashboard },
    { label: "workspace.nav.instances", icon: Server },
    { label: "workspace.nav.backups", icon: Archive },
    { label: "workspace.nav.activity", icon: Activity },
    { label: "workspace.nav.billing", icon: CircleDollarSign },
    { label: "workspace.nav.members", icon: Users },
    { label: "workspace.nav.settings", icon: Settings },
  ],
  platform: [
    { label: "platform.nav.overview", icon: LayoutDashboard },
    { label: "platform.nav.workspaces", icon: Building2 },
    { label: "platform.nav.users", icon: Users },
    { label: "platform.nav.plans", icon: ListChecks },
    { label: "platform.nav.orders", icon: CircleDollarSign },
    { label: "platform.nav.instances", icon: Boxes },
    { label: "platform.nav.regions", icon: Network },
    { label: "platform.nav.audit", icon: ShieldCheck },
  ],
  region: [
    { label: "region.nav.overview", icon: LayoutDashboard },
    { label: "region.nav.nodes", icon: Server },
    { label: "region.nav.deployments", icon: Boxes },
    { label: "region.nav.tasks", icon: ListChecks },
    { label: "region.nav.capacity", icon: Gauge },
    { label: "region.nav.storage", icon: HardDrive },
    { label: "region.nav.monitoring", icon: Activity },
    { label: "region.nav.settings", icon: Settings },
  ],
};

const areaCopy: Record<Area, { title: MessageKey; emptyTitle: MessageKey; emptyDescription: MessageKey }> = {
  workspace: { title: "workspace.title", emptyTitle: "workspace.emptyTitle", emptyDescription: "workspace.emptyDescription" },
  platform: { title: "platform.title", emptyTitle: "platform.emptyTitle", emptyDescription: "platform.emptyDescription" },
  region: { title: "region.title", emptyTitle: "region.emptyTitle", emptyDescription: "region.emptyDescription" },
};

export function AppShell({ area, scope, workspaceSlug = "northstar", children }: { area: Area; scope?: string; workspaceSlug?: string; children?: ReactNode }) {
  const { locale, setLocale, theme, setTheme, t } = usePreferences();
  const [navigationOpen, setNavigationOpen] = useState(false);
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const sessionQuery = useQuery({ queryKey: queryKeys.session, queryFn: controlPlane.session });
  const workspacesQuery = useQuery({ queryKey: ["workspaces"], queryFn: controlPlane.workspaces, enabled: area === "workspace" });
  const currentWorkspace = workspacesQuery.data?.find((item) => item.slug === workspaceSlug);
  const selectWorkspace = useMutation({
    mutationFn: (workspace: Workspace) => controlPlane.selectWorkspace(workspace.id).then(() => workspace),
    onSuccess: (workspace) => {
      if (currentWorkspace) {
        queryClient.removeQueries({ queryKey: queryKeys.workspace(currentWorkspace.id) });
      }
      router.push(`/w/${workspace.slug}`);
    },
  });
  const copy = areaCopy[area];
  const workspaceName = currentWorkspace?.name ?? (workspaceSlug === "ember" ? "Ember Realms" : t("workspace.scope"));
  const scopeName = area === "workspace" ? workspaceName : area === "platform" ? t("platform.scope") : scope ?? "region-local";

  const navHref = (item: NavItem, index: number) => {
    if (area === "workspace" && item.label === "workspace.nav.instances") {
      return `/w/${workspaceSlug}/instances`;
    }
    if (area === "workspace" && item.label === "workspace.nav.billing") {
      return `/w/${workspaceSlug}/billing`;
    }
    if (area === "workspace" && item.label === "workspace.nav.members") {
      return `/w/${workspaceSlug}/members`;
    }
    if (area === "workspace" && index === 0) {
      return `/w/${workspaceSlug}`;
    }
    if (area === "platform" && index === 0) {
      return "/platform";
    }
    if (area === "platform") {
      const platformRoutes: Partial<Record<MessageKey, string>> = {
        "platform.nav.workspaces": "/platform/workspaces",
        "platform.nav.plans": "/platform/plans",
        "platform.nav.orders": "/platform/orders",
        "platform.nav.instances": "/platform/instances",
        "platform.nav.regions": "/platform/regions",
      };
      if (platformRoutes[item.label]) return platformRoutes[item.label]!;
    }
    if (area === "region" && index === 0) {
      return `/platform/regions/${scope ?? "region-local"}`;
    }
	if (area === "region") {
		const regionRoutes: Partial<Record<MessageKey, string>> = {
			"region.nav.nodes": "nodes",
			"region.nav.deployments": "deployments",
			"region.nav.tasks": "tasks",
			"region.nav.capacity": "capacity",
			"region.nav.storage": "storage",
			"region.nav.monitoring": "monitoring",
		};
		if (regionRoutes[item.label]) return `/platform/regions/${scope ?? "region-local"}/${regionRoutes[item.label]}`;
	}
    return `#${item.label.replaceAll(".", "-")}`;
  };

  const sidebar = (
    <>
      <div className="brand-row">
        <div className="brand-mark" aria-hidden="true">G</div>
        <span>{t("product.name")}</span>
      </div>
      <div className="area-context">
        <span>{t(copy.title)}</span>
        <strong>{scopeName}</strong>
      </div>
      <nav className="primary-nav" aria-label={t(copy.title)}>
        {navigation[area].map((item, index) => {
          const Icon = item.icon;
          const href = navHref(item, index);
          const active = pathname === href;
          return (
            <Link
              className={cn("nav-item", active && "nav-item-active")}
              href={href}
              key={item.label}
              aria-current={active ? "page" : undefined}
              onClick={() => setNavigationOpen(false)}
            >
              <Icon aria-hidden="true" size={17} strokeWidth={1.8} />
              {t(item.label)}
            </Link>
          );
        })}
      </nav>
    </>
  );

  return (
    <div className="app-shell">
      <aside className="desktop-sidebar">{sidebar}</aside>
      <AnimatePresence>
        {navigationOpen ? (
          <>
            <motion.button
              aria-hidden="true"
              className="mobile-backdrop"
              tabIndex={-1}
              type="button"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={() => setNavigationOpen(false)}
            />
            <motion.aside
              aria-label={t(copy.title)}
              className="mobile-sidebar"
              initial={{ x: "-100%" }}
              animate={{ x: 0 }}
              exit={{ x: "-100%" }}
              transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
            >
              <Button className="mobile-close" variant="quiet" size="icon" onClick={() => setNavigationOpen(false)} aria-label={t("common.closeNavigation")}>
                <X size={19} aria-hidden="true" />
              </Button>
              {sidebar}
            </motion.aside>
          </>
        ) : null}
      </AnimatePresence>

      <div className="shell-main">
        <header className="topbar">
          <div className="topbar-start">
            <Button className="mobile-menu" variant="quiet" size="icon" onClick={() => setNavigationOpen(true)} aria-label={t("common.openNavigation")}>
              <Menu size={19} aria-hidden="true" />
            </Button>
            {area === "workspace" ? (
              <details className="scope-menu">
                <summary className="scope-switcher">
                  <span><small>{t("workspace.scopeLabel")}</small>{scopeName}</span>
                  <ChevronDown size={15} aria-hidden="true" />
                </summary>
                <div className="scope-menu-content">
                  {workspacesQuery.data?.map((workspace) => (
                    <button disabled={selectWorkspace.isPending} key={workspace.id} onClick={() => selectWorkspace.mutate(workspace)} type="button">
                      {workspace.name}
                    </button>
                  ))}
                </div>
              </details>
            ) : area === "region" ? (
              <button className="scope-switcher" type="button">
                <span><small>{t("region.scopeLabel")}</small>{scopeName}</span>
                <ChevronDown size={15} aria-hidden="true" />
              </button>
            ) : (
              <strong className="platform-label">{t("platform.scope")}</strong>
            )}
          </div>
          <div className="topbar-actions">
            <Button variant="quiet" size="icon" aria-label={t("common.search")}>
              <Search size={18} aria-hidden="true" />
            </Button>
            <label className="compact-control">
              <span>{t("common.locale")}</span>
              <select value={locale} onChange={(event) => setLocale(event.target.value as "en" | "zh-CN")}>
                <option value="en">{t("locale.english")}</option>
                <option value="zh-CN">{t("locale.chinese")}</option>
              </select>
            </label>
            <label className="compact-control theme-control">
              <span>{t("common.theme")}</span>
              <select value={theme} onChange={(event) => setTheme(event.target.value as "light" | "dark" | "system")}>
                <option value="system">{t("theme.system")}</option>
                <option value="light">{t("theme.light")}</option>
                <option value="dark">{t("theme.dark")}</option>
              </select>
            </label>
            <details className="user-menu">
              <summary aria-label={t("common.account")}><UserRound size={18} aria-hidden="true" /></summary>
              <div className="user-menu-content">
                <Link href="/account">{t("common.account")}</Link>
                {sessionQuery.data?.platformOperator ? <Link href="/platform">{t("platform.title")}</Link> : null}
              </div>
            </details>
          </div>
        </header>

        <main className="content-canvas">
          <div className="page-heading">
            <div>
              <p>{t(area === "region" ? "region.phase" : "empty.phase")}</p>
              <h1>{t(copy.title)}</h1>
            </div>
            <span className="scope-id">{scopeName}</span>
          </div>
          {children ?? (
            <section className="empty-state" aria-labelledby="empty-state-title">
              <div className="empty-glyph" aria-hidden="true"><Database size={24} strokeWidth={1.6} /></div>
              <div>
                <h2 id="empty-state-title">{t(copy.emptyTitle)}</h2>
                <p>{t(copy.emptyDescription)}</p>
              </div>
            </section>
          )}
        </main>
      </div>
    </div>
  );
}
