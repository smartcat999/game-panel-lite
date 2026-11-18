"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Cpu,
  Globe,
  LayoutGrid,
  LogOut,
  Plus,
  Search,
  Server,
  Settings,
  Shield,
  Users,
} from "lucide-react";
import LogoMark from "../landing/LogoMark";
import { useConsole } from "../../console/ConsoleProvider";
import { phaseLabel, phaseOf } from "../../console/lib/format";
import type { Tenant } from "../../console/lib/types";
import { Input, focusRing } from "./ui";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");
const GITHUB_REPO_URL = "https://github.com/smartcat999/game-panel-lite";

function GitHubIcon({ className = "h-4 w-4" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
      <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
    </svg>
  );
}

function useClickOutside(ref: React.RefObject<HTMLElement | null>, onOutside: () => void, active: boolean) {
  useEffect(() => {
    if (!active) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onOutside();
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onOutside();
    document.addEventListener("mousedown", handler);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", handler);
      document.removeEventListener("keydown", onKey);
    };
  }, [ref, onOutside, active]);
}

const AVATAR_COLORS = ["#2B3AE8", "#1F9D55", "#C2410C", "#7C3AED", "#0E7490", "#BE185D"];
function avatarColor(name: string) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return AVATAR_COLORS[h % AVATAR_COLORS.length];
}

export function Avatar({ name, size = 28, round }: { name: string; size?: number; round?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        "inline-flex shrink-0 items-center justify-center font-semibold text-white",
        round ? "rounded-full" : "rounded-lg",
      )}
      style={{ width: size, height: size, background: avatarColor(name || "?"), fontSize: size * 0.42 }}
    >
      {(name || "?").slice(0, 1).toUpperCase()}
    </span>
  );
}

/** Strips redundant trailing "Personal" when a personal workspace already shows the "个人" badge. */
function cleanWorkspaceName(t?: Tenant, fallback?: string): string {
  const raw = (t?.spec.displayName || t?.metadata.name || fallback || "选择工作区").trim();
  if (t?.spec.type !== "Organization" && /\s+personal$/i.test(raw)) {
    return raw.replace(/\s+personal$/i, "").trim() || raw;
  }
  return raw;
}

const ADMIN_SECTIONS: Array<{
  href: string;
  label: string;
  match?: (pathname: string) => boolean;
}> = [
  { href: "/console/admin/nodes", label: "计算节点" },
  { href: "/console/admin/registry", label: "镜像站" },
  { href: "/console/admin/runtimes", label: "应用市场" },
  { href: "/console/admin/users", label: "用户权限" },
];

// ------------------------------------------------------------ command menu
interface Command {
  id: string;
  group: string;
  label: string;
  hint?: string;
  icon: ReactNode;
  href?: string;
  run?: () => void;
}

