"use client";

import { createContext, useContext, type ReactNode } from "react";
import { QueryClient, useQuery } from "@tanstack/react-query";
import { getAuthBootstrap } from "./api";
import type { UserAccount } from "./types";

const AuthQueryClient = createContext<QueryClient | null>(null);

// Identity queries always use the outer client, including inside account caches.
export function AuthQueryProvider({ client, children }: { client: QueryClient; children: ReactNode }) {
  return <AuthQueryClient.Provider value={client}>{children}</AuthQueryClient.Provider>;
}

export function useAuthBootstrap() {
  const client = useContext(AuthQueryClient);
  if (!client) throw new Error("AuthQueryProvider is required");
  return useQuery({
    queryKey: ["auth-bootstrap"],
    queryFn: ({ signal }) => getAuthBootstrap(signal),
    retry: false,
    staleTime: 30_000,
    refetchInterval: 30_000
  }, client);
}

export function accountCacheKey(account: UserAccount): string {
  return JSON.stringify([account.id, account.role, account.permissions ? [...account.permissions].sort() : null]);
}
