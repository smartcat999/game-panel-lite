"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listMyOrganizations, type Organization } from "./api";
import { accountCacheKey, useAuthBootstrap } from "./auth-session";
import { consoleSurfaceForPathname } from "./console-routing";

export type TenantOrganization = Organization & {
  membershipRole: "owner" | "admin" | "member" | "viewer";
};

type TenantSelection = {
  accountKey: string;
  organizationId?: string;
};

type TenantContextValue = {
  organizations: TenantOrganization[];
  currentOrganization?: TenantOrganization;
  isLoading: boolean;
  selectOrganization: (organizationId: string) => void;
};

const TenantContext = createContext<TenantContextValue | null>(null);

export function TenantContextProvider({ children }: { children: ReactNode }) {
  const account = useAuthBootstrap().data?.account;
  const isTenantConsole = consoleSurfaceForPathname(usePathname()) === "tenant";
  const queryClient = useQueryClient();
  const currentAccountKey = account ? accountCacheKey(account) : "";
  const storageKey = currentAccountKey ? `gamepanel.tenant-organization.${currentAccountKey}` : "";
  const [selection, setSelection] = useState<TenantSelection>({ accountKey: "" });

  const organizationsQuery = useQuery({
    queryKey: ["tenant-organizations", account?.id, account?.platformRole],
    queryFn: async (): Promise<TenantOrganization[]> => listMyOrganizations(),
    enabled: Boolean(account) && isTenantConsole,
    retry: false,
    staleTime: 30_000
  });

  const organizations = useMemo(() => organizationsQuery.data ?? [], [organizationsQuery.data]);
  const selectionReady = isTenantConsole && Boolean(currentAccountKey) && selection.accountKey === currentAccountKey;
  const currentOrganization = selectionReady
    ? organizations.find((organization) => organization.id === selection.organizationId)
    : undefined;

  useEffect(() => {
    if (!isTenantConsole || !account || organizationsQuery.isLoading) return;
    const savedOrganizationId = storageKey ? window.localStorage.getItem(storageKey) : null;
    const organizationId = organizations.some((organization) => organization.id === savedOrganizationId)
      ? savedOrganizationId ?? undefined
      : organizations[0]?.id;
    setSelection({ accountKey: currentAccountKey, organizationId });
  }, [account, currentAccountKey, isTenantConsole, organizations, organizationsQuery.isLoading, storageKey]);

  const selectOrganization = useCallback((organizationId: string) => {
    if (!organizations.some((organization) => organization.id === organizationId)) return;
    void queryClient.cancelQueries({ queryKey: ["game-servers", "tenant"] });
    setSelection({ accountKey: currentAccountKey, organizationId });
    if (storageKey) window.localStorage.setItem(storageKey, organizationId);
  }, [currentAccountKey, organizations, queryClient, storageKey]);

  return (
    <TenantContext.Provider value={{
      organizations,
      currentOrganization,
      isLoading: isTenantConsole && (organizationsQuery.isLoading || !selectionReady),
      selectOrganization
    }}>
      {children}
    </TenantContext.Provider>
  );
}

export function useTenantContext(): TenantContextValue {
  const context = useContext(TenantContext);
  if (!context) throw new Error("TenantContextProvider is required");
  return context;
}
