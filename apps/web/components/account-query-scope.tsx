"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { ToastProvider } from "./toast-context";

// A replaced account gets a different client and a fresh component tree. Late
// callbacks retain the old client and cannot populate the next account's cache.
export function AccountQueryScope({ children }: { children: ReactNode }) {
  const [client] = useState(() => new QueryClient());
  useEffect(() => () => client.clear(), [client]);
  return <QueryClientProvider client={client}><ToastProvider>{children}</ToastProvider></QueryClientProvider>;
}
