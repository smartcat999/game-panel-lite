"use client";

import { Suspense, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useConsole } from "./ConsoleProvider";
import { tenantRoute } from "./lib/routes";
import { Card, EmptyState } from "../components/console/ui";

const ALLOWED_TARGETS = new Set(["new", "members"]);

function ConsoleResolver() {
  const { ready, loading, tenant } = useConsole();
  const router = useRouter();
  const searchParams = useSearchParams();

  useEffect(() => {
    if (!ready || loading || !tenant) return;
    const to = searchParams.get("to")?.trim() || "";
    const sp = new URLSearchParams(searchParams.toString());
    sp.delete("to");
    const sub = ALLOWED_TARGETS.has(to) ? `/${to}` : "";
    const qs = sp.toString();
    router.replace(tenantRoute(tenant, qs ? `${sub}?${qs}` : sub));
  }, [ready, loading, tenant, searchParams, router]);

  if (ready && !loading && !tenant) {
    return (
      <Card>
        <EmptyState
          title="暂无可用工作区"
          body="当前账号尚未加入任何工作区，请联系平台管理员分配工作区权限。"
        />
      </Card>
    );
  }

  return <div className="py-20 text-center text-sm text-[#8A90A6]">正在进入工作区…</div>;
}

export default function ConsoleEntryPage() {
  return (
    <Suspense fallback={<div className="py-20 text-center text-sm text-[#8A90A6]">正在进入工作区…</div>}>
      <ConsoleResolver />
    </Suspense>
  );
}
