"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useMemo, useState } from "react";

import { catalogs, type Locale, type MessageKey } from "@/lib/messages";
import { controlPlane } from "@/lib/control-plane";

type Theme = "light" | "dark" | "system";

type PreferencesContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  theme: Theme;
  setTheme: (theme: Theme) => void;
  timeZone: string;
  setTimeZone: (timeZone: string) => void;
  t: (key: MessageKey) => string;
};

const PreferencesContext = createContext<PreferencesContextValue | null>(null);

export function Providers({ children, initialLocale, initialTheme, initialTimeZone }: { children: React.ReactNode; initialLocale: Locale; initialTheme: Theme; initialTimeZone: string }) {
  const [queryClient] = useState(() => new QueryClient());
  const [locale, setLocaleState] = useState<Locale>(initialLocale);
  const [theme, setThemeState] = useState<Theme>(initialTheme);
  const [timeZone, setTimeZoneState] = useState(initialTimeZone);

  useEffect(() => {
    const storedLocale = window.localStorage.getItem("gamepanel.locale");
    const nextLocale: Locale = storedLocale === "zh-CN" ? "zh-CN" : initialLocale;
    const storedTheme = window.localStorage.getItem("gamepanel.theme");
    const nextTheme: Theme = storedTheme === "light" || storedTheme === "dark" ? storedTheme : initialTheme;
    const nextTimeZone = window.localStorage.getItem("gamepanel.timeZone") ?? initialTimeZone;
    document.documentElement.lang = nextLocale;
    document.documentElement.dataset.theme = nextTheme;
    setLocaleState(nextLocale);
    setThemeState(nextTheme);
    setTimeZoneState(nextTimeZone);
    void controlPlane.preferences().then((preferences) => {
      document.documentElement.lang = preferences.locale;
      document.documentElement.dataset.theme = preferences.theme;
      setLocaleState(preferences.locale);
      setThemeState(preferences.theme);
      setTimeZoneState(preferences.timeZone);
    }).catch(() => undefined);
  }, [initialLocale, initialTheme, initialTimeZone]);

  const setLocale = (nextLocale: Locale) => {
    window.localStorage.setItem("gamepanel.locale", nextLocale);
    document.cookie = `gamepanel.locale=${nextLocale}; Path=/; Max-Age=31536000; SameSite=Lax`;
    document.documentElement.lang = nextLocale;
    setLocaleState(nextLocale);
    void controlPlane.updatePreferences({ locale: nextLocale }).catch(() => undefined);
  };

  const setTheme = (nextTheme: Theme) => {
    window.localStorage.setItem("gamepanel.theme", nextTheme);
    document.cookie = `gamepanel.theme=${nextTheme}; Path=/; Max-Age=31536000; SameSite=Lax`;
    document.documentElement.dataset.theme = nextTheme;
    setThemeState(nextTheme);
    void controlPlane.updatePreferences({ theme: nextTheme }).catch(() => undefined);
  };

  const setTimeZone = (nextTimeZone: string) => {
    window.localStorage.setItem("gamepanel.timeZone", nextTimeZone);
    document.cookie = `gamepanel.timeZone=${encodeURIComponent(nextTimeZone)}; Path=/; Max-Age=31536000; SameSite=Lax`;
    setTimeZoneState(nextTimeZone);
    void controlPlane.updatePreferences({ timeZone: nextTimeZone }).catch(() => undefined);
  };

  const value = useMemo<PreferencesContextValue>(() => ({
    locale,
    setLocale,
    theme,
    setTheme,
    timeZone,
    setTimeZone,
    t: (key) => catalogs[locale][key],
  }), [locale, theme, timeZone]);

  return (
    <QueryClientProvider client={queryClient}>
      <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>
    </QueryClientProvider>
  );
}

export function usePreferences() {
  const value = useContext(PreferencesContext);
  if (!value) {
    throw new Error("usePreferences must be used within Providers");
  }
  return value;
}
