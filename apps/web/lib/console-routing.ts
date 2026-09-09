export type ConsoleSurface = "tenant" | "platform" | "account" | "public";

export function consoleSurfaceForPathname(pathname: string): ConsoleSurface {
  if (pathname === "/platform" || pathname.startsWith("/platform/")) return "platform";
  if (pathname === "/account" || pathname.startsWith("/account/")) return "account";
  if (pathname === "/" || pathname.startsWith("/share/")) return "public";
  return "tenant";
}
