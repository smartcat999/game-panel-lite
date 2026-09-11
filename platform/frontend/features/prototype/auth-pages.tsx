"use client";

import { ArrowLeft, ArrowRight, Check, Cloud, Github, KeyRound, Lock, ShieldCheck, Sparkles, User } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { api, type Workspace } from "@/lib/api";
import { usePrototype } from "@/lib/prototype-store";

export function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [mustChange, setMustChange] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const enterWorkspace = async () => {
    const workspaces = await api<Workspace[]>("/workspaces");
    router.push(workspaces[0] ? `/w/${workspaces[0].slug}/instances` : "/account");
  };

  const signIn = async () => {
    if (!username.trim()) {
      setError("请输入账号或用户名");
      return;
    }
    if (!password) {
      setError("请输入密码");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await api<{ mustChangePassword: boolean }>("/auth/password/sign-in", {
        method: "POST",
        body: JSON.stringify({ username: username.trim(), password }),
      });
      if (result.mustChangePassword) {
        setMustChange(true);
      } else {
        await enterWorkspace();
      }
    } catch {
      setError("用户名或密码不正确，请重新检查");
    } finally {
      setBusy(false);
    }
  };

  const changePassword = async () => {
    setBusy(true);
    setError("");
    try {
      await api<void>("/auth/password", {
        method: "PUT",
        body: JSON.stringify({ password: newPassword }),
      });
      await enterWorkspace();
    } catch {
      setError("新密码至少需 12 位，请重新设置");
    } finally {
      setBusy(false);
    }
  };

  const github = async () => {
    setBusy(true);
    setError("");
    try {
      const result = await api<{ authorizationUrl: string }>("/auth/github/start", {
        method: "POST",
        body: JSON.stringify({ returnPath: "/w/ember/instances" }),
      });
      window.location.assign(result.authorizationUrl);
    } catch {
      setError("GitHub 快捷登录尚未开启，请使用平台账号登录");
      setBusy(false);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-[#080c14] p-4 sm:p-6">
      {/* Background glow */}
      <div className="pointer-events-none fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 size-[450px] rounded-full bg-emerald-500/10 blur-[130px]" />

      <div className="relative w-full max-w-[420px]">
        {/* Back link */}
        <div className="mb-4">
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 text-xs text-zinc-400 transition hover:text-emerald-400"
          >
            <ArrowLeft className="size-3.5" />
            <span>返回云平台官网</span>
          </Link>
        </div>

        {/* Card */}
        <section className="overflow-hidden rounded-2xl border border-white/[0.1] bg-[#0e1420] p-7 shadow-2xl shadow-black/80">
          {/* Header & Brand */}
          <div className="flex items-center gap-3 border-b border-white/[0.08] pb-5">
            <div className="flex size-10 items-center justify-center rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-1.5 shadow-sm shadow-emerald-500/20">
              <Image
                alt="GamePanel Cloud"
                height={36}
                src="/avatar.svg"
                width={36}
                className="pixelated object-contain"
              />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-white tracking-tight">GamePanel Cloud</span>
                <span className="rounded bg-emerald-500/20 px-1.5 py-0.5 text-[9px] font-semibold text-emerald-400">
                  SaaS PLATFORM
                </span>
              </div>
              <p className="text-xs text-zinc-400">游戏服务器云端托管控制台</p>
            </div>
          </div>

          <div className="mt-6 mb-5">
            <h1 className="text-lg font-bold text-white tracking-tight">
              {mustChange ? "初次登录安全重置" : "登录云控制台"}
            </h1>
            <p className="mt-1 text-xs text-zinc-400">
              {mustChange
                ? "请为账号设置高强度密码以保护工作区"
                : "输入凭证进入您的小队工作区管理游戏云服"}
            </p>
          </div>

          {mustChange ? (
            <form
              className="flex flex-col gap-4"
              onSubmit={(event) => {
                event.preventDefault();
                void changePassword();
              }}
            >
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-semibold text-zinc-300">设置新密码</label>
                <div className="relative">
                  <Lock className="absolute left-3 top-3 size-4 text-zinc-500" />
                  <input
                    autoFocus
                    minLength={12}
                    placeholder="至少 12 位字符"
                    onChange={(event) => setNewPassword(event.target.value)}
                    type="password"
                    value={newPassword}
                    className="h-10 w-full rounded-lg border border-white/[0.12] bg-[#080c14] pl-9 pr-3 text-sm text-zinc-100 placeholder:text-zinc-600 focus:border-emerald-500 focus:outline-none"
                  />
                </div>
              </div>

              <Button
                disabled={busy}
                type="submit"
                className="h-10 w-full bg-emerald-500 font-bold text-zinc-950 hover:bg-emerald-400"
              >
                {busy ? "正在保存..." : "保存新密码并进入控制台"}
                <ArrowRight className="size-4 ml-1" />
              </Button>
            </form>
          ) : (
            <form
              className="flex flex-col gap-4"
              onSubmit={(event) => {
                event.preventDefault();
                void signIn();
              }}
            >
              {/* Username Field */}
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-semibold text-zinc-300">账号 / 邮箱 / 用户名</label>
                <div className="relative">
                  <User className="absolute left-3 top-3 size-4 text-zinc-500" />
                  <input
                    autoComplete="username"
                    placeholder="请输入账号 (默认 admin)"
                    onChange={(event) => setUsername(event.target.value)}
                    value={username}
                    className="h-10 w-full rounded-lg border border-white/[0.12] bg-[#080c14] pl-9 pr-3 text-sm text-zinc-100 placeholder:text-zinc-600 focus:border-emerald-500 focus:outline-none"
                  />
                </div>
              </div>

              {/* Password Field */}
              <div className="flex flex-col gap-1.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-semibold text-zinc-300">登录密码</label>
                  <span className="text-[11px] text-zinc-500">平台安全验证</span>
                </div>
                <div className="relative">
                  <KeyRound className="absolute left-3 top-3 size-4 text-zinc-500" />
                  <input
                    autoComplete="current-password"
                    type="password"
                    placeholder="••••••••"
                    onChange={(event) => setPassword(event.target.value)}
                    value={password}
                    className="h-10 w-full rounded-lg border border-white/[0.12] bg-[#080c14] pl-9 pr-3 text-sm text-zinc-100 placeholder:text-zinc-600 focus:border-emerald-500 focus:outline-none"
                  />
                </div>
              </div>

              {/* Primary Submit Button */}
              <Button
                disabled={busy}
                type="submit"
                className="mt-1 h-10 w-full bg-emerald-500 font-bold text-zinc-950 shadow-md shadow-emerald-500/25 hover:bg-emerald-400 active:scale-[0.98]"
              >
                {busy ? "正在验证凭据..." : "登录云控制台"}
                <ArrowRight className="size-4 ml-1" />
              </Button>
            </form>
          )}

          {/* Error notification */}
          {error && (
            <div className="mt-4 rounded-lg border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
              {error}
            </div>
          )}

          {/* Secondary Divider & GitHub OAuth */}
          {!mustChange && (
            <>
              <div className="relative my-5 flex items-center justify-center">
                <div className="w-full border-t border-white/[0.08]" />
                <span className="absolute bg-[#0e1420] px-3 text-[11px] font-medium text-zinc-500">
                  其他登录方式
                </span>
              </div>

              <Button
                type="button"
                variant="secondary"
                disabled={busy}
                onClick={github}
                className="h-10 w-full border border-white/[0.1] bg-[#0a0e17] text-zinc-300 hover:border-white/[0.2] hover:bg-[#121927] hover:text-white"
              >
                <Github className="size-4 mr-1.5" />
                使用 GitHub 快捷登录
              </Button>
            </>
          )}

          {/* Platform Note */}
          <div className="mt-6 flex items-center justify-center gap-1.5 border-t border-white/[0.06] pt-4 text-center text-[11px] text-zinc-500">
            <Cloud className="size-3.5 text-emerald-400" />
            <span>GamePanel Cloud · 高可用多可用区游戏托管平台</span>
          </div>
        </section>
      </div>
    </main>
  );
}

export function InvitationPage() {
  const { acceptInvitation, invited } = usePrototype();
  return (
    <main className="flex min-h-screen items-center justify-center bg-[#080c14] p-4 sm:p-6">
      <section className="w-full max-w-[400px] overflow-hidden rounded-2xl border border-white/[0.1] bg-[#0e1420] p-7 text-center shadow-2xl shadow-black/80">
        <div className="flex items-center gap-3 border-b border-white/[0.08] pb-5 text-left">
          <Image alt="GamePanel Cloud" height={32} src="/avatar.svg" width={32} className="pixelated" />
          <strong className="text-white font-bold">GamePanel Cloud</strong>
        </div>
        <div className="mx-auto mt-6 flex size-12 items-center justify-center rounded-full border border-emerald-500/30 bg-emerald-500/10 text-emerald-400">
          <Check size={24} />
        </div>
        <div className="mt-4 mb-5">
          <h1 className="text-lg font-bold text-white">{invited ? "已加入工作区" : "工作区邀请"}</h1>
          <p className="text-xs text-zinc-400 mt-1">管理员邀请你加入 Ember Realms 游戏服务器工作区</p>
        </div>
        <dl className="mb-5 overflow-hidden rounded-lg border border-white/[0.08] bg-[#080c14] text-left text-xs">
          <div className="flex justify-between border-b border-white/[0.08] p-3">
            <dt className="text-zinc-500">工作区角色</dt>
            <dd className="font-semibold text-zinc-200">小队运维员</dd>
          </div>
          <div className="flex justify-between p-3">
            <dt className="text-zinc-500">登录账号</dt>
            <dd className="font-semibold text-zinc-200">GitHub · pengwu</dd>
          </div>
        </dl>
        <Button
          asChild
          onClick={acceptInvitation}
          className="h-10 w-full bg-emerald-500 font-bold text-zinc-950 hover:bg-emerald-400"
        >
          <Link href="/w/ember/instances">
            {invited ? "进入工作区" : "接受邀请并进入"}
            <ArrowRight size={16} className="ml-1" />
          </Link>
        </Button>
      </section>
    </main>
  );
}
