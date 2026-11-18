"use client";

import Link from "next/link";
import { ShieldAlert } from "lucide-react";
import { Button, Card } from "@/components/ui";
import { useI18n } from "@/lib/i18n";

export function PermissionDenied() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  return (
    <Card className="mx-auto mt-16 max-w-xl p-8 text-center">
      <span className="mx-auto flex size-12 items-center justify-center rounded-full border border-panel-gold/30 bg-panel-gold/10 text-panel-gold">
        <ShieldAlert aria-hidden="true" className="size-6" />
      </span>
      <h1 className="mt-4 text-lg font-semibold text-white">{isZh ? "当前账号无权访问此功能" : "Your account cannot access this feature"}</h1>
      <p className="mt-2 text-sm leading-6 text-slate-400">
        {isZh ? "如需执行服务器维护或系统管理操作，请联系面板管理员调整账号角色。" : "Ask a panel administrator to update your role if you need server maintenance or system management access."}
      </p>
      <Link className="mt-5 inline-flex" href="/dashboard">
        <Button variant="secondary">{isZh ? "返回仪表盘" : "Back to dashboard"}</Button>
      </Link>
    </Card>
  );
}
