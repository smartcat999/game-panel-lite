import type { Metadata } from "next";
import { cookies } from "next/headers";

import { Providers } from "@/components/providers";
import type { UserPreferences } from "@/lib/control-plane";

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

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const cookieStore = await cookies();
  let storedPreferences: UserPreferences | undefined;
  if (process.env.GAMEPANEL_CONTROL_PLANE_URL) {
    try {
      const response = await fetch(`${process.env.GAMEPANEL_CONTROL_PLANE_URL}/v1/user-preferences`, {
        cache: "no-store",
        headers: { Authorization: "Bearer local-preview" },
      });
      if (response.ok) {
        storedPreferences = await response.json() as UserPreferences;
      }
    } catch {
      storedPreferences = undefined;
    }
  }
  const locale = storedPreferences?.locale ?? (cookieStore.get("gamepanel.locale")?.value === "zh-CN" ? "zh-CN" : "en");
  const themeCookie = cookieStore.get("gamepanel.theme")?.value;
  const theme = storedPreferences?.theme ?? (themeCookie === "light" || themeCookie === "dark" ? themeCookie : "system");
  const timeZone = storedPreferences?.timeZone ?? cookieStore.get("gamepanel.timeZone")?.value ?? "Asia/Shanghai";
  return (
    <html lang={locale} data-theme={theme} suppressHydrationWarning>
      <head><script dangerouslySetInnerHTML={{ __html: preferenceBootstrap }} /></head>
      <body><Providers initialLocale={locale} initialTheme={theme} initialTimeZone={timeZone}>{children}</Providers></body>
    </html>
  );
}
