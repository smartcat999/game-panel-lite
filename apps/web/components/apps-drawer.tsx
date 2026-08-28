"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  Archive,
  Bookmark,
  Box,
  Gamepad2,
  Gauge,
  Globe2,
  HardDrive,
  PackageCheck,
  Plus,
  Server,
  Settings,
  X
} from "lucide-react";
import { useEffect } from "react";
import { Button } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface AppsDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function AppsDrawer({ open, onClose }: AppsDrawerProps) {
  const pathname = usePathname();
  const { locale } = useI18n();
  const isZh = locale === "zh";

  // Close on ESC
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if ((e.metaKey || e.ctrlKey) && e.key === "b") {
        e.preventDefault();
        onClose();
      }
    };
    if (open) {
      window.addEventListener("keydown", handleKeyDown);
    }
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const appGroups = [
    {
      title: isZh ? "核心工作台" : "Core Workflows",
      items: [
        { href: "/dashboard", label: isZh ? "仪表盘" : "Dashboard", desc: isZh ? "集群资源与实例总览" : "Infrastructure & servers overview", icon: Gauge },
        { href: "/servers", label: isZh ? "游戏服务器" : "Game Servers", desc: isZh ? "多实例运行与控制台管理" : "Instances management & web console", icon: HardDrive },
        { href: "/activity", label: isZh ? "监控与活动" : "Monitoring & Activity", desc: isZh ? "实时时序指标与审计日志" : "Live metrics & audit streams", icon: Activity }
      ]
    },
    {
      title: isZh ? "游戏与数字资产" : "Game Assets & Mods",
      items: [
        { href: "/games", label: isZh ? "游戏库" : "Game Library", desc: isZh ? "支持的原生与模组服务端" : "Supported games & engines", icon: Gamepad2 },
        { href: "/mods", label: isZh ? "模组工坊" : "Mod Workshop", desc: isZh ? "Steam 创意工坊与自定义模组" : "Workshop items & custom modpacks", icon: Box },
        { href: "/presets", label: isZh ? "配置预设" : "Configuration Presets", desc: isZh ? "一键快速配置模版" : "One-click configuration templates", icon: Bookmark },
        { href: "/worlds", label: isZh ? "世界地图" : "World Archives", desc: isZh ? "游戏存档与地图文件管理" : "Saves & map files management", icon: Globe2 },
        { href: "/backups", label: isZh ? "备份中心" : "Backup Vault", desc: isZh ? "快照创建与一键还原" : "Snapshots & disaster recovery", icon: Archive }
      ]
    },
    {
      title: isZh ? "系统与集群管理" : "Cluster & System",
      items: [
        { href: "/settings", label: isZh ? "控制台设置" : "Settings & Fleet", desc: isZh ? "团队 RBAC、计算节点与安全" : "Team RBAC, compute nodes & security", icon: Settings },
        { href: "/versions", label: isZh ? "版本与更新" : "Version & Updates", desc: isZh ? "系统固件与升级通道" : "System release & channels", icon: PackageCheck }
      ]
    }
  ];

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/60 backdrop-blur-xs transition-opacity" onClick={onClose} />

      {/* Drawer Body */}
      <div className="relative z-10 flex h-full w-full max-w-md flex-col border-l border-slate-800 bg-slate-950/98 p-6 shadow-2xl backdrop-blur-2xl animate-in slide-in-from-right duration-200">
        <div className="flex items-center justify-between border-b border-slate-800 pb-4">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-lg bg-panel-green/15 text-panel-green">
              <Server className="size-4" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-100">{isZh ? "全局应用与工具导航" : "Command Apps & Tools"}</h3>
              <p className="text-[11px] text-slate-400">{isZh ? "GamePanel Lite 云控制台矩阵" : "GamePanel Lite Cloud Suite"}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-white transition"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Quick Create Server Action */}
        <div className="mt-4">
          <Link href="/servers/new" onClick={onClose} className="block">
            <Button variant="primary" className="w-full h-9 justify-center gap-2 text-xs font-semibold">
              <Plus className="size-4" />
              {isZh ? "新建游戏服务器" : "Create New Server"}
            </Button>
          </Link>
        </div>

        {/* Groups */}
        <div className="mt-4 flex-1 space-y-6 overflow-y-auto pr-1">
          {appGroups.map((group) => (
            <div key={group.title}>
              <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-500 mb-2">
                {group.title}
              </div>
              <div className="space-y-1">
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const isActive = pathname === item.href || pathname.startsWith(item.href + "/");
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={onClose}
                      className={cn(
                        "flex items-start gap-3 rounded-xl p-2.5 text-xs transition",
                        isActive
                          ? "border border-panel-green/40 bg-panel-green/10 text-panel-green"
                          : "border border-transparent hover:border-slate-800 hover:bg-slate-900/70 text-slate-300"
                      )}
                    >
                      <div className={cn("mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg border", isActive ? "border-panel-green/40 bg-panel-green/20 text-panel-green" : "border-slate-800 bg-slate-900 text-slate-400")}>
                        <Icon className="size-3.5" />
                      </div>
                      <div>
                        <div className="font-semibold text-slate-100">{item.label}</div>
                        <div className="text-[11px] text-slate-500 leading-tight mt-0.5">{item.desc}</div>
                      </div>
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </div>

        {/* Footer */}
        <div className="border-t border-slate-800 pt-4 flex items-center justify-between text-[11px] text-slate-500 font-mono">
          <span>GamePanel Lite v0.3.2</span>
          <span>Press ESC or ⌘B to close</span>
        </div>
      </div>
    </div>
  );
}
