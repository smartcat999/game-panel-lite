import { NextRequest, NextResponse } from "next/server";

export function middleware(req: NextRequest) {
  // Use dynamic key lookup so Next.js compiler does not inline undefined at build time.
  const k1 = "GAMEPANEL_APISERVER_URL";
  const k2 = "NEXT_PUBLIC_APISERVER_URL";
  const origin = (process.env[k1] || process.env[k2] || "http://127.0.0.1:8080").replace(/\/+$/, "");
  const target = new URL(`${req.nextUrl.pathname}${req.nextUrl.search}`, origin);
  return NextResponse.rewrite(target);
}

export const config = {
  matcher: ["/apis/:path*", "/openapi/:path*", "/livez", "/readyz"],
};
