import "./globals.css";
import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import { AuthGate } from "@/components/auth-gate";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: "GamePanel Lite | Self-hosted game server panel",
  description: "Modern lightweight self-hosted panel for Docker-backed game servers, lifecycle operations, logs, and mods."
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>
        <Providers>
          <AuthGate>
            <AppShell>{children}</AppShell>
          </AuthGate>
        </Providers>
      </body>
    </html>
  );
}
