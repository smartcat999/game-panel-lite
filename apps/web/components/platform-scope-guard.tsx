"use client";

import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useAuthBootstrap } from "@/lib/auth-session";
import { useConsoleContext } from "@/lib/console-context";

export function PlatformScopeGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const auth = useAuthBootstrap();
  const { scope, selectPlatform } = useConsoleContext();
  const isPlatformAdmin = auth.data?.account?.platformRole === "platform_admin";

  useEffect(() => {
    if (auth.isLoading) return;
    if (!isPlatformAdmin) {
      router.replace("/servers");
      return;
    }
    if (scope.kind !== "platform") selectPlatform();
  }, [auth.isLoading, isPlatformAdmin, router, scope.kind, selectPlatform]);

  if (auth.isLoading || !isPlatformAdmin || scope.kind !== "platform") {
    return <div className="h-40 animate-pulse rounded-xl border bg-white micro-border" />;
  }
  return <>{children}</>;
}
