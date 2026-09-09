"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Languages, Monitor, Moon, Sun } from "lucide-react";
import { useState, type ReactNode } from "react";
import { ConsolePageHeader } from "@/components/console-page-header";
import { updateAccountPreferences } from "@/lib/api";
import { useI18n, type Locale } from "@/lib/i18n";
import { useTheme } from "@/lib/theme";
import type { AuthBootstrap, ThemeMode } from "@/lib/types";
import { cn } from "@/lib/utils";

export default function AccountSettingsPage() {
  const { locale, setLocale } = useI18n();
  const { theme, setTheme } = useTheme();
  const isZh = locale === "zh";
  const queryClient = useQueryClient();
  const [saved, setSaved] = useState(false);

  const mutation = useMutation({
    mutationFn: ({ nextLocale, nextTheme }: { nextLocale: Locale; nextTheme: ThemeMode }) =>
      updateAccountPreferences({ locale: nextLocale, theme: nextTheme }),
    onSuccess: (preferences) => {
      setLocale(preferences.locale);
      setTheme(preferences.theme);
      queryClient.setQueryData<AuthBootstrap>(["auth-bootstrap"], (current) => current?.account
        ? { ...current, account: { ...current.account, preferences } }
        : current);
      setSaved(true);
      window.setTimeout(() => setSaved(false), 2000);
    },
    onError: () => {
      const preferences = queryClient.getQueryData<AuthBootstrap>(["auth-bootstrap"])?.account?.preferences;
      if (!preferences) return;
      setLocale(preferences.locale);
      setTheme(preferences.theme);
    }
  });

  const save = (nextLocale: Locale, nextTheme: ThemeMode) => {
    setLocale(nextLocale);
    setTheme(nextTheme);
    mutation.mutate({ nextLocale, nextTheme });
  };

  return (
    <div className="space-y-3">
      <ConsolePageHeader title={isZh ? "账号设置" : "Account settings"} />
      <section className="max-w-2xl space-y-5 rounded-xl border bg-white p-5 micro-border subtle-elevation">
        <div className="flex items-start gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600"><Languages className="size-4" /></div>
          <div>
            <h2 className="text-sm font-bold text-slate-900">{isZh ? "界面偏好" : "Interface preferences"}</h2>
            <p className="mt-0.5 text-[11px] leading-relaxed text-slate-500">
              {isZh ? "语言和主题绑定当前账号，并在租户控制台与平台控制台之间保持一致。" : "Language and theme belong to this account and remain consistent across tenant and platform consoles."}
            </p>
          </div>
        </div>

        <PreferenceGroup label={isZh ? "语言" : "Language"}>
          <PreferenceButton active={locale === "zh"} disabled={mutation.isPending} onClick={() => save("zh", theme)} label="简体中文" />
          <PreferenceButton active={locale === "en"} disabled={mutation.isPending} onClick={() => save("en", theme)} label="English" />
        </PreferenceGroup>

        <PreferenceGroup label={isZh ? "主题" : "Theme"}>
          <PreferenceButton active={theme === "light"} disabled={mutation.isPending} onClick={() => save(locale, "light")} label={isZh ? "浅色" : "Light"} icon={<Sun className="size-3.5" />} />
          <PreferenceButton active={theme === "dark"} disabled={mutation.isPending} onClick={() => save(locale, "dark")} label={isZh ? "深色" : "Dark"} icon={<Moon className="size-3.5" />} />
          <PreferenceButton active={theme === "system"} disabled={mutation.isPending} onClick={() => save(locale, "system")} label={isZh ? "跟随系统" : "System"} icon={<Monitor className="size-3.5" />} />
        </PreferenceGroup>

        <div className="min-h-5 border-t border-slate-100 pt-3 text-[11px]">
          {saved ? <span className="flex items-center gap-1 font-semibold text-emerald-600"><Check className="size-3.5" />{isZh ? "账号偏好已保存" : "Account preferences saved"}</span> : null}
          {mutation.isError ? <span className="font-semibold text-rose-600">{isZh ? "保存失败，请稍后重试" : "Could not save. Please try again."}</span> : null}
        </div>
      </section>
    </div>
  );
}

function PreferenceGroup({ label, children }: { label: string; children: ReactNode }) {
  return <div className="space-y-2"><div className="text-[11px] font-semibold text-slate-600">{label}</div><div className="flex flex-wrap gap-2">{children}</div></div>;
}

function PreferenceButton({ active, disabled, onClick, label, icon }: { active: boolean; disabled: boolean; onClick: () => void; label: string; icon?: ReactNode }) {
  return (
    <button type="button" disabled={disabled} onClick={onClick} aria-pressed={active} className={cn("flex h-9 items-center gap-2 rounded-lg border px-3 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-70", active ? "border-emerald-500 bg-emerald-50 text-emerald-800" : "border-slate-200 bg-white text-slate-600 hover:border-slate-300 hover:bg-slate-50")}>
      {icon}{label}{active ? <Check className="size-3.5" /> : null}
    </button>
  );
}
