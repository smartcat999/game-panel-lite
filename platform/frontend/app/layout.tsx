import type { Metadata } from "next";

import { Providers } from "@/components/providers";

import "./globals.css";

export const metadata: Metadata = {
  title: "GamePanel",
  description: "Hosted game server operations",
};

const preferenceBootstrap = `(() => {
  try {
    const locale = localStorage.getItem("gamepanel.locale");
    const theme = localStorage.getItem("gamepanel.theme");
    document.documentElement.lang = locale === "zh-CN" ? "zh-CN" : "en";
    document.documentElement.dataset.theme = theme === "light" || theme === "dark" ? theme : "system";
  } catch (_) {}
})();`;

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" data-theme="system" suppressHydrationWarning>
      <head><script dangerouslySetInnerHTML={{ __html: preferenceBootstrap }} /></head>
      <body><Providers>{children}</Providers></body>
    </html>
  );
}
