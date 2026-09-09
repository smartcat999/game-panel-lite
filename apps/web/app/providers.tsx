"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { AuthQueryProvider } from "@/lib/auth-session";
import { sessionExpiredEvent } from "@/lib/session-events";
import type { AuthBootstrap } from "@/lib/types";
import { I18nProvider } from "@/lib/i18n";
import { PerspectiveProvider } from "@/lib/perspective-context";

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(() => new QueryClient());

  useEffect(() => {
    const expired = () => {
      void client.cancelQueries({ queryKey: ["auth-bootstrap"] });
      client.setQueryData<AuthBootstrap>(["auth-bootstrap"], (current) => ({
        initialized: current?.initialized ?? true,
        allowRegistration: current?.allowRegistration ?? false
      }));
      void client.invalidateQueries({ queryKey: ["auth-bootstrap"] });
    };
    window.addEventListener(sessionExpiredEvent, expired);
    return () => window.removeEventListener(sessionExpiredEvent, expired);
  }, [client]);

  return (
    <AuthQueryProvider client={client}>
      <QueryClientProvider client={client}>
        <I18nProvider>
          <PerspectiveProvider>
            {children}
          </PerspectiveProvider>
        </I18nProvider>
      </QueryClientProvider>
    </AuthQueryProvider>
  );
}
