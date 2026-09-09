"use client";

import { usePerspective, type PerspectiveRole } from "@/lib/perspective-context";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function TopNav() {
  const { perspective, setPerspective } = usePerspective();
  const { locale } = useI18n();
  const isZh = locale === "zh";

  const perspectives: Array<{ id: PerspectiveRole; label: string; desc: string }> = [
    {
      id: "user",
      label: isZh ? "普通成员" : "Member",
      desc: isZh ? "战队协作者：只读权限" : "Collaborator: Read-only access"
    },
    {
      id: "admin",
      label: isZh ? "工作区管理员" : "Workspace Admin",
      desc: isZh ? "工作区管理员：管实例/存档/账单，无底层硬件设施" : "Workspace Admin: Manages servers & billing, no infra"
    },
    {
      id: "super",
      label: isZh ? "平台总管" : "Platform Superadmin",
      desc: isZh ? "平台总管：多租户治理与底层硬件基础设施" : "Superadmin: Multi-tenant & hardware cluster"
    }
  ];

  return (
    <nav className="flex items-center rounded-lg border border-slate-200/80 bg-slate-100/90 p-0.5 text-xs select-none">
      {perspectives.map((p) => {
        const active = perspective === p.id;
        return (
          <button
            key={p.id}
            type="button"
            title={p.desc}
            onClick={() => setPerspective(p.id)}
            className={cn(
              "rounded-md px-2.5 py-1 text-[11px] font-medium transition-all",
              active
                ? "bg-white text-slate-900 font-bold shadow-xs border border-slate-200/60"
                : "text-slate-500 hover:text-slate-900"
            )}
          >
            {p.label}
          </button>
        );
      })}
    </nav>
  );
}