function CommandMenu({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const { instances, isAdmin, canManageTenant, tenantObj, tenant } = useConsole();
  const [q, setQ] = useState("");
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setQ("");
      setCursor(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  const commands = useMemo<Command[]>(() => {
    const ws = cleanWorkspaceName(tenantObj, tenant);
    const list: Command[] = [
      { id: "servers", group: `工作区 ${ws}`, label: "我的服务器", icon: <Server className="h-4 w-4" />, href: "/console" },
      { id: "members", group: `工作区 ${ws}`, label: "成员", icon: <Users className="h-4 w-4" />, href: "/console/members" },
      ...(canManageTenant
        ? [{ id: "new", group: "快捷操作", label: "新建服务器", icon: <Plus className="h-4 w-4" />, href: "/console/new" }]
        : []),
      { id: "settings", group: "快捷操作", label: "个人设置", icon: <Settings className="h-4 w-4" />, href: "/console/settings" },
      ...instances.map((i) => ({
        id: `srv-${i.metadata.name}`,
        group: "跳转到服务器",
        label: i.metadata.name,
        hint: phaseLabel(phaseOf(i)),
        icon: <Box className="h-4 w-4" />,
        href: `/console/servers/${encodeURIComponent(i.metadata.name)}`,
      })),
    ];
    if (isAdmin) {
      list.push(
        { id: "a-nodes", group: "平台管理", label: "计算节点", icon: <Cpu className="h-4 w-4" />, href: "/console/admin/nodes" },
        { id: "a-registry", group: "平台管理", label: "镜像站", icon: <Globe className="h-4 w-4" />, href: "/console/admin/registry" },
        { id: "a-rt", group: "平台管理", label: "应用市场", icon: <LayoutGrid className="h-4 w-4" />, href: "/console/admin/runtimes" },
        { id: "a-users", group: "平台管理", label: "用户权限", icon: <Users className="h-4 w-4" />, href: "/console/admin/users" },
      );
    }
    return list;
  }, [instances, isAdmin, canManageTenant, tenantObj, tenant]);

  const filtered = commands.filter((c) => {
    const s = q.trim().toLowerCase();
    return !s || c.label.toLowerCase().includes(s) || c.group.toLowerCase().includes(s);
  });

  const go = (c: Command) => {
    onClose();
    if (c.href) router.push(c.href);
    c.run?.();
  };

  if (!open) return null;
  let lastGroup = "";
  return (
    <div className="fixed inset-0 z-[95] flex items-start justify-center px-4 pt-[12vh]">
      <div className="absolute inset-0 bg-[#14161F]/30 backdrop-blur-[2px]" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="搜索"
        className="relative w-full max-w-[560px] overflow-hidden rounded-[24px] bg-white shadow-[0_40px_80px_-30px_rgba(20,22,31,0.5)]"
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setCursor((c) => Math.min(filtered.length - 1, c + 1));
          }
          if (e.key === "ArrowUp") {
            e.preventDefault();
            setCursor((c) => Math.max(0, c - 1));
          }
          if (e.key === "Enter" && filtered[cursor]) go(filtered[cursor]);
        }}
      >
        <div className="flex items-center gap-3 border-b border-[#EEF0F7] px-5 focus-within:bg-[#FAFBFD]">
          <Search className="h-5 w-5 text-[#8A90A6]" />
          <Input
            ref={inputRef}
            variant="bare"
            type="search"
            autoCorrect="off"
            autoCapitalize="none"
            spellCheck={false}
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setCursor(0);
            }}
            placeholder="搜索页面、服务器或操作"
            aria-label="搜索"
            className="h-14 w-full text-[15px] [&::-webkit-search-cancel-button]:hidden"
          />
          <kbd className="rounded-md border border-[#E3E6F2] px-1.5 py-0.5 text-[11px] text-[#8A90A6]">Esc</kbd>
        </div>
        <div className="max-h-[60vh] overflow-y-auto p-2">
          {filtered.map((c, idx) => {
            const header = c.group !== lastGroup ? c.group : null;
            lastGroup = c.group;
            return (
              <div key={c.id}>
                {header && <div className="px-3 pb-1.5 pt-3 text-xs font-medium text-[#8A90A6]">{header}</div>}
                <button
                  type="button"
                  onMouseEnter={() => setCursor(idx)}
                  onClick={() => go(c)}
                  className={cx(
                    "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm",
                    idx === cursor ? "bg-[#EEF1FF] text-[#2B3AE8]" : "text-[#14161F]",
                  )}
                >
                  <span className={idx === cursor ? "text-[#2B3AE8]" : "text-[#5B6075]"}>{c.icon}</span>
                  <span className="flex-1 truncate">{c.label}</span>
                  {c.hint && <span className="text-xs text-[#8A90A6]">{c.hint}</span>}
                </button>
              </div>
            );
          })}
          {!filtered.length && <p className="px-3 py-10 text-center text-sm text-[#8A90A6]">没有找到「{q}」</p>}
        </div>
      </div>
    </div>
  );
}

// --------------------------------------------------------------- user menu
function UserMenu() {
  const { userName, me, isAdmin, logout } = useConsole();
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useClickOutside(ref, close, open);
  const inAdmin = pathname?.startsWith("/console/admin");

  const item = "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm text-[#14161F] hover:bg-[#F5F7FD]";
  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-label="账号菜单"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cx("rounded-full", focusRing)}
      >
        <Avatar name={userName} size={36} round />
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 top-11 z-50 w-[min(16.25rem,calc(100dvw-1.5rem))] overflow-clip rounded-[20px] border border-[#E3E6F2] bg-white p-1.5 whitespace-normal shadow-[0_24px_60px_-24px_rgba(20,22,31,0.35)]"
        >
          <div className="flex items-center gap-3 px-3 py-3">
            <Avatar name={userName} size={36} round />
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-[#14161F]">{me?.spec.displayName || userName}</div>
              <div className="truncate text-xs text-[#8A90A6]">{me?.spec.email || (isAdmin ? "平台管理员" : "")}</div>
            </div>
          </div>
          <div className="my-1 h-px bg-[#EEF0F7]" />
          <Link role="menuitem" href="/console/settings" onClick={close} className={item}>
            <Settings className="h-4 w-4 text-[#5B6075]" />
            个人设置
          </Link>
          {isAdmin && (
            <Link role="menuitem" href={inAdmin ? "/console" : "/console/admin/nodes"} onClick={close} className={item}>
              <Shield className="h-4 w-4 text-[#5B6075]" />
              {inAdmin ? "返回工作区" : "平台管理"}
            </Link>
          )}
          <Link role="menuitem" href="/" onClick={close} className={item}>
            <Globe className="h-4 w-4 text-[#5B6075]" />
            返回官网
          </Link>
          <a
            role="menuitem"
            href={GITHUB_REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            onClick={close}
            className={item}
          >
            <GitHubIcon className="h-4 w-4 text-[#5B6075]" />
            GitHub
          </a>
          <div className="my-1 h-px bg-[#EEF0F7]" />
          <button role="menuitem" type="button" onClick={() => void logout()} className={cx(item, "text-[#B42318]")}>
            <LogOut className="h-4 w-4" />
            退出登录
          </button>
        </div>
      )}
    </div>
  );
}

