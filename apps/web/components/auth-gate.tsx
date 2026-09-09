"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Server } from "lucide-react";
import { usePathname } from "next/navigation";
import { Button, Card, Input } from "@/components/ui";
import { loginAdmin, registerUser, setupAdmin } from "@/lib/api";
import { useAuthBootstrap } from "@/lib/auth-session";
import { useI18n } from "@/lib/i18n";

export function AuthGate({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/") || pathname.startsWith("/join")) {
    return <>{children}</>;
  }
  return <ProtectedAuthGate>{children}</ProtectedAuthGate>;
}

function ProtectedAuthGate({ children }: { children: ReactNode }) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const authQuery = useAuthBootstrap();

  const refreshAuth = async () => {
    await authQuery.refetch();
    queryClient.getMutationCache().clear();
  };

  if (authQuery.isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
        <div className="flex flex-col items-center gap-3 text-center">
          <div className="flex size-10 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 animate-pulse">
            <Server className="size-5" />
          </div>
          <p className="text-xs font-semibold text-slate-700">
            {isZh ? "正在验证控制台权限..." : "Verifying session..."}
          </p>
        </div>
      </main>
    );
  }

  if (authQuery.isError) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
        <Card className="w-full max-w-sm p-6 text-center space-y-3">
          <h2 className="text-sm font-bold text-slate-900">
            {isZh ? "后端服务未连接" : "Backend Unavailable"}
          </h2>
          <p className="text-xs text-slate-500">
            {isZh ? "无法连接到 API 服务，请确认 Go 后端已启动。" : "Cannot connect to API server. Please make sure the Go service is running."}
          </p>
          <Button variant="secondary" onClick={() => authQuery.refetch()} className="w-full">
            {isZh ? "重试连接" : "Retry"}
          </Button>
        </Card>
      </main>
    );
  }

  if (!authQuery.data?.initialized) {
    return <AuthForm initialMode="setup" allowRegistration={false} onSuccess={refreshAuth} />;
  }

  if (!authQuery.data.account) {
    return <AuthForm initialMode="login" allowRegistration={authQuery.data.allowRegistration} onSuccess={refreshAuth} />;
  }

  return <>{children}</>;
}

function AuthForm({
  initialMode,
  allowRegistration,
  onSuccess
}: {
  initialMode: "setup" | "login" | "register";
  allowRegistration: boolean;
  onSuccess: () => Promise<void>;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const [mode, setMode] = useState<"setup" | "login" | "register">(initialMode);
  const [username, setUsername] = useState(mode === "setup" ? "admin" : "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  const isSetup = mode === "setup";
  const isRegister = mode === "register";

  const mutation = useMutation({
    mutationFn: () => {
      if (isSetup) return setupAdmin(username, password);
      if (isRegister) return registerUser(username, password);
      return loginAdmin(username, password);
    },
    onSuccess: async () => {
      setError("");
      await onSuccess();
    },
    onError: (err) => setError(err instanceof Error ? err.message : isZh ? "认证失败，请检查账号密码" : "Authentication failed")
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    mutation.mutate();
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4 text-slate-900">
      <Card className="w-full max-w-sm p-6 space-y-4">
        <div className="flex items-center gap-2.5">
          <div className="flex size-9 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 font-bold border border-emerald-500/20">
            GP
          </div>
          <div>
            <h1 className="text-sm font-bold text-slate-900 leading-tight">
              {isSetup ? (isZh ? "初始化超级管理员" : "Setup Administrator") : isRegister ? (isZh ? "注册成员账号" : "Register Account") : (isZh ? "登录控制台" : "Sign In")}
            </h1>
            <p className="text-[11px] text-slate-400">
              {isSetup ? (isZh ? "GamePanel Lite 首次使用设置" : "First-time panel setup") : (isZh ? "游戏服务器轻量管控控制台" : "Game server management")}
            </p>
          </div>
        </div>

        <form onSubmit={submit} className="space-y-3 pt-1">
          <div>
            <label className="block text-[11px] font-semibold text-slate-700 mb-1">
              {isZh ? "用户名" : "Username"}
            </label>
            <Input
              required
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={isZh ? "管理员用户名" : "Username"}
              autoComplete="username"
            />
          </div>

          <div>
            <label className="block text-[11px] font-semibold text-slate-700 mb-1">
              {isZh ? "密码" : "Password"}
            </label>
            <Input
              required
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={isZh ? "至少 8 位密码" : "At least 8 characters"}
              autoComplete={isSetup || isRegister ? "new-password" : "current-password"}
            />
          </div>

          {error && (
            <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700 font-medium">
              {error}
            </p>
          )}

          <Button type="submit" disabled={mutation.isPending || !username || !password} className="w-full h-8 mt-1">
            {mutation.isPending
              ? (isZh ? "提交中..." : "Submitting...")
              : isSetup
              ? (isZh ? "创建超级管理员" : "Create Admin")
              : isRegister
              ? (isZh ? "注册账号" : "Register")
              : (isZh ? "立即登录" : "Sign In")}
          </Button>

          {!isSetup && allowRegistration && (
            <div className="pt-2 text-center text-xs text-slate-500">
              {isRegister ? (
                <span>
                  {isZh ? "已有账号？" : "Already have an account?"}{" "}
                  <button type="button" onClick={() => setMode("login")} className="font-semibold text-emerald-600 hover:underline">
                    {isZh ? "直接登录" : "Sign In"}
                  </button>
                </span>
              ) : (
                <span>
                  {isZh ? "没有账号？" : "No account?"}{" "}
                  <button type="button" onClick={() => setMode("register")} className="font-semibold text-emerald-600 hover:underline">
                    {isZh ? "注册新账号" : "Register"}
                  </button>
                </span>
              )}
            </div>
          )}
        </form>
      </Card>
    </main>
  );
}
