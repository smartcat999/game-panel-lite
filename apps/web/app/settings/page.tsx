"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Languages, Monitor, Moon, Save, Sun } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { ConsolePageHeader } from "@/components/console-page-header";
import { Button, Input } from "@/components/ui";
import { getSettings, updateAccountPreferences, updatePublicHost } from "@/lib/api";
import { useI18n, type Locale } from "@/lib/i18n";
import { useTheme } from "@/lib/theme";
import type { AuthBootstrap, ThemeMode } from "@/lib/types";
import { cn } from "@/lib/utils";

export default function SettingsPage() {
  const { locale, setLocale } = useI18n();
  const { theme, setTheme } = useTheme();
  const isZh = locale === "zh";
  const queryClient = useQueryClient();
  const [host, setHost] = useState("");
  const [hostSaved, setHostSaved] = useState(false);
  const [preferenceSaved, setPreferenceSaved] = useState(false);

  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });

  useEffect(() => {
    if (settingsQuery.data?.publicHost) setHost(settingsQuery.data.publicHost);
  }, [settingsQuery.data?.publicHost]);

  const preferenceMutation = useMutation({
    mutationFn: ({ nextLocale, nextTheme }: { nextLocale: Locale; nextTheme: ThemeMode }) =>
      updateAccountPreferences({ locale: nextLocale, theme: nextTheme }),
    onSuccess: (preferences) => {
      setLocale(preferences.locale);
      setTheme(preferences.theme);
      queryClient.setQueryData<AuthBootstrap>(["auth-bootstrap"], (current) => current?.account
        ? { ...current, account: { ...current.account, preferences } }
        : current);
      setPreferenceSaved(true);
      window.setTimeout(() => setPreferenceSaved(false), 2000);
    },
    onError: () => {
      const saved = queryClient.getQueryData<AuthBootstrap>(["auth-bootstrap"])?.account?.preferences;
      if (!saved) return;
      setLocale(saved.locale);
      setTheme(saved.theme);
    }
  });

  const savePreferences = (nextLocale: Locale, nextTheme: ThemeMode) => {
    setLocale(nextLocale);
    setTheme(nextTheme);
    preferenceMutation.mutate({ nextLocale, nextTheme });
  };

  const hostMutation = useMutation({
    mutationFn: updatePublicHost,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      setHostSaved(true);
      window.setTimeout(() => setHostSaved(false), 2000);
    }
  });

  return (
    <div className="space-y-3">
      <ConsolePageHeader title={isZh ? "账号设置" : "Account settings"} />

      <section className="max-w-2xl space-y-5 rounded-xl border bg-white p-5 micro-border subtle-elevation">
        <div className="flex items-start gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
            <Languages className="size-4" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-900">{isZh ? "界面偏好" : "Interface preferences"}</h2>
            <p className="mt-0.5 text-[11px] leading-relaxed text-slate-500">
              {isZh ? "语言和主题会保存到当前账号，并在其他设备登录时自动同步。" : "Language and theme are saved to this account and synced when you sign in on another device."}
            </p>
          </div>
        </div>

        <PreferenceGroup label={isZh ? "语言" : "Language"}>
          <PreferenceButton active={locale === "zh"} disabled={preferenceMutation.isPending} onClick={() => savePreferences("zh", theme)} label="简体中文" />
          <PreferenceButton active={locale === "en"} disabled={preferenceMutation.isPending} onClick={() => savePreferences("en", theme)} label="English" />
        </PreferenceGroup>

        <PreferenceGroup label={isZh ? "主题" : "Theme"}>
          <PreferenceButton active={theme === "light"} disabled={preferenceMutation.isPending} onClick={() => savePreferences(locale, "light")} label={isZh ? "浅色" : "Light"} icon={<Sun className="size-3.5" />} />
          <PreferenceButton active={theme === "dark"} disabled={preferenceMutation.isPending} onClick={() => savePreferences(locale, "dark")} label={isZh ? "深色" : "Dark"} icon={<Moon className="size-3.5" />} />
          <PreferenceButton active={theme === "system"} disabled={preferenceMutation.isPending} onClick={() => savePreferences(locale, "system")} label={isZh ? "跟随系统" : "System"} icon={<Monitor className="size-3.5" />} />
        </PreferenceGroup>

        <div className="min-h-5 border-t border-slate-100 pt-3 text-[11px]">
          {preferenceSaved && <span className="flex items-center gap-1 font-semibold text-emerald-600"><Check className="size-3.5" />{isZh ? "账号偏好已保存" : "Account preferences saved"}</span>}
          {preferenceMutation.isError && <span className="font-semibold text-rose-600">{isZh ? "保存失败，请稍后重试" : "Could not save. Please try again."}</span>}
        </div>
      </section>

      <form onSubmit={(event) => { event.preventDefault(); hostMutation.mutate(host.trim()); }} className="max-w-2xl space-y-4 rounded-xl border bg-white p-5 micro-border subtle-elevation">
        <div>
          <h2 className="text-xs font-bold uppercase tracking-wider text-slate-900">{isZh ? "公网连接" : "Public endpoint"}</h2>
          <p className="mt-0.5 text-[11px] text-slate-500">{isZh ? "玩家连接实例时显示的公网地址。" : "The public address shown to players when they connect."}</p>
        </div>
        <label className="block text-xs font-semibold text-slate-700">
          {isZh ? "公网主机地址" : "Public host"}
          <Input value={host} onChange={(event) => setHost(event.target.value)} placeholder={isZh ? "例如：play.example.com" : "For example: play.example.com"} className="mt-1 font-mono text-xs" />
        </label>
        <div className="flex items-center justify-between border-t border-slate-100 pt-3">
          {hostSaved ? <span className="flex items-center gap-1 text-xs font-semibold text-emerald-600"><Check className="size-3.5" />{isZh ? "设置已保存" : "Settings saved"}</span> : <span />}
          <Button type="submit" disabled={hostMutation.isPending} className="h-8 bg-slate-900 px-4 text-xs text-white hover:bg-slate-800">
            <Save className="size-3.5" />{hostMutation.isPending ? (isZh ? "保存中…" : "Saving…") : (isZh ? "保存" : "Save")}
          </Button>
        </div>
      </form>
    </div>
  );
}

function PreferenceGroup({ label, children }: { label: string; children: ReactNode }) {
  return <div className="space-y-2"><div className="text-[11px] font-semibold text-slate-600">{label}</div><div className="flex flex-wrap gap-2">{children}</div></div>;
}

function PreferenceButton({ active, disabled, onClick, label, icon }: { active: boolean; disabled: boolean; onClick: () => void; label: string; icon?: ReactNode }) {
  return (
    <button type="button" disabled={disabled} onClick={onClick} aria-pressed={active} className={cn("flex h-9 items-center gap-2 rounded-lg border px-3 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-70", active ? "border-emerald-500 bg-emerald-50 text-emerald-800" : "border-slate-200 bg-white text-slate-600 hover:border-slate-300 hover:bg-slate-50")}>
      {icon}{label}{active && <Check className="size-3.5" />}
    </button>
  );
}
