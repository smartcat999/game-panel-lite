"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Gamepad2, LockKeyhole, ShieldCheck, UserPlus } from "lucide-react";
import { usePathname } from "next/navigation";
import { Button, Card, Input } from "@/components/ui";
import { getAuthBootstrap, loginAdmin, registerUser, setupAdmin } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export function AuthGate({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/" || pathname.startsWith("/share/")) {
    return children;
  }
  return <ProtectedAuthGate>{children}</ProtectedAuthGate>;
}

function ProtectedAuthGate({ children }: { children: ReactNode }) {
  const { t, locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const authQuery = useQuery({ queryKey: ["auth-bootstrap"], queryFn: getAuthBootstrap, retry: false });

  const refreshAuth = async () => {
    await queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
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

  return children;
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
