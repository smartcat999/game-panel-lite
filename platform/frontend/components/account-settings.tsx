"use client";

import { ArrowLeft, Languages, MonitorCog } from "lucide-react";
import Link from "next/link";

import { usePreferences } from "@/components/providers";

export function AccountSettings() {
  const { locale, setLocale, theme, setTheme, timeZone, setTimeZone, t } = usePreferences();

  return (
    <div className="account-page">
      <header className="account-header">
        <Link href="/w/northstar"><ArrowLeft size={17} aria-hidden="true" />{t("account.backToWorkspace")}</Link>
        <strong>{t("product.name")}</strong>
      </header>
      <main className="account-content">
        <div className="account-heading">
          <h1>{t("account.settingsTitle")}</h1>
          <p>{t("account.settingsDescription")}</p>
        </div>
        <section className="settings-section" aria-labelledby="preferences-heading">
          <div className="settings-section-title">
            <MonitorCog size={20} aria-hidden="true" />
            <h2 id="preferences-heading">{t("account.preferences")}</h2>
          </div>
          <div className="setting-row">
            <div><label htmlFor="account-locale">{t("common.locale")}</label><p>{t("account.localeHelp")}</p></div>
            <select id="account-locale" value={locale} onChange={(event) => setLocale(event.target.value as "en" | "zh-CN")}>
              <option value="en">{t("locale.english")}</option><option value="zh-CN">{t("locale.chinese")}</option>
            </select>
          </div>
          <div className="setting-row">
            <div><label htmlFor="account-theme">{t("common.theme")}</label><p>{t("account.themeHelp")}</p></div>
            <select id="account-theme" value={theme} onChange={(event) => setTheme(event.target.value as "light" | "dark" | "system")}>
              <option value="system">{t("theme.system")}</option><option value="light">{t("theme.light")}</option><option value="dark">{t("theme.dark")}</option>
            </select>
          </div>
          <div className="setting-row">
            <div><label htmlFor="account-time-zone">{t("account.timeZone")}</label><p>{t("account.timeZoneHelp")}</p></div>
            <select id="account-time-zone" value={timeZone} onChange={(event) => setTimeZone(event.target.value)}>
              <option value="Asia/Shanghai">Asia/Shanghai</option><option value="UTC">UTC</option><option value="America/Los_Angeles">America/Los_Angeles</option>
            </select>
          </div>
          <div className="settings-footnote"><Languages size={16} aria-hidden="true" />{t("locale.current")}</div>
        </section>
      </main>
    </div>
  );
}
