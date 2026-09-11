import type { Metadata } from "next";
import { cookies } from "next/headers";

import { Providers } from "@/components/providers";

import "./globals.css";

export const metadata: Metadata = {
  title: "GamePanel",
  description: "Hosted game server operations",
};

const preferenceBootstrap = `(() => {
  try {
    const locale = localStorage.getItem("gamepanel.locale");
    document.documentElement.lang = locale === "en" ? "en" : "zh-CN";
    document.documentElement.dataset.theme = "dark";
    localStorage.setItem("gamepanel.theme", "dark");
  } catch (_) {}
})();`;

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const cookieStore = await cookies();
  const locale = cookieStore.get("gamepanel.locale")?.value === "en" ? "en" : "zh-CN";
  const theme = "dark";
  const timeZone = cookieStore.get("gamepanel.timeZone")?.value ?? "Asia/Shanghai";
  return (
    <html lang={locale} data-theme="dark" suppressHydrationWarning>
      <head><script dangerouslySetInnerHTML={{ __html: preferenceBootstrap }} /></head>
      <body><Providers initialLocale={locale} initialTheme={theme} initialTimeZone={timeZone}>{children}</Providers></body>
    </html>
  );
}
