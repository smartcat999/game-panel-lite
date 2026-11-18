import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "GamePanel Lite — 游戏服务器管理面板",
  description:
    "Kubernetes-style declarative multi-tenant game server orchestration platform",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" className="dark" data-theme="dark">
      <body>{children}</body>
    </html>
  );
}
