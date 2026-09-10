"use client";

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
  Settings,
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
import { cn } from "@/lib/utils";

type Area = "workspace" | "platform" | "region";
type NavItem = { label: string; href: string; icon: LucideIcon };

function workspaceNavigation(slug: string): Array<{ group: string; items: NavItem[] }> {
  return [
    { group: "计算", items: [{ label: "实例", href: `/w/${slug}/instances`, icon: Server }] },
    { group: "数据", items: [{ label: "备份", href: `/w/${slug}/backups`, icon: Archive }] },
    { group: "运维", items: [{ label: "操作", href: `/w/${slug}/operations`, icon: ListChecks }] },
    { group: "工作区", items: [
      { label: "账单", href: `/w/${slug}/billing`, icon: CircleDollarSign },
      { label: "成员", href: `/w/${slug}/members`, icon: Users },
      { label: "设置", href: `/w/${slug}/settings`, icon: Settings },
    ] },
  ];
}

const platformNavigation: Array<{ group: string; items: NavItem[] }> = [
  { group: "平台", items: [
    { label: "区域", href: "/platform/regions", icon: Network },
    { label: "实例", href: "/platform/instances", icon: Boxes },
    { label: "工作区", href: "/platform/workspaces", icon: Building2 },
    { label: "用户", href: "/platform/users", icon: Users },
  ] },
  { group: "商业", items: [
    { label: "额度发放", href: "/platform/credit-grants", icon: CircleDollarSign },
    { label: "价格表", href: "/platform/price-books", icon: Database },
  ] },
  { group: "安全", items: [{ label: "审计", href: "/platform/audit", icon: ShieldCheck }] },
];

function regionNavigation(regionId: string): Array<{ group: string; items: NavItem[] }> {
  return [
    { group: "区域", items: [
      { label: "节点", href: `/platform/regions/${regionId}/nodes`, icon: Server },
      { label: "部署", href: `/platform/regions/${regionId}/deployments`, icon: Boxes },
      { label: "任务", href: `/platform/regions/${regionId}/tasks`, icon: ListChecks },
      { label: "容量", href: `/platform/regions/${regionId}/capacity`, icon: Gauge },
      { label: "存储", href: `/platform/regions/${regionId}/storage`, icon: HardDrive },
      { label: "监控", href: `/platform/regions/${regionId}/monitoring`, icon: Bell },
    ] },
  ];
}

export function AppShell({ area, scope, workspaceSlug = "ember", children }: { area: Area; scope?: string; workspaceSlug?: string; children: ReactNode }) {
  const [mobileOpen, setMobileOpen] = useState(false);
  const pathname = usePathname();
  const scopeName = area === "workspace" ? "Ember Realms" : area === "platform" ? "平台管理" : scope ?? "cn-east-1";
  const groups = area === "workspace" ? workspaceNavigation(workspaceSlug) : area === "platform" ? platformNavigation : regionNavigation(scope ?? "cn-east-1");

  const sidebar = (
    <>
      <div className="sidebar-title">
        <span>{area === "workspace" ? "WORKSPACE" : area === "platform" ? "PLATFORM" : "REGION"}</span>
        <button aria-label="收起导航" type="button"><Menu size={15} /></button>
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
          <Image alt="GamePanel" height={28} src="/icon.svg" width={28} />
          <span className="topbar-divider" />
          <button className="workspace-switcher" type="button"><span className="online-dot" />{scopeName}<ChevronDown size={14} /></button>
        </div>
        <div className="topbar-tools">
          <button aria-label="文档" type="button"><BookOpen size={17} /></button>
          <button aria-label="通知" className="notification" type="button"><Bell size={18} /><span /></button>
          <span className="topbar-divider" />
          <details className="account-menu">
            <summary><span className="avatar">PW</span><span>Peng Wu</span><ChevronDown size={14} /></summary>
            <div className="menu-popover">
              <Link href="/account"><UserRound size={15} />账户设置</Link>
              <Link href="/platform"><ShieldCheck size={15} />平台管理</Link>
              <Link href="/login">退出登录</Link>
            </div>
          </details>
        </div>
      </header>

      <button className="mobile-nav-trigger" onClick={() => setMobileOpen(true)} type="button"><Menu size={18} />导航</button>
      <aside className="prototype-sidebar">{sidebar}</aside>
      {mobileOpen ? <div className="mobile-nav-overlay" onClick={() => setMobileOpen(false)}><aside onClick={(event) => event.stopPropagation()}><Button aria-label="关闭导航" className="mobile-nav-close" onClick={() => setMobileOpen(false)} size="icon" variant="quiet"><X size={18} /></Button>{sidebar}</aside></div> : null}
      <main className="prototype-main">{children}</main>
    </div>
  );
}
