export type ConsoleSurface = "tenant" | "platform" | "account" | "public";
export type PlatformArea = "business" | "infrastructure";

export function consoleSurfaceForPathname(pathname: string): ConsoleSurface {
  if (pathname === "/platform" || pathname.startsWith("/platform/")) return "platform";
  if (pathname === "/account" || pathname.startsWith("/account/")) return "account";
  if (pathname === "/" || pathname.startsWith("/share/")) return "public";
  return "tenant";
}

export function platformAreaForPathname(pathname: string): PlatformArea | undefined {
  if (consoleSurfaceForPathname(pathname) !== "platform") return undefined;
  if (pathname === "/platform/regions" || pathname.startsWith("/platform/regions/")) return "infrastructure";
  return "business";
}
