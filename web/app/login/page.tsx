"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import LogoMark from "../components/landing/LogoMark";
import VoxelIsland from "../components/landing/VoxelIsland";
import { localizeApiError, saveSession } from "../console/lib/session";

const focusRing =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#2B3AE8] focus-visible:ring-offset-2 focus-visible:ring-offset-white";

const inputClass =
  "block w-full rounded-full border border-[#D5DAEE] bg-white px-5 py-3.5 text-[15px] text-[#14161F] placeholder:text-[#7A7F95] transition-[border-color,box-shadow] hover:border-[#B8C0E6] focus:border-[#2B3AE8] focus:outline-none focus:ring-4 focus:ring-[#2B3AE8]/15";

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await fetch("/apis/iam.gamepanel.io/v1/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          apiVersion: "iam.gamepanel.io/v1",
          kind: "Session",
          subjectKind: "User",
          identifier: username.trim(),
          username: username.trim(),
          password,
        }),
      });

      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        const raw =
          body?.message ||
          body?.error ||
          `登录失败 (${res.status})，请检查用户名和密码`;
        setError(localizeApiError(String(raw), res.status));
        return;
      }

      const token = body?.token;
      if (!token) {
        setError("登录失败：服务端没有返回会话凭证，请稍后重试");
        return;
      }

      saveSession({
        token,
        sessionName: typeof body?.metadata?.name === "string" ? body.metadata.name : undefined,
        userName:
          body?.user?.metadata?.name ||
          (typeof body?.user === "string" ? body.user : "") ||
          username.trim(),
      });
      // Only allow same-origin relative redirects (e.g. /console?to=new&game=...).
      const next = new URLSearchParams(window.location.search).get("next");
      router.replace(next && next.startsWith("/") && !next.startsWith("//") ? next : "/console");
    } catch (err) {
      setError(
        err instanceof Error
          ? localizeApiError(err.message)
          : "网络连接失败，请检查网络后重试",
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="relative min-h-[100dvh] overflow-hidden bg-[linear-gradient(180deg,#C9D4FF_0%,#DCE3FF_50%,#F5F7FD_100%)] text-[#14161F] antialiased">
      {/* nav — same floating pill as the homepage */}
      <header className="absolute inset-x-0 top-4 z-20 px-4">
        <div className="mx-auto flex h-14 max-w-[1240px] items-center justify-between gap-2 rounded-full border border-white/70 bg-white/70 pl-3.5 pr-2 shadow-[0_8px_30px_-12px_rgba(43,58,232,0.25)] backdrop-blur-xl sm:pl-4">
          <Link
            href="/"
            className={`flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full text-[15px] font-semibold tracking-tight sm:gap-2.5 ${focusRing}`}
          >
            <LogoMark />
            <span>GamePanel Lite</span>
          </Link>
          <Link
            href="/"
            className={`shrink-0 whitespace-nowrap rounded-full px-4 py-2 text-sm font-medium text-[#3D4258] transition-colors hover:bg-white hover:text-[#14161F] sm:px-5 ${focusRing}`}
          >
            返回首页
          </Link>
        </div>
      </header>

      <main className="mx-auto grid min-h-[100dvh] max-w-[1240px] grid-cols-1 items-center gap-8 px-6 pb-10 pt-28 lg:grid-cols-2">
        {/* island — hidden on small screens so the form stays above the fold */}
        <div className="relative hidden h-[620px] lg:block [mask-image:linear-gradient(to_bottom,#000_80%,transparent)]">
          <VoxelIsland labels={[]} />
        </div>

        <div className="mx-auto w-full max-w-[420px]">
          <div className="rounded-[28px] border border-white/80 bg-white/80 p-8 shadow-[0_30px_60px_-30px_rgba(43,58,232,0.45)] backdrop-blur-xl sm:p-10">
            <h1 className="text-[32px] font-bold leading-tight tracking-tight">欢迎回来</h1>
            <p className="mt-2 text-[15px] text-[#5B6075]">登录后管理你的游戏服务器。</p>

            <form onSubmit={handleSubmit} className="mt-8 space-y-5">
              <div className="space-y-2">
                <label htmlFor="username" className="block pl-1 text-sm font-medium text-[#3D4258]">
                  用户名或邮箱
                </label>
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className={inputClass}
                  placeholder="输入用户名或邮箱"
                  required
                  autoComplete="username"
                />
              </div>

              <div className="space-y-2">
                <label htmlFor="password" className="block pl-1 text-sm font-medium text-[#3D4258]">
                  密码
                </label>
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className={inputClass}
                  placeholder="输入密码"
                  required
                  autoComplete="current-password"
                  aria-invalid={error ? true : undefined}
                  aria-describedby={error ? "login-error" : undefined}
                />
              </div>

              {error && (
                <p
                  id="login-error"
                  role="alert"
                  className="rounded-2xl bg-[#FDECEC] px-4 py-3 text-sm leading-relaxed text-[#B42318]"
                >
                  {error}
                </p>
              )}

              <button
                type="submit"
                disabled={loading}
                className={`mt-2 w-full rounded-full bg-[#2B3AE8] px-6 py-4 text-base font-semibold text-white shadow-[0_14px_30px_-12px_rgba(43,58,232,0.8)] transition-[transform,background-color,opacity] hover:-translate-y-0.5 hover:bg-[#2230D0] active:translate-y-0 active:scale-[0.98] disabled:translate-y-0 disabled:cursor-wait disabled:opacity-70 ${focusRing}`}
              >
                {loading ? "正在登录…" : "登录"}
              </button>
            </form>
          </div>
        </div>
      </main>
    </div>
  );
}
