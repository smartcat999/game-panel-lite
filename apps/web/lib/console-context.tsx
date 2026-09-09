"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listMyOrganizations, type Organization } from "./api";
import { accountCacheKey, useAuthBootstrap } from "./auth-session";

export type ConsoleScope =
  | { kind: "platform" }
  | { kind: "organization"; organizationId: string };

export type ConsoleOrganization = Organization & {
  membershipRole: "owner" | "admin" | "member" | "viewer";
};

type ConsoleContextValue = {
  scope: ConsoleScope;
  organizations: ConsoleOrganization[];
  currentOrganization?: ConsoleOrganization;
  isLoading: boolean;
  selectPlatform: () => void;
  selectOrganization: (organizationId: string) => void;
};

const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function ConsoleContextProvider({ children }: { children: ReactNode }) {
  const auth = useAuthBootstrap();
  const account = auth.data?.account;
  const isPlatformAdmin = account?.platformRole === "platform_admin";
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<ConsoleScope>({ kind: "platform" });
  const storageKey = account ? `gamepanel.console-scope.${accountCacheKey(account)}` : "";

  const organizationsQuery = useQuery({
    queryKey: ["console-organizations", account?.id, account?.platformRole],
    queryFn: async (): Promise<ConsoleOrganization[]> => {
      return listMyOrganizations();
    },
    enabled: Boolean(account),
    retry: false,
    staleTime: 30_000
  });

  const organizations = useMemo(() => organizationsQuery.data ?? [], [organizationsQuery.data]);
  const resolvedScope: ConsoleScope = !isPlatformAdmin && scope.kind === "platform" && organizations[0]
    ? { kind: "organization", organizationId: organizations[0].id }
    : scope;
  const currentOrganization = resolvedScope.kind === "organization"
    ? organizations.find((organization) => organization.id === resolvedScope.organizationId)
    : undefined;

  useEffect(() => {
    if (!account || organizationsQuery.isLoading) return;
    const saved = storageKey ? window.localStorage.getItem(storageKey) : null;
    const savedOrganization = saved?.startsWith("organization:") ? saved.slice("organization:".length) : "";
    if (savedOrganization && organizations.some((organization) => organization.id === savedOrganization)) {
      setScope({ kind: "organization", organizationId: savedOrganization });
      return;
    }
    if (isPlatformAdmin) {
      setScope({ kind: "platform" });
      return;
    }
    const first = organizations[0];
    if (first) setScope({ kind: "organization", organizationId: first.id });
  }, [account, isPlatformAdmin, organizations, organizationsQuery.isLoading, storageKey]);

  const changeScope = useCallback((next: ConsoleScope) => {
    void queryClient.cancelQueries({ queryKey: ["game-servers"] });
    setScope(next);
    if (storageKey) {
      window.localStorage.setItem(storageKey, next.kind === "platform" ? "platform" : `organization:${next.organizationId}`);
    }
  }, [queryClient, storageKey]);

  return (
    <ConsoleContext.Provider value={{
      scope: resolvedScope,
      organizations,
      currentOrganization,
      isLoading: organizationsQuery.isLoading,
      selectPlatform: () => changeScope({ kind: "platform" }),
      selectOrganization: (organizationId) => {
        if (organizations.some((organization) => organization.id === organizationId)) {
          changeScope({ kind: "organization", organizationId });
        }
      }
    }}>
      {children}
    </ConsoleContext.Provider>
  );
}

export function useConsoleContext(): ConsoleContextValue {
  const context = useContext(ConsoleContext);
  if (!context) throw new Error("ConsoleContextProvider is required");
  return context;
}
