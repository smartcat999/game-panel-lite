"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Gamepad2, LockKeyhole, ShieldCheck, UserPlus } from "lucide-react";
import { usePathname } from "next/navigation";
import { Button, Card, Input } from "@/components/ui";
import { getOAuthProviders, loginAdmin, registerUser, setupAdmin } from "@/lib/api";
import { AccountQueryScope } from "./account-query-scope";
import { accountCacheKey, useAuthBootstrap } from "@/lib/auth-session";
import { useI18n } from "@/lib/i18n";

export function AuthGate({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/") || pathname.startsWith("/join")) {
    return children;
  }
  return <ProtectedAuthGate>{children}</ProtectedAuthGate>;
}

function ProtectedAuthGate({ children }: { children: ReactNode }) {
  const { t, locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const authQuery = useAuthBootstrap();

  const refreshAuth = async () => {
    await authQuery.refetch();
    queryClient.getMutationCache().clear();
  };

  if (authQuery.isLoading) {
    return <AuthFrame title={isZh ? "正在验证控制台权限..." : t("authLoading")} description={isZh ? "正在连接管理中心服务" : t("authLoadingDescription")} />;
  }

  if (authQuery.isError) {
    return (
      <AuthFrame title={isZh ? "服务暂时不可用" : t("authApiUnavailable")} description={isZh ? "无法连接到后端 API 服务，请确认服务已启动" : t("authApiUnavailableDescription")}>
        <Button variant="secondary" onClick={() => authQuery.refetch()}>
          {isZh ? "重新尝试" : t("retry")}
        </Button>
      </AuthFrame>
    );
  }

  if (!authQuery.data?.initialized) {
    return <AuthForm initialMode="setup" allowRegistration={false} onSuccess={refreshAuth} />;
  }

  if (!authQuery.data.account) {
    return <AuthForm initialMode="login" allowRegistration={authQuery.data.allowRegistration} onSuccess={refreshAuth} />;
  }

  return <AccountQueryScope key={accountCacheKey(authQuery.data.account)}>{children}</AccountQueryScope>;
}

function AuthFrame({ title, description, children }: { title: string; description: string; children?: ReactNode }) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-panel-bg px-4 text-slate-100">
      <Card className="w-full max-w-md p-6 border-slate-800 bg-slate-950/80 shadow-2xl">
        <div className="flex items-start gap-4">
          <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-panel-green text-slate-950 shadow-md">
            <Gamepad2 className="size-6" />
          </span>
          <div>
            <p className="text-xs font-bold uppercase tracking-wider text-panel-green">GamePanel Lite</p>
            <h1 className="mt-1 text-xl font-bold text-white tracking-tight">{title}</h1>
            <p className="mt-1.5 text-xs leading-relaxed text-slate-400">{description}</p>
            {children ? <div className="mt-5">{children}</div> : null}
          </div>
        </div>
      </Card>
    </main>
  );
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

  const { data: oauthProviders } = useQuery({
    queryKey: ["oauth-providers"],
    queryFn: getOAuthProviders,
    staleTime: 60000,
    enabled: !isSetup,
  });

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
    <main className="flex min-h-screen items-center justify-center bg-panel-bg px-4 text-slate-100">
      <Card className="w-full max-w-md p-6 sm:p-8 border-slate-800 bg-slate-950/80 shadow-2xl rounded-2xl">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-xl bg-panel-green text-slate-950 shadow-md">
            {isSetup ? <ShieldCheck className="size-6" /> : isRegister ? <UserPlus className="size-6" /> : <LockKeyhole className="size-6" />}
          </span>
          <div>
            <p className="text-xs font-bold uppercase tracking-wider text-panel-green">GamePanel Lite</p>
            <h1 className="text-xl font-bold text-white tracking-tight">
              {isSetup
                ? isZh ? "初始化超级管理员" : "Setup Administrator"
                : isRegister
                ? isZh ? "注册开黑账号" : "Register Account"
                : isZh ? "登录控制台" : "Sign In"}
            </h1>
          </div>
        </div>

        <p className="mt-3 text-xs leading-relaxed text-slate-400">
          {isSetup
            ? isZh ? "欢迎使用 GamePanel Lite！请创建首个系统超级管理员账号。" : "Welcome! Create the initial administrator account."
            : isRegister
            ? isZh ? "创建你的开黑成员账号，即可加入服务器管理。" : "Create your member account to access servers."
            : isZh ? "请输入账号密码进入游戏服务器管理控制台。" : "Enter credentials to access game server management."}
        </p>

        <form className="mt-6 space-y-4" onSubmit={submit}>
          <label className="block">
            <span className="text-xs font-medium text-slate-300">{isZh ? "用户名" : "Username"}</span>
            <Input
              required
              className="mt-1.5 w-full bg-slate-900 border-slate-800 focus:border-panel-green text-xs"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder={isZh ? "3-32位英文/数字/下划线" : "Username"}
              autoComplete="username"
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-300">{isZh ? "密码" : "Password"}</span>
            <Input
              required
              className="mt-1.5 w-full bg-slate-900 border-slate-800 focus:border-panel-green text-xs"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder={isZh ? "至少8位密码" : "At least 8 characters"}
              autoComplete={isSetup || isRegister ? "new-password" : "current-password"}
            />
          </label>

          {error ? (
            <p className="rounded-lg border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-200 font-medium">
              {error}
            </p>
          ) : null}

          <Button className="h-10 w-full text-xs font-bold mt-2" type="submit" disabled={mutation.isPending || !username || !password}>
            {mutation.isPending
              ? isZh ? "正在提交..." : "Submitting..."
              : isSetup
              ? isZh ? "完成初始化并进入面板" : "Complete Setup"
              : isRegister
              ? isZh ? "立即注册并登录" : "Register & Sign In"
              : isZh ? "立即登录" : "Sign In"}
          </Button>

          {/* Third-Party OAuth Social Logins */}
          {!isSetup && (oauthProviders?.github || oauthProviders?.google) && (
            <div className="pt-3">
              <div className="relative my-3">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-slate-800" />
                </div>
                <div className="relative flex justify-center text-[10px] uppercase">
                  <span className="bg-slate-950 px-2 text-slate-500 font-semibold">
                    {isZh ? "或者使用第三方账号快捷登录" : "Or continue with"}
                  </span>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-2">
                {oauthProviders.github && (
                  <a
                    href="/api/auth/oauth/github/authorize"
                    className="flex items-center justify-center gap-2.5 h-9 rounded-lg border border-slate-800 bg-slate-900/90 text-xs font-medium text-slate-200 hover:border-slate-700 hover:bg-slate-800 transition"
                  >
                    <svg className="size-4 fill-current text-white" viewBox="0 0 24 24">
                      <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
                    </svg>
                    <span>{isZh ? "使用 GitHub 账号登录" : "Continue with GitHub"}</span>
                  </a>
                )}

                {oauthProviders.google && (
                  <a
                    href="/api/auth/oauth/google/authorize"
                    className="flex items-center justify-center gap-2.5 h-9 rounded-lg border border-slate-800 bg-slate-900/90 text-xs font-medium text-slate-200 hover:border-slate-700 hover:bg-slate-800 transition"
                  >
                    <svg className="size-4" viewBox="0 0 24 24">
                      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z" />
                      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z" />
                    </svg>
                    <span>{isZh ? "使用 Google 账号登录" : "Continue with Google"}</span>
                  </a>
                )}
              </div>
            </div>
          )}

          {/* Toggle between Login and Register if enabled */}
          {!isSetup && allowRegistration && (
            <div className="pt-2 text-center border-t border-slate-800/80">
              {isRegister ? (
                <p className="text-xs text-slate-400">
                  {isZh ? "已有账号？" : "Already have an account?"}{" "}
                  <button
                    type="button"
                    onClick={() => {
                      setMode("login");
                      setError("");
                    }}
                    className="font-bold text-panel-green hover:underline ml-1"
                  >
                    {isZh ? "直接登录" : "Sign In"}
                  </button>
                </p>
              ) : (
                <p className="text-xs text-slate-400">
                  {isZh ? "还没有账号？" : "Don't have an account?"}{" "}
                  <button
                    type="button"
                    onClick={() => {
                      setMode("register");
                      setError("");
                    }}
                    className="font-bold text-panel-green hover:underline ml-1"
                  >
                    {isZh ? "注册新账号" : "Register"}
                  </button>
                </p>
              )}
            </div>
          )}
        </form>
      </Card>
    </main>
  );
}
