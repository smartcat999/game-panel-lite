"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

import { LocaleProvider } from "@/lib/i18n";
import { PrototypeProvider } from "@/lib/prototype-store";

export function Providers({ children, initialLocale }: { children: React.ReactNode; initialLocale: string; initialTheme: string; initialTimeZone: string }) {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false } } }));
  return <QueryClientProvider client={client}><LocaleProvider initialLocale={initialLocale}><PrototypeProvider>{children}</PrototypeProvider></LocaleProvider></QueryClientProvider>;
}
