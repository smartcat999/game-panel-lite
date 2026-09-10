"use client";

import { PrototypeProvider } from "@/lib/prototype-store";

export function Providers({ children }: { children: React.ReactNode; initialLocale: string; initialTheme: string; initialTimeZone: string }) {
  return <PrototypeProvider>{children}</PrototypeProvider>;
}
