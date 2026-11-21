"use client";

import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { usePathname, useRouter } from "next/navigation";
import { parseNumericQuantity, resourceOf } from "./lib/format";
import { parseTenantFromPathname, tenantRoute } from "./lib/routes";
import { subscribeResourceWatch } from "./lib/ws";
import {
  clearSession,
  readSessionName,
  readToken,
  readUserName,
  SESSION_STORAGE_KEY,
  TENANT_STORAGE_KEY,
  USER_STORAGE_KEY,
} from "./lib/session";
import type {
  AppItem,
  GlobalRoleBindingItem,
  GlobalRoleItem,
  InstanceItem,
  NodeItem,
  RuntimeClass,
  Tenant,
  UserItem,
} from "./lib/types";

export type ToastKind = "success" | "error" | "info";
interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

export interface QuotaSummary {
  instances: { used: number; reserved?: number; max: number };
  cpu: { used: number; reserved?: number; max: number };
  memory: { used: number; reserved?: number; max: number };
  storage: { used: number; reserved?: number; max: number };
  backups: { used: number; max: number };
  backupStorage: { used: number; max: number };
}

interface ConsoleContextValue {
  ready: boolean;
  token: string | null;
  userName: string;
  me: UserItem | null;
  isAdmin: boolean;
  tenantRole: string;
  canManageTenant: boolean;
  canOperateInstances: boolean;

  tenants: Tenant[];
  tenant: string;
  tenantObj: Tenant | undefined;
  setTenant: (name: string) => void;

  instances: InstanceItem[];
  instancesLoading: boolean;
  live: boolean;
  nodes: NodeItem[];
  runtimeClasses: RuntimeClass[];
  apps: AppItem[];
  users: UserItem[];
  globalRoles: GlobalRoleItem[];
  globalRoleBindings: GlobalRoleBindingItem[];
  quota: QuotaSummary;

  loading: boolean;
  reload: () => Promise<void>;
  reloadInstances: () => Promise<void>;
  apiFetch: (path: string, init?: RequestInit) => Promise<Response>;
  toast: (kind: ToastKind, text: string) => void;
  logout: () => Promise<void>;
}

const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function useConsole() {
  const ctx = useContext(ConsoleContext);
  if (!ctx) throw new Error("useConsole must be used inside <ConsoleProvider>");
  return ctx;
}

const tenantPath = (t: string) => `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(t)}`;

function isStaleInstanceVersion(existing: InstanceItem, incoming: InstanceItem): boolean {
  if (existing.metadata.uid && incoming.metadata.uid && existing.metadata.uid !== incoming.metadata.uid) {
    return false;
  }
  const curRV = Number(existing.metadata.resourceVersion || 0);
  const nextRV = Number(incoming.metadata.resourceVersion || 0);
  if (curRV > 0 && nextRV > 0 && nextRV !== curRV) {
    return nextRV < curRV;
  }
  const curSV = Number(existing.metadata.statusVersion || 0);
  const nextSV = Number(incoming.metadata.statusVersion || 0);
  return curSV > 0 && nextSV > 0 && nextSV < curSV;
}

function resolvePreferredTenant(items: Tenant[], currentName: string): string {
  return (
    (currentName &&
      (items.find((x) => x.metadata.name === currentName) ||
        items.find((x) => x.spec?.type === "Personal" && x.spec?.owner === currentName) ||
        items.find((x) => x.spec?.owner === currentName))?.metadata.name) ||
    items[0]?.metadata.name ||
    ""
  );
}

