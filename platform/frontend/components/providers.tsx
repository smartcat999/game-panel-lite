"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useMemo, useState } from "react";

import { catalogs, type Locale, type MessageKey } from "@/lib/messages";

type Theme = "light" | "dark" | "system";

type PreferencesContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  theme: Theme;
  setTheme: (theme: Theme) => void;
  t: (key: MessageKey) => string;
};

const PreferencesContext = createContext<PreferencesContextValue | null>(null);

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => new QueryClient());
  const [locale, setLocaleState] = useState<Locale>("en");
  const [theme, setThemeState] = useState<Theme>("system");

  useEffect(() => {
    const storedLocale = window.localStorage.getItem("gamepanel.locale");
    const nextLocale: Locale = storedLocale === "zh-CN" || (!storedLocale && navigator.language.startsWith("zh")) ? "zh-CN" : "en";
    const storedTheme = window.localStorage.getItem("gamepanel.theme");
    const nextTheme: Theme = storedTheme === "light" || storedTheme === "dark" ? storedTheme : "system";
    document.documentElement.lang = nextLocale;
    document.documentElement.dataset.theme = nextTheme;
    setLocaleState(nextLocale);
    setThemeState(nextTheme);
  }, []);

  const setLocale = (nextLocale: Locale) => {
    window.localStorage.setItem("gamepanel.locale", nextLocale);
    document.documentElement.lang = nextLocale;
    setLocaleState(nextLocale);
  };

  const setTheme = (nextTheme: Theme) => {
    window.localStorage.setItem("gamepanel.theme", nextTheme);
    document.documentElement.dataset.theme = nextTheme;
    setThemeState(nextTheme);
  };

  const value = useMemo<PreferencesContextValue>(() => ({
    locale,
    setLocale,
    theme,
    setTheme,
    t: (key) => catalogs[locale][key],
  }), [locale, theme]);

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
