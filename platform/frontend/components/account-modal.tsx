"use client";

import { Check, Globe, KeyRound, Laptop, Lock, Moon, Palette, Shield, Sun, User, X } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function AccountModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { locale } = useI18n();
  const [activeTab, setActiveTab] = useState<"preferences" | "security" | "profile">("preferences");
  const [currentTheme, setCurrentTheme] = useState("dark");
  const [selectedLocale, setSelectedLocale] = useState(locale);

  // Password change state
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordStatus, setPasswordStatus] = useState<"idle" | "saving" | "success" | "error">("idle");
  const [errorMessage, setErrorMessage] = useState("");

  useEffect(() => {
    if (typeof window !== "undefined") {
      const theme = localStorage.getItem("gamepanel.theme") ?? "dark";
      setCurrentTheme(theme);
    }
  }, [open]);

  if (!open) return null;

  const handleThemeChange = (theme: "dark" | "light" | "system") => {
    setCurrentTheme(theme);
    document.cookie = `gamepanel.theme=${theme}; path=/; max-age=31536000`;
    localStorage.setItem("gamepanel.theme", theme);
    document.documentElement.dataset.theme = theme === "system" ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light") : theme;
  };

  const handleLocaleChange = (lang: "zh-CN" | "en") => {
    setSelectedLocale(lang);
    document.cookie = `gamepanel.locale=${lang}; path=/; max-age=31536000`;
    localStorage.setItem("gamepanel.locale", lang);
    window.location.reload();
  };

  const handlePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password.length < 8) {
      setErrorMessage(locale === "zh-CN" ? "密码长度至少需要 8 个字符" : "Password must be at least 8 characters");
      setPasswordStatus("error");
      return;
    }
    if (password !== confirmPassword) {
      setErrorMessage(locale === "zh-CN" ? "两次输入的密码不一致" : "Passwords do not match");
      setPasswordStatus("error");
      return;
    }

    setPasswordStatus("saving");
    setErrorMessage("");
    try {
      await api<void>("/auth/password", { method: "PUT", body: JSON.stringify({ password }) });
      setPasswordStatus("success");
      setPassword("");
      setConfirmPassword("");
      setTimeout(() => setPasswordStatus("idle"), 3000);
    } catch {
      setErrorMessage(locale === "zh-CN" ? "更新密码失败，请稍后重试" : "Failed to update password");
      setPasswordStatus("error");
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md animate-in fade-in-50 duration-150">
      <div
        className="w-full max-w-2xl bg-[#0e131b] border border-white/[0.12] rounded-2xl shadow-2xl overflow-hidden flex flex-col md:flex-row min-h-[460px] animate-in zoom-in-95 duration-150"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal Left Sidebar: Tabs */}
        <aside className="w-full md:w-52 border-b md:border-b-0 md:border-r border-white/[0.08] bg-[#0a0d13] p-4 flex flex-col justify-between shrink-0">
          <div className="space-y-4">
            <div className="flex items-center gap-2.5 px-2">
              <div className="w-6 h-6 rounded-md bg-emerald-500/20 border border-emerald-500/30 flex items-center justify-center text-emerald-400">
                <User size={13} />
              </div>
              <span className="text-xs font-bold text-white tracking-tight">
                {locale === "zh-CN" ? "个人偏好与设置" : "Account Settings"}
              </span>
            </div>

            <nav className="space-y-1">
              <button
                type="button"
                onClick={() => setActiveTab("preferences")}
                className={cn(
                  "w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left",
                  activeTab === "preferences"
                    ? "bg-white/[0.08] text-white font-semibold"
                    : "text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.03]"
                )}
              >
                <Palette size={14} className={activeTab === "preferences" ? "text-emerald-400" : "text-zinc-500"} />
                {locale === "zh-CN" ? "界面与语言" : "Interface & Language"}
              </button>

              <button
                type="button"
                onClick={() => setActiveTab("security")}
                className={cn(
                  "w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left",
                  activeTab === "security"
                    ? "bg-white/[0.08] text-white font-semibold"
                    : "text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.03]"
                )}
              >
                <Lock size={14} className={activeTab === "security" ? "text-emerald-400" : "text-zinc-500"} />
                {locale === "zh-CN" ? "密码与安全" : "Password & Security"}
              </button>

              <button
                type="button"
                onClick={() => setActiveTab("profile")}
                className={cn(
                  "w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left",
                  activeTab === "profile"
                    ? "bg-white/[0.08] text-white font-semibold"
                    : "text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.03]"
                )}
              >
                <Shield size={14} className={activeTab === "profile" ? "text-emerald-400" : "text-zinc-500"} />
                {locale === "zh-CN" ? "账号信息" : "Profile Details"}
              </button>
            </nav>
          </div>

          <div className="pt-4 border-t border-white/[0.06] text-[10px] text-zinc-500 font-mono px-2">
            GamePanel v2.4
          </div>
        </aside>

        {/* Modal Right Main Content Area */}
        <div className="flex-1 flex flex-col min-w-0 bg-[#0e131b]">
          {/* Top Bar with Close Button */}
          <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.06]">
            <h2 className="text-sm font-semibold text-white">
              {activeTab === "preferences" && (locale === "zh-CN" ? "界面与偏好设置" : "Interface & Preferences")}
              {activeTab === "security" && (locale === "zh-CN" ? "密码修改与安全认证" : "Password & Security")}
              {activeTab === "profile" && (locale === "zh-CN" ? "账号及权限详情" : "Profile Details")}
            </h2>
            <button
              type="button"
              onClick={onClose}
              className="p-1 rounded-md text-zinc-400 hover:text-white hover:bg-white/[0.06] transition-colors"
            >
              <X size={16} />
            </button>
          </div>

          {/* Body Content by Tab */}
          <div className="p-6 overflow-y-auto flex-1 space-y-6">
            {/* TAB 1: PREFERENCES */}
            {activeTab === "preferences" && (
              <div className="space-y-6">
                {/* Language Setting */}
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-zinc-200 flex items-center gap-2">
                    <Globe size={14} className="text-emerald-400" />
                    {locale === "zh-CN" ? "显示语言 / Display Language" : "Display Language"}
                  </label>
                  <p className="text-[11px] text-zinc-400">
                    {locale === "zh-CN" ? "选择管理平台所使用的默认交互界面语言" : "Select default language for interface localization"}
                  </p>
                  <div className="grid grid-cols-2 gap-3 pt-1">
                    <button
                      type="button"
                      onClick={() => handleLocaleChange("zh-CN")}
                      className={cn(
                        "flex items-center justify-between p-3 rounded-xl border text-xs font-medium transition-all text-left",
                        selectedLocale === "zh-CN"
                          ? "bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold"
                          : "bg-white/[0.02] border-white/[0.08] text-zinc-300 hover:bg-white/[0.05]"
                      )}
                    >
                      <div className="flex flex-col">
                        <span>简体中文</span>
                        <span className="text-[10px] text-zinc-500 font-mono">Simplified Chinese</span>
                      </div>
                      {selectedLocale === "zh-CN" && <Check size={15} className="text-emerald-400" />}
                    </button>

                    <button
                      type="button"
                      onClick={() => handleLocaleChange("en")}
                      className={cn(
                        "flex items-center justify-between p-3 rounded-xl border text-xs font-medium transition-all text-left",
                        selectedLocale === "en"
                          ? "bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold"
                          : "bg-white/[0.02] border-white/[0.08] text-zinc-300 hover:bg-white/[0.05]"
                      )}
                    >
                      <div className="flex flex-col">
                        <span>English</span>
                        <span className="text-[10px] text-zinc-500 font-mono">United States</span>
                      </div>
                      {selectedLocale === "en" && <Check size={15} className="text-emerald-400" />}
                    </button>
                  </div>
                </div>

                {/* Theme Selection */}
                <div className="space-y-2 pt-4 border-t border-white/[0.06]">
                  <label className="text-xs font-semibold text-zinc-200 flex items-center gap-2">
                    <Palette size={14} className="text-emerald-400" />
                    {locale === "zh-CN" ? "外观主题 / Theme Mode" : "Appearance Theme"}
                  </label>
                  <p className="text-[11px] text-zinc-400">
                    {locale === "zh-CN" ? "为控制台切换暗夜石墨色或明亮主题" : "Choose your favorite theme mode"}
                  </p>
                  <div className="grid grid-cols-3 gap-3 pt-1">
                    <button
                      type="button"
                      onClick={() => handleThemeChange("dark")}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border gap-2 text-xs font-medium transition-all",
                        currentTheme === "dark"
                          ? "bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold"
                          : "bg-white/[0.02] border-white/[0.08] text-zinc-400 hover:bg-white/[0.05]"
                      )}
                    >
                      <Moon size={16} />
                      <span>{locale === "zh-CN" ? "深色暗夜" : "Dark"}</span>
                    </button>

                    <button
                      type="button"
                      onClick={() => handleThemeChange("light")}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border gap-2 text-xs font-medium transition-all",
                        currentTheme === "light"
                          ? "bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold"
                          : "bg-white/[0.02] border-white/[0.08] text-zinc-400 hover:bg-white/[0.05]"
                      )}
                    >
                      <Sun size={16} />
                      <span>{locale === "zh-CN" ? "浅色明亮" : "Light"}</span>
                    </button>

                    <button
                      type="button"
                      onClick={() => handleThemeChange("system")}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border gap-2 text-xs font-medium transition-all",
                        currentTheme === "system"
                          ? "bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold"
                          : "bg-white/[0.02] border-white/[0.08] text-zinc-400 hover:bg-white/[0.05]"
                      )}
                    >
                      <Laptop size={16} />
                      <span>{locale === "zh-CN" ? "跟随系统" : "System"}</span>
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* TAB 2: SECURITY */}
            {activeTab === "security" && (
              <form onSubmit={handlePasswordSubmit} className="space-y-4">
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-zinc-200">
                    {locale === "zh-CN" ? "新密码" : "New Password"}
                  </label>
                  <input
                    type="password"
                    placeholder="••••••••••••"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    className="w-full px-3 py-2 rounded-lg bg-white/[0.03] border border-white/[0.1] text-xs text-white placeholder-zinc-500 focus:outline-none focus:border-emerald-500 transition-colors"
                  />
                  <p className="text-[10.5px] text-zinc-500">
                    {locale === "zh-CN" ? "建议长度不少于 8 位，包含字母与符号" : "Must be at least 8 characters"}
                  </p>
                </div>

                <div className="space-y-1">
                  <label className="text-xs font-semibold text-zinc-200">
                    {locale === "zh-CN" ? "确认新密码" : "Confirm New Password"}
                  </label>
                  <input
                    type="password"
                    placeholder="••••••••••••"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                    className="w-full px-3 py-2 rounded-lg bg-white/[0.03] border border-white/[0.1] text-xs text-white placeholder-zinc-500 focus:outline-none focus:border-emerald-500 transition-colors"
                  />
                </div>

                {errorMessage && (
                  <div className="p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-400 text-xs font-medium">
                    {errorMessage}
                  </div>
                )}

                {passwordStatus === "success" && (
                  <div className="p-2.5 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-medium flex items-center gap-2">
                    <Check size={14} />
                    {locale === "zh-CN" ? "密码已成功修改！" : "Password updated successfully!"}
                  </div>
                )}

                <div className="pt-3">
                  <Button
                    type="submit"
                    disabled={passwordStatus === "saving" || !password}
                    className="w-full bg-white text-zinc-950 hover:bg-zinc-100 font-semibold text-xs h-9"
                  >
                    <KeyRound size={14} />
                    {passwordStatus === "saving"
                      ? (locale === "zh-CN" ? "正在保存..." : "Updating...")
                      : (locale === "zh-CN" ? "更新本地密码" : "Update Password")}
                  </Button>
                </div>
              </form>
            )}

            {/* TAB 3: PROFILE */}
            {activeTab === "profile" && (
              <div className="space-y-4 text-xs">
                <div className="p-4 rounded-xl border border-white/[0.08] bg-white/[0.02] space-y-3">
                  <div className="flex justify-between items-center py-1 border-b border-white/[0.05]">
                    <span className="text-zinc-400">{locale === "zh-CN" ? "账号角色" : "Account Role"}</span>
                    <span className="font-semibold text-emerald-400">Owner / SuperAdmin</span>
                  </div>
                  <div className="flex justify-between items-center py-1 border-b border-white/[0.05]">
                    <span className="text-zinc-400">{locale === "zh-CN" ? "主邮箱" : "Primary Email"}</span>
                    <span className="font-mono text-zinc-200">admin@gamepanel.internal</span>
                  </div>
                  <div className="flex justify-between items-center py-1">
                    <span className="text-zinc-400">{locale === "zh-CN" ? "认证源" : "Auth Provider"}</span>
                    <span className="text-zinc-200">Local DB + Session Cookie</span>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
