"use client";

import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

type Locale = "zh" | "en";

const messages = {
  zh: {
    searchServers: "搜索服务器...",
    docker: "Docker",
    online: "在线",
    createServer: "创建服务器",
    navDashboard: "仪表盘",
    navServers: "服务器",
    navWorlds: "世界",
    navBackups: "备份",
    navMods: "模组",
    navActivity: "活动",
    navSettings: "设置",
    terrariaReady: "Terraria 就绪",
    settingsTitle: "设置",
    settingsDescription: "运行时和本地面板设置。",
    dockerRuntime: "Docker 运行时",
    dockerChecking: "正在检查 Docker 运行时...",
    dockerApiUnavailable: "API 不可用。请先启动 Go 后端后再检查 Docker。",
    available: "可用",
    unavailable: "不可用",
    dockerSockTitle: "Docker Socket / Host",
    dockerSockDescription:
      "后端会读取 GAMEPANEL_DOCKER_HOST。常见值：unix:///var/run/docker.sock、unix:///Users/你/.docker/run/docker.sock 或 tcp://127.0.0.1:2375。",
    configuredValue: "当前配置",
    dataDirectories: "数据目录",
    dataDirectoriesDescription: "每个服务器实例都会在配置的 GamePanel 数据根目录下使用独立数据目录。",
    language: "语言",
    chinese: "中文",
    english: "EN"
  },
  en: {
    searchServers: "Search servers...",
    docker: "Docker",
    online: "Online",
    createServer: "Create Server",
    navDashboard: "Dashboard",
    navServers: "Servers",
    navWorlds: "Worlds",
    navBackups: "Backups",
    navMods: "Mods",
    navActivity: "Activity",
    navSettings: "Settings",
    terrariaReady: "Terraria Ready",
    settingsTitle: "Settings",
    settingsDescription: "Runtime and local panel settings.",
    dockerRuntime: "Docker Runtime",
    dockerChecking: "Checking Docker runtime...",
    dockerApiUnavailable: "API unavailable. Start the Go backend to check Docker.",
    available: "Available",
    unavailable: "Unavailable",
    dockerSockTitle: "Docker Socket / Host",
    dockerSockDescription:
      "The backend reads GAMEPANEL_DOCKER_HOST. Common values: unix:///var/run/docker.sock, unix:///Users/you/.docker/run/docker.sock, or tcp://127.0.0.1:2375.",
    configuredValue: "Configured value",
    dataDirectories: "Data Directories",
    dataDirectoriesDescription:
      "Each server instance will use an isolated data directory under the configured GamePanel data root.",
    language: "Language",
    chinese: "中文",
    english: "EN"
  }
} as const;

type MessageKey = keyof typeof messages.zh;

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey) => string;
};

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>("zh");
  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      setLocale,
      t: (key) => messages[locale][key]
    }),
    [locale]
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error("useI18n must be used within I18nProvider");
  }
  return context;
}