// ------------------------------------------------------------------ topbar
export default function TopBar() {
  const pathname = usePathname() || "";
  const [cmdOpen, setCmdOpen] = useState(false);
  const inAdmin = pathname.startsWith("/console/admin");

  const workspaceSections = useMemo(
    () => [
      {
        href: "/console",
        label: "服务器",
        match: (p: string) => p === "/console" || p.startsWith("/console/servers/") || p.startsWith("/console/new"),
      },
      {
        href: "/console/members",
        label: "成员",
        match: (p: string) => p.startsWith("/console/members"),
      },
    ],
    [],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCmdOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <>
      <header className="sticky top-0 z-40 border-b border-[#E3E6F2] bg-white/85 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1320px] items-center gap-1.5 px-3 sm:gap-2 sm:px-6">
          {/* Left: Brand + Admin Pill */}
          <Link href="/console" className={cx("flex shrink-0 items-center gap-2.5 rounded-full pr-1", focusRing)}>
            <LogoMark />
            <span className="hidden text-[15px] font-semibold tracking-tight text-[#14161F] sm:inline">
              GamePanel Lite
            </span>
          </Link>

          {inAdmin && (
            <span className="inline-flex h-7 items-center gap-1.5 rounded-full border border-[#C7D0FE] bg-[#EEF1FF] px-3 text-xs font-semibold text-[#2B3AE8]">
              <Shield className="h-3.5 w-3.5 shrink-0" />
              平台管理
            </span>
          )}

          {/* Center: Inline Primary Navigation (Admin sections or Workspace sections) */}
          <span className="mx-1.5 hidden h-4 w-px shrink-0 bg-[#E3E6F2] lg:inline-block" aria-hidden />

          <nav
            aria-label={inAdmin ? "平台管理导航" : "工作区导航"}
            className="hidden shrink-0 items-center gap-1 lg:flex"
          >
            {inAdmin
              ? ADMIN_SECTIONS.map((s) => {
                  const active = s.match ? s.match(pathname) : pathname === s.href || pathname.startsWith(`${s.href}/`);
                  return (
                    <Link
                      key={s.href}
                      href={s.href}
                      aria-current={active ? "page" : undefined}
                      className={cx(
                        "inline-flex h-8 shrink-0 items-center whitespace-nowrap rounded-full px-3 text-[13px] font-medium transition-colors",
                        active
                          ? "bg-[#14161F] text-white shadow-xs"
                          : "text-[#5B6075] hover:bg-[#EEF1FA] hover:text-[#14161F]",
                        focusRing,
                      )}
                    >
                      {s.label}
                    </Link>
                  );
                })
              : workspaceSections.map((s) => {
                  const active = s.match(pathname);
                  return (
                    <Link
                      key={s.href}
                      href={s.href}
                      aria-current={active ? "page" : undefined}
                      className={cx(
                        "inline-flex h-8 shrink-0 items-center whitespace-nowrap rounded-full px-3 text-[13px] font-medium transition-colors",
                        active
                          ? "bg-[#14161F] text-white shadow-xs"
                          : "text-[#5B6075] hover:bg-[#EEF1FA] hover:text-[#14161F]",
                        focusRing,
                      )}
                    >
                      {s.label}
                    </Link>
                  );
                })}
          </nav>

          {/* Right: Search (⌘K) + User Avatar */}
          <div className="ml-auto flex shrink-0 items-center gap-2 sm:gap-2.5">
            <button
              type="button"
              onClick={() => setCmdOpen(true)}
              className={cx(
                "inline-flex h-9 shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-[#E3E6F2] bg-white px-3.5 text-sm font-medium text-[#14161F] transition-colors hover:border-[#B8C0E6] hover:bg-[#F7F8FC]",
                focusRing,
              )}
            >
              <Search className="h-3.5 w-3.5 shrink-0 text-[#5B6075]" aria-hidden="true" />
              <span className="hidden sm:inline">搜索</span>
              <kbd className="hidden rounded bg-[#F1F3FB] px-1.5 py-0.5 font-mono text-[11px] font-medium text-[#8A90A6] md:inline">
                ⌘K
              </kbd>
            </button>

            <UserMenu />
          </div>
        </div>
      </header>
      <CommandMenu open={cmdOpen} onClose={() => setCmdOpen(false)} />
    </>
  );
}
