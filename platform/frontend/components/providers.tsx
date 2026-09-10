"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

import { PrototypeProvider } from "@/lib/prototype-store";

export function Providers({ children }: { children: React.ReactNode; initialLocale: string; initialTheme: string; initialTimeZone: string }) {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false } } }));
  return <QueryClientProvider client={client}><PrototypeProvider>{children}</PrototypeProvider></QueryClientProvider>;
}
