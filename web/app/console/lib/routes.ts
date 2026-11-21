const TENANT_PATH_RE = /^\/console\/t\/([^/?#]+)(\/[^?#]*)?/;

export function parseTenantFromPathname(pathname: string | null | undefined): string {
  if (!pathname) return "";
  const match = pathname.match(TENANT_PATH_RE);
  if (!match) return "";
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1];
  }
}

export function parseTenantSubpath(pathname: string | null | undefined): string | null {
  if (!pathname) return null;
  const match = pathname.match(TENANT_PATH_RE);
  if (!match) return null;
  const sub = match[2] || "";
  return sub === "/" ? "" : sub;
}

export function tenantRoute(tenant: string, subpath = ""): string {
  const t = tenant.trim();
  if (!t) return "/console";
  const s = subpath.trim();
  let normSub = "";
  if (s && s !== "/") {
    normSub = s.startsWith("/") || s.startsWith("?") ? s : `/${s}`;
  }
  return `/console/t/${encodeURIComponent(t)}${normSub}`;
}

export function tenantServerRoute(tenant: string, name: string, query?: string): string {
  const base = tenantRoute(tenant, `/servers/${encodeURIComponent(name)}`);
  const q = query?.trim();
  if (!q) return base;
  return `${base}${q.startsWith("?") ? q : `?${q}`}`;
}

export function isTenantServerDetailPath(pathname: string | null | undefined): boolean {
  const sub = parseTenantSubpath(pathname);
  return sub !== null && sub.startsWith("/servers/");
}