export function ConsoleProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();

  const [ready, setReady] = useState(false);
  const [token, setToken] = useState<string | null>(null);
  const [userName, setUserName] = useState("");
  const [me, setMe] = useState<UserItem | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);

  const [tenants, setTenants] = useState<Tenant[]>([]);
  const routeTenant = useMemo(() => parseTenantFromPathname(pathname), [pathname]);
  const [storedTenant, setStoredTenant] = useState<string>(() => parseTenantFromPathname(pathname));
  const tenant = routeTenant || storedTenant;
  const tenantRef = useRef(tenant);
  const [tenantRole, setTenantRole] = useState<string>("tenant-admin");
  const [instances, setInstances] = useState<InstanceItem[]>([]);
  const [instancesTenant, setInstancesTenant] = useState("");
  const [instancesLoading, setInstancesLoading] = useState(true);
  const [live, setLive] = useState(false);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [runtimeClasses, setRuntimeClasses] = useState<RuntimeClass[]>([]);
  const [apps, setApps] = useState<AppItem[]>([]);
  const [users, setUsers] = useState<UserItem[]>([]);
  const [globalRoles, setGlobalRoles] = useState<GlobalRoleItem[]>([]);
  const [globalRoleBindings, setGlobalRoleBindings] = useState<GlobalRoleBindingItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const toastId = useRef(0);
  const refreshedMissingTenantRef = useRef("");

  useEffect(() => {
    tenantRef.current = tenant;
  }, [tenant]);

  // ---------------------------------------------------------------- session
  useEffect(() => {
    const t = readToken();
    const urlTenant =
      typeof window !== "undefined" ? parseTenantFromPathname(window.location.pathname) : "";
    if (!t) {
      const nextTarget =
        typeof window !== "undefined"
          ? `${window.location.pathname}${window.location.search}`
          : pathname || "/console";
      router.replace(`/login?next=${encodeURIComponent(nextTarget)}`);
      return;
    }
    setToken(t);
    setUserName(readUserName(t));
    const initialTenant = urlTenant || localStorage.getItem(TENANT_STORAGE_KEY) || "";
    if (urlTenant) {
      localStorage.setItem(TENANT_STORAGE_KEY, urlTenant);
    }
    setStoredTenant(initialTenant);
    setReady(true);
    // pathname intentionally excluded: only evaluated once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [router]);

  useEffect(() => {
    if (!routeTenant) return;
    setStoredTenant(routeTenant);
    if (typeof window !== "undefined") {
      localStorage.setItem(TENANT_STORAGE_KEY, routeTenant);
    }
  }, [routeTenant]);

  const toast = useCallback((kind: ToastKind, text: string) => {
    const id = ++toastId.current;
    setToasts((prev) => [...prev.slice(-2), { id, kind, text }]);
    setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== id)), kind === "error" ? 6000 : 3500);
  }, []);

  const apiFetch = useCallback(
    async (path: string, init?: RequestInit) => {
      const headers = new Headers(init?.headers || {});
      const t = token || readToken();
      if (t) headers.set("Authorization", `Bearer ${t}`);
      if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
      const res = await fetch(path, { ...init, headers });
      if (res.status === 401) {
        clearSession();
        if (typeof window !== "undefined") {
          window.location.replace("/login");
        } else {
          router.replace("/login");
        }
        throw new Error("登录已过期，请重新登录");
      }
      return res;
    },
    [token, router],
  );

  const setTenant = useCallback((name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setStoredTenant(trimmed);
    localStorage.setItem(TENANT_STORAGE_KEY, trimmed);
  }, []);

  // ------------------------------------------------------------------ loads
  const reloadInstances = useCallback(async () => {
    const targetTenant = tenant;
    if (!targetTenant) {
      setInstancesLoading(false);
      setInstancesTenant("");
      return;
    }
    setInstancesLoading(true);
    try {
      const res = await apiFetch(`${tenantPath(targetTenant)}/instances`);
      if (tenantRef.current !== targetTenant) return;
      if (res.ok) {
        const items: InstanceItem[] = (await res.json())?.items || [];
        if (tenantRef.current !== targetTenant) return;
        setInstances((prev) => {
          if (prev.length === 0) return items;
          const prevByName = new Map(prev.map((p) => [p.metadata.name, p]));
          return items.map((inc) => {
            const cur = prevByName.get(inc.metadata.name);
            return cur && isStaleInstanceVersion(cur, inc) ? cur : inc;
          });
        });
      }
    } finally {
      if (tenantRef.current === targetTenant) {
        setInstancesLoading(false);
        setInstancesTenant(targetTenant);
      }
    }
  }, [apiFetch, tenant]);

  const resolveCurrentSession = useCallback(async (): Promise<{ sessionName: string; userName: string } | null> => {
    const sid = readSessionName(token);
    if (sid) {
      const sRes = await apiFetch(`/apis/iam.gamepanel.io/v1/sessions/${encodeURIComponent(sid)}`);
      if (sRes.ok) {
        const sess = await sRes.json().catch(() => null);
        const sName = typeof sess?.metadata?.name === "string" ? sess.metadata.name : sid;
        const sUser = typeof sess?.user === "string" ? sess.user : "";
        if (sName) localStorage.setItem(SESSION_STORAGE_KEY, sName);
        if (sUser) {
          setUserName(sUser);
          localStorage.setItem(USER_STORAGE_KEY, sUser);
        }
        return { sessionName: sName, userName: sUser };
      }
    }
    const listRes = await apiFetch("/apis/iam.gamepanel.io/v1/sessions");
    if (!listRes.ok) return null;
    const body = await listRes.json().catch(() => null);
    const items: Array<{
      metadata?: { name?: string; annotations?: Record<string, string> };
      user?: string;
    }> = Array.isArray(body?.items) ? body.items : [];
    const cur = items.find((s) => s?.metadata?.annotations?.["iam.gamepanel.io/current"] === "true") || items[0];
    if (!cur) return null;
    const sName = typeof cur.metadata?.name === "string" ? cur.metadata.name : "";
    const sUser = typeof cur.user === "string" ? cur.user : "";
    if (sName) localStorage.setItem(SESSION_STORAGE_KEY, sName);
    if (sUser) {
      setUserName(sUser);
      localStorage.setItem(USER_STORAGE_KEY, sUser);
    }
    return { sessionName: sName, userName: sUser };
  }, [apiFetch, token]);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      let currentName = readUserName();
      if (!currentName) {
        const resolved = await resolveCurrentSession();
        if (resolved?.userName) {
          currentName = resolved.userName;
        }
      }
      const [meRes, tRes, rcRes, appsRes, nRes] = await Promise.all([
        currentName ? apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(currentName)}`) : Promise.resolve(null),
        apiFetch("/apis/core.gamepanel.io/v1/tenants"),
        apiFetch("/apis/core.gamepanel.io/v1/runtimeclasses"),
        apiFetch("/apis/core.gamepanel.io/v1/apps"),
        apiFetch("/apis/core.gamepanel.io/v1/nodes"),
      ]);
      if (meRes && meRes.ok) {
        const u: UserItem = await meRes.json();
        setMe(u);
        if (u?.metadata?.name) {
          currentName = u.metadata.name;
          setUserName(u.metadata.name);
          localStorage.setItem(USER_STORAGE_KEY, u.metadata.name);
        }
      } else if (meRes && !meRes.ok) {
        const resolved = await resolveCurrentSession();
        if (resolved?.userName) {
          currentName = resolved.userName;
          const retryMe = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(currentName)}`);
          if (retryMe.ok) setMe(await retryMe.json());
        }
      }
      if (tRes.ok) {
        const items: Tenant[] = (await tRes.json())?.items || [];
        setTenants(items);
        const activeRouteTenant =
          typeof window !== "undefined" ? parseTenantFromPathname(window.location.pathname) : "";
        const currentCandidate =
          activeRouteTenant ||
          tenantRef.current ||
          (typeof window !== "undefined" ? localStorage.getItem(TENANT_STORAGE_KEY) || "" : "");
        const fallbackTenant = resolvePreferredTenant(items, currentName);
        const resolved = items.some((x) => x.metadata.name === currentCandidate)
          ? currentCandidate
          : fallbackTenant;
        if (resolved) {
          if (typeof window !== "undefined") {
            localStorage.setItem(TENANT_STORAGE_KEY, resolved);
          }
          setStoredTenant(resolved);
          if (activeRouteTenant && activeRouteTenant !== resolved) {
            router.replace(tenantRoute(resolved));
          }
        } else {
          setStoredTenant("");
          if (activeRouteTenant) {
            router.replace("/console");
          }
        }
      }
      if (rcRes.ok) setRuntimeClasses((await rcRes.json())?.items || []);
      if (appsRes.ok) setApps((await appsRes.json())?.items || []);
      // Listing nodes is admin-only: use it as the capability probe for the platform admin area.
      if (nRes.ok) {
        setNodes((await nRes.json())?.items || []);
        setIsAdmin(true);
        const [uRes, gRes, grbRes] = await Promise.all([
          apiFetch("/apis/iam.gamepanel.io/v1/users"),
          apiFetch("/apis/iam.gamepanel.io/v1/globalroles"),
          apiFetch("/apis/iam.gamepanel.io/v1/globalrolebindings"),
        ]);
        if (uRes.ok) setUsers((await uRes.json())?.items || []);
        if (gRes.ok) setGlobalRoles((await gRes.json())?.items || []);
        if (grbRes.ok) setGlobalRoleBindings((await grbRes.json())?.items || []);
      } else {
        setIsAdmin(false);
      }
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "加载数据失败");
    } finally {
      setLoading(false);
    }
  }, [apiFetch, resolveCurrentSession, router, toast]);

  useEffect(() => {
    if (ready) void reload();
  }, [ready, reload]);

  useEffect(() => {
    if (!ready || !token || !tenant || loading) return;
    if (tenants.some((t) => t.metadata.name === tenant)) {
      refreshedMissingTenantRef.current = "";
      return;
    }
    if (refreshedMissingTenantRef.current === tenant) return;
    refreshedMissingTenantRef.current = tenant;
    void reload();
  }, [ready, token, tenant, tenants, loading, reload]);

  // Periodically verify that the current session has not been revoked from another device.
  useEffect(() => {
    if (!ready || !token) return;
    const checkSession = () => {
      if (typeof document !== "undefined" && document.visibilityState !== "visible") return;
      const sid = readSessionName(token);
      const url = sid
        ? `/apis/iam.gamepanel.io/v1/sessions/${encodeURIComponent(sid)}`
        : "/apis/iam.gamepanel.io/v1/sessions";
      void apiFetch(url).catch(() => undefined);
    };
    const timer = setInterval(checkSession, 10000);
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", checkSession);
    }
    return () => {
      clearInterval(timer);
      if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", checkSession);
      }
    };
  }, [ready, token, apiFetch]);

  // ------------------------------------------------- live instance stream (100% WebSocket ?watch=true)
  useEffect(() => {
    if (ready && !loading && (!token || !tenant)) {
      setInstancesLoading(false);
    }
  }, [ready, loading, token, tenant]);

  useEffect(() => {
    if (!token || !tenant) return;
    const subscribedTenant = tenant;
    setInstances([]);
    setInstancesTenant("");
    setInstancesLoading(true);
    setLive(false);
    void reloadInstances().catch(() => undefined);

    const stopWatch = subscribeResourceWatch<InstanceItem>({
      path: `${tenantPath(subscribedTenant)}/instances`,
      token,
      onConnectionChange: (connected) => {
        if (tenantRef.current === subscribedTenant) setLive(connected);
      },
      onReconnect: () => {
        void reloadInstances().catch(() => undefined);
      },
      onEvent: (ev) => {
        if (tenantRef.current !== subscribedTenant) return;
        const obj = ev?.object;
        const name = obj?.metadata?.name;
        if (!name) return;
        if (ev.type === "DELETED") {
          setInstances((prev) => prev.filter((item) => item.metadata.name !== name));
        } else if (ev.type === "ADDED" || ev.type === "MODIFIED") {
          setInstances((prev) => {
            const idx = prev.findIndex((item) => item.metadata.name === name);
            if (idx === -1) return [...prev, obj];
            if (isStaleInstanceVersion(prev[idx], obj)) return prev;
            const next = [...prev];
            next[idx] = obj;
            return next;
          });
        }
      },
    });

    return () => {
      stopWatch();
      setLive(false);
    };
  }, [token, tenant, reloadInstances]);

  const scopedInstances = useMemo(
    () => (instancesTenant === tenant ? instances : []),
    [instances, instancesTenant, tenant],
  );
  const scopedInstancesLoading = instancesLoading || (Boolean(tenant) && instancesTenant !== tenant);

  // ------------------------------------------------------------------ quota & tenant role
  const tenantObj = useMemo(() => tenants.find((t) => t.metadata.name === tenant), [tenants, tenant]);

  useEffect(() => {
    if (!token || !tenant || !userName) return;
    if (isAdmin || tenantObj?.spec?.owner === userName) {
      setTenantRole("tenant-admin");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await apiFetch(
          `/apis/iam.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/tenantrolebindings`,
        );
        if (cancelled) return;
        if (!res.ok) {
          setTenantRole("tenant-viewer");
          return;
        }
        const items: Array<{
          roleRef?: { name?: string };
          subjects?: Array<{ kind?: string; name?: string }>;
        }> = (await res.json())?.items || [];
        const rank = (r: string) =>
          r === "tenant-admin" ? 0 : r === "instance-operator" ? 1 : r === "tenant-viewer" ? 2 : 1;
        let best = "tenant-viewer";
        let bestRank = 99;
        for (const b of items) {
          const r = b.roleRef?.name || "";
          const matched = (b.subjects || []).some(
            (s) => (!s.kind || s.kind === "User") && (s.name === userName || s.name === "*"),
          );
          if (matched && rank(r) < bestRank) {
            best = r;
            bestRank = rank(r);
          }
        }
        if (!cancelled) setTenantRole(best);
      } catch {
        if (!cancelled) setTenantRole("tenant-viewer");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, tenant, userName, isAdmin, tenantObj, apiFetch]);

  const canManageTenant = isAdmin || tenantObj?.spec?.owner === userName || tenantRole === "tenant-admin";
  const canOperateInstances = canManageTenant || tenantRole === "instance-operator";

  const quota = useMemo<QuotaSummary>(() => {
    const q = tenantObj?.spec?.quota || {};
    const u = tenantObj?.status?.usage || {};
    const r = tenantObj?.status?.reserved || {};
    const activeInsts = scopedInstances.filter((i) => !i.metadata.deletionTimestamp && i.status?.phase !== "Terminating");
    const sumActive = (key: "cpu" | "memory" | "storage") =>
      activeInsts.reduce((acc, i) => acc + parseNumericQuantity(resourceOf(i, key) || "0"), 0);
    const sumAll = (key: "cpu" | "memory" | "storage") =>
      scopedInstances.reduce((acc, i) => acc + parseNumericQuantity(resourceOf(i, key) || "0"), 0);
    let liveBackups = 0;
    let liveBackupBytes = 0;
    for (const inst of scopedInstances) {
      for (const b of inst.status?.backups || []) {
        liveBackups += 1;
        liveBackupBytes += b.sizeBytes || 0;
      }
    }
    const usedBackups = scopedInstances.length > 0 ? liveBackups : parseInt(u.backups || "0", 10) || 0;
    const usedBackupGB =
      scopedInstances.length > 0
        ? Math.round((liveBackupBytes / (1024 * 1024 * 1024)) * 100) / 100
        : Math.round(parseNumericQuantity(u.backupStorage || "0") * 100) / 100;
    const reservedInsts = Math.max(scopedInstances.length, parseInt(r.instances || "0", 10) || 0);
    const reservedCpu = Math.max(sumAll("cpu"), parseNumericQuantity(r.cpu || "0") || 0);
    const reservedMem = Math.max(sumAll("memory"), parseNumericQuantity(r.memory || "0") || 0);
    const reservedDisk = Math.max(sumAll("storage"), parseNumericQuantity(r.storage || "0") || 0);
    return {
      instances: {
        used: activeInsts.length,
        reserved: reservedInsts,
        max: parseInt(q.instances || "10", 10) || 10,
      },
      cpu: {
        used: sumActive("cpu"),
        reserved: reservedCpu,
        max: parseNumericQuantity(q.cpu || "16") || 16,
      },
      memory: {
        used: sumActive("memory"),
        reserved: reservedMem,
        max: parseNumericQuantity(q.memory || "32Gi") || 32,
      },
      storage: {
        used: sumActive("storage"),
        reserved: reservedDisk,
        max: parseNumericQuantity(q.storage || "100Gi") || 100,
      },
      backups: { used: usedBackups, max: parseInt(q.backups || "10", 10) || 10 },
      backupStorage: { used: usedBackupGB, max: parseNumericQuantity(q.backupStorage || "50Gi") || 50 },
    };
  }, [tenantObj, scopedInstances]);

  const logout = useCallback(async () => {
    try {
      const sid = readSessionName(token) || (await resolveCurrentSession())?.sessionName || "";
      if (sid) {
        await apiFetch(`/apis/iam.gamepanel.io/v1/sessions/${encodeURIComponent(sid)}`, { method: "DELETE" });
      }
    } catch {
      // ignore
    }
    clearSession();
    router.replace("/");
  }, [apiFetch, resolveCurrentSession, router, token]);

  const value = useMemo<ConsoleContextValue>(
    () => ({
      ready,
      token,
      userName,
      me,
      isAdmin,
      tenantRole,
      canManageTenant,
      canOperateInstances,
      tenants,
      tenant,
      tenantObj,
      setTenant,
      instances: scopedInstances,
      instancesLoading: scopedInstancesLoading,
      live,
      nodes,
      runtimeClasses,
      apps,
      users,
      globalRoles,
      globalRoleBindings,
      quota,
      loading,
      reload,
      reloadInstances,
      apiFetch,
      toast,
      logout,
    }),
    [
      ready,
      token,
      userName,
      me,
      isAdmin,
      tenantRole,
      canManageTenant,
      canOperateInstances,
      tenants,
      tenant,
      tenantObj,
      setTenant,
      scopedInstances,
      scopedInstancesLoading,
      live,
      nodes,
      runtimeClasses,
      apps,
      users,
      globalRoles,
      globalRoleBindings,
      quota,
      loading,
      reload,
      reloadInstances,
      apiFetch,
      toast,
      logout,
    ],
  );

  return (
    <ConsoleContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed bottom-6 left-1/2 z-[100] flex w-full max-w-md -translate-x-1/2 flex-col items-center gap-2 px-4"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            role={t.kind === "error" ? "alert" : "status"}
            className={`pointer-events-auto flex items-center gap-2.5 rounded-full px-5 py-3 text-sm font-medium shadow-[0_16px_40px_-16px_rgba(20,22,31,0.45)] ${
              t.kind === "error"
                ? "bg-[#D92D20] text-white"
                : t.kind === "success"
                  ? "bg-[#14161F] text-white"
                  : "bg-white text-[#14161F] border border-[#E3E6F2]"
            }`}
          >
            {t.kind === "success" && <span className="h-2 w-2 rounded-full bg-[#3DBE6B]" />}
            {t.text}
          </div>
        ))}
      </div>
    </ConsoleContext.Provider>
  );
}
