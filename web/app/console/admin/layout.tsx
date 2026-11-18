"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { useConsole } from "../ConsoleProvider";
import { Card, EmptyState, focusRing, LinkButton, Skeleton } from "../../components/console/ui";

const SECTIONS: Array<{
  href: string;
  label: string;
  match?: (pathname: string) => boolean;
}> = [
  { href: "/console/admin/nodes", label: "计算节点" },
  { href: "/console/admin/registry", label: "镜像站" },
  { href: "/console/admin/runtimes", label: "应用市场" },
  { href: "/console/admin/users", label: "用户权限" },
];

export default function AdminLayout({ children }: { children: ReactNode }) {
  const { isAdmin, loading } = useConsole();
  const pathname = usePathname() || "";

  if (!isAdmin && loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-10 w-80 rounded-full" />
        <Skeleton className="h-64 w-full rounded-[20px]" />
      </div>
    );
  }

  if (!isAdmin) {
    return (
      <Card>
        <EmptyState
          image="/landing/features/console.jpg"
          title="需要平台管理员权限"
          body="这里是平台管理区域，只有平台管理员可以访问。如需权限，请联系管理员为你的账号绑定平台管理员角色。"
          action={
            <LinkButton href="/console" variant="primary">
              返回控制台
            </LinkButton>
          }
        />
      </Card>
    );
  }

  return (
    <div>
      <nav aria-label="平台管理" className="mb-6 overflow-x-auto lg:hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        <div className="inline-flex flex-nowrap gap-1 rounded-full bg-[#EEF0F7] p-1">
          {SECTIONS.map((s) => {
            const active = s.match ? s.match(pathname) : pathname === s.href || pathname.startsWith(`${s.href}/`);
            return (
              <Link
                key={s.href}
                href={s.href}
                aria-current={active ? "page" : undefined}
                className={`inline-flex h-8 flex-none items-center whitespace-nowrap rounded-full px-4 text-[13px] font-medium transition-colors ${
                  active
                    ? "bg-white text-[#14161F] shadow-[0_2px_8px_-2px_rgba(20,22,31,0.15)]"
                    : "text-[#5B6075] hover:text-[#14161F]"
                } ${focusRing}`}
              >
                {s.label}
              </Link>
            );
          })}
        </div>
      </nav>
      {children}
    </div>
  );
}
