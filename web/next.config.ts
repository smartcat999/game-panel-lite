import type { NextConfig } from "next";

const apiserverOrigin =
  process.env.GAMEPANEL_APISERVER_URL ||
  process.env.NEXT_PUBLIC_APISERVER_URL ||
  "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: process.cwd(),
  poweredByHeader: false,
  compress: false,
  // Landing images are pre-compressed; skip runtime optimisation so the
  // standalone image does not need a platform-specific sharp binary.
  images: { unoptimized: true },
  async rewrites() {
    return [
      { source: "/apis/:path*", destination: `${apiserverOrigin}/apis/:path*` },
      { source: "/openapi/:path*", destination: `${apiserverOrigin}/openapi/:path*` },
      { source: "/livez", destination: `${apiserverOrigin}/livez` },
      { source: "/readyz", destination: `${apiserverOrigin}/readyz` },
    ];
  },
};

export default nextConfig;
