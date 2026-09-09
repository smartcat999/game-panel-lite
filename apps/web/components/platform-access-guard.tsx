"use client";

import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useAuthBootstrap } from "@/lib/auth-session";

export function PlatformAccessGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const auth = useAuthBootstrap();
  const isPlatformAdmin = auth.data?.account?.platformRole === "platform_admin";

  useEffect(() => {
    if (auth.isLoading) return;
    if (!isPlatformAdmin) {
      router.replace("/servers");
    }
  }, [auth.isLoading, isPlatformAdmin, router]);

  if (auth.isLoading || !isPlatformAdmin) {
    return <div className="h-40 animate-pulse rounded-xl border bg-white micro-border" />;
  }
  return <>{children}</>;
}
