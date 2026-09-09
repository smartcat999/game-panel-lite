import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  outputFileTracingRoot: process.cwd(),
  poweredByHeader: false,
  async rewrites() {
    return [{
      source: "/control-plane/:path*",
      destination: `${process.env.GAMEPANEL_CONTROL_PLANE_URL ?? "http://127.0.0.1:8080"}/:path*`,
    }];
  },
};

export default nextConfig;
