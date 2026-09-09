"use client";

import React, { Suspense, useState, type FormEvent } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Gamepad2,
  Building2,
  Shield,
  Check,
  AlertCircle,
  ArrowRight,
  Clock,
  LogIn,
  UserPlus,
  Sparkles,
  Loader2
} from "lucide-react";
import {
  getInvitationInfo,
  acceptInvitation,
  loginAdmin,
  registerUser
} from "@/lib/api";
import { useAuthBootstrap } from "@/lib/auth-session";
import { Button, Card, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export default function JoinPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center bg-panel-bg text-slate-400 text-xs">
          <Loader2 className="size-5 animate-spin text-panel-green mr-2" />
          <span>Loading invite...</span>
        </div>
      }
    >
      <JoinContent />
    </Suspense>
  );
}

function JoinContent() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();

  const token = searchParams.get("token") || "";

  const authQuery = useAuthBootstrap();
  const currentUser = authQuery.data?.account;
  const allowRegistration = authQuery.data?.allowRegistration ?? false;

  // Inline login/register state if user not authenticated
  const [authMode, setAuthMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [authError, setAuthError] = useState("");
  const [isAuthLoading, setIsAuthLoading] = useState(false);

  const [acceptSuccess, setAcceptSuccess] = useState(false);
  const [acceptError, setAcceptError] = useState<string | null>(null);

  // Query invitation info
  const inviteQuery = useQuery({
    queryKey: ["invitation-info", token],
    queryFn: () => getInvitationInfo(token),
    enabled: Boolean(token),
    retry: false
  });

  const invitation = inviteQuery.data;

  // Accept mutation
  const acceptMutation = useMutation({
    mutationFn: () => acceptInvitation(token),
    onSuccess: () => {
      setAcceptSuccess(true);
      setAcceptError(null);
      queryClient.invalidateQueries({ queryKey: ["organizations"] });
      queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
      setTimeout(() => {
        router.push("/servers");
      }, 1500);
    },
    onError: (err: Error) => {
      setAcceptError(err.message || "Failed to accept invitation");
    }
  });

  const handleAuthSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password) return;
    setAuthError("");
    setIsAuthLoading(true);
    try {
      if (authMode === "login") {
        await loginAdmin(username.trim(), password);
      } else {
        await registerUser(username.trim(), password);
      }
      await authQuery.refetch();
    } catch (err: unknown) {
      setAuthError(err instanceof Error ? err.message : "Authentication failed");
    } finally {
      setIsAuthLoading(false);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-panel-bg px-4 py-12 text-slate-100">
      <div className="w-full max-w-md space-y-4">
        {/* Brand Header */}
        <div className="flex items-center justify-center gap-2.5 text-center">
          <span className="flex size-9 items-center justify-center rounded-xl bg-panel-green text-slate-950 font-black shadow-lg shadow-panel-green/20">
            <Gamepad2 className="size-5" />
          </span>
          <span className="text-base font-black tracking-tight text-white">
            GamePanel <span className="text-panel-green">Lite</span>
          </span>
        </div>

        {/* Loading state */}
        {inviteQuery.isLoading && (
          <Card className="p-8 border-slate-800 bg-slate-900/90 text-center space-y-3">
            <Loader2 className="size-6 animate-spin text-panel-green mx-auto" />
            <p className="text-xs text-slate-400">
              {isZh ? "正在验证战队邀请口令..." : "Verifying team invitation..."}
            </p>
          </Card>
        )}

        {/* Invalid or expired invite */}
        {(inviteQuery.isError || (!inviteQuery.isLoading && !invitation)) && (
          <Card className="p-6 border-slate-800 bg-slate-900/90 shadow-2xl space-y-4 text-center">
            <div className="flex size-12 items-center justify-center rounded-2xl border border-red-500/30 bg-red-950/20 text-red-400 mx-auto">
              <AlertCircle className="size-6" />
            </div>
            <div>
              <h2 className="text-base font-bold text-white">
                {isZh ? "邀请链接无效或已失效" : "Invalid or Expired Invitation"}
              </h2>
              <p className="mt-1 text-xs text-slate-400 leading-relaxed">
                {isZh
                  ? "此战队邀请链接可能已被创建者作废、已超过使用人数上限，或已超过有效期限。请联系服主重新获取邀请链接。"
                  : "This invitation link is invalid, has reached its maximum uses, or has expired. Please contact the server owner for a new link."}
              </p>
            </div>
            <Button
              onClick={() => router.push("/dashboard")}
              className="w-full bg-slate-800 hover:bg-slate-700 text-xs text-white"
            >
              {isZh ? "返回控制台首页" : "Go to Dashboard"}
            </Button>
          </Card>
        )}

        {/* Valid Invite Details */}
        {invitation && (
          <Card className="overflow-hidden border-slate-800 bg-slate-900/90 shadow-2xl">
            <div className="border-b border-slate-800/80 bg-slate-950/40 p-6 text-center space-y-3">
              <div className="flex size-12 items-center justify-center rounded-2xl border border-panel-green/40 bg-panel-green/10 text-panel-green mx-auto">
                <Building2 className="size-6" />
              </div>
              <div>
                <span className="text-[11px] font-bold uppercase tracking-wider text-panel-green">
                  {isZh ? "战队邀请加入" : "Team Invitation"}
                </span>
                <h1 className="mt-1 text-lg font-bold text-white tracking-tight">
                  {invitation.organizationName}
                </h1>
                <p className="mt-1 text-xs text-slate-400">
                  {isZh
                    ? `由 ${invitation.inviterName || "服主"} 邀请你加入此工作区`
                    : `Invited by ${invitation.inviterName || "the host"}`}
                </p>
              </div>

              {/* Roles and limits pill */}
              <div className="flex flex-wrap items-center justify-center gap-2 pt-1">
                <span
                  className={cn(
                    "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-bold uppercase",
                    invitation.role === "member"
                      ? "bg-panel-green/15 text-panel-green border border-panel-green/30"
                      : "bg-sky-500/15 text-sky-400 border border-sky-500/30"
                  )}
                >
                  <Shield className="size-3" />
                  {invitation.role === "member"
                    ? (isZh ? "协同运维成员 (Member)" : "Member")
                    : (isZh ? "只读观察员 (Viewer)" : "Viewer")}
                </span>

                <span className="inline-flex items-center gap-1 rounded-full bg-slate-800/80 px-2.5 py-0.5 text-[11px] text-slate-400 font-mono">
                  <Clock className="size-3" />
                  {new Date(invitation.expiresAt).toLocaleDateString()} {isZh ? "前有效" : "Expires"}
                </span>
              </div>
            </div>

            <div className="p-6 space-y-4">
              {acceptSuccess ? (
                <div className="rounded-xl border border-panel-green/40 bg-panel-green/10 p-5 text-center space-y-2">
                  <div className="flex size-10 items-center justify-center rounded-full bg-panel-green text-slate-950 mx-auto font-bold">
                    <Check className="size-5" />
                  </div>
                  <h3 className="text-sm font-bold text-white">
                    {isZh ? "成功加入团队！" : "Successfully Joined Team!"}
                  </h3>
                  <p className="text-xs text-panel-green">
                    {isZh ? "正在跳转至服务器控制台..." : "Redirecting to server panel..."}
                  </p>
                </div>
              ) : currentUser ? (
                /* Authenticated State: One-click join */
                <div className="space-y-4">
                  <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3.5 flex items-center justify-between text-xs">
                    <div className="flex items-center gap-2.5">
                      <div className="flex size-7 items-center justify-center rounded-lg bg-panel-green/20 text-panel-green font-bold font-mono">
                        {currentUser.username.substring(0, 2).toUpperCase()}
                      </div>
                      <div>
                        <div className="text-slate-200 font-bold font-mono">
                          {currentUser.username}
                        </div>
                        <div className="text-[10px] text-slate-500 font-mono">
                          {isZh ? "当前登录身份: " : "Logged in as: "} {currentUser.role}
                        </div>
                      </div>
                    </div>
                    <span className="rounded bg-slate-800 px-2 py-0.5 text-[10px] text-slate-300 font-mono">
                      {isZh ? "已就绪" : "Ready"}
                    </span>
                  </div>

                  {acceptError && (
                    <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2 text-xs text-red-400">
                      <AlertCircle className="size-3.5 shrink-0" />
                      <span>{acceptError}</span>
                    </div>
                  )}

                  <Button
                    type="button"
                    disabled={acceptMutation.isPending}
                    onClick={() => acceptMutation.mutate()}
                    className="w-full h-10 bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90 text-xs shadow-lg shadow-panel-green/20"
                  >
                    {acceptMutation.isPending ? (
                      <>
                        <Loader2 className="mr-2 size-4 animate-spin" />
                        {isZh ? "正在加入团队..." : "Joining..."}
                      </>
                    ) : (
                      <>
                        <Sparkles className="mr-1.5 size-4" />
                        {isZh ? "接受邀请并加入团队" : "Accept Invitation & Join Team"}
                        <ArrowRight className="ml-1.5 size-4" />
                      </>
                    )}
                  </Button>
                </div>
              ) : (
                /* Unauthenticated State: Inline login or register */
                <div className="space-y-4">
                  <div className="flex items-center justify-between border-b border-slate-800 pb-2.5">
                    <span className="text-xs font-bold text-slate-300">
                      {isZh ? "请先登录以接受团队邀请" : "Sign in to accept invitation"}
                    </span>
                    <div className="flex items-center gap-1 text-[11px]">
                      <button
                        type="button"
                        onClick={() => {
                          setAuthMode("login");
                          setAuthError("");
                        }}
                        className={cn(
                          "rounded px-2 py-0.5 font-medium transition",
                          authMode === "login"
                            ? "bg-panel-green/15 text-panel-green font-bold"
                            : "text-slate-400 hover:text-white"
                        )}
                      >
                        {isZh ? "登录" : "Sign In"}
                      </button>
                      {allowRegistration && (
                        <button
                          type="button"
                          onClick={() => {
                            setAuthMode("register");
                            setAuthError("");
                          }}
                          className={cn(
                            "rounded px-2 py-0.5 font-medium transition",
                            authMode === "register"
                              ? "bg-panel-green/15 text-panel-green font-bold"
                              : "text-slate-400 hover:text-white"
                          )}
                        >
                          {isZh ? "注册" : "Register"}
                        </button>
                      )}
                    </div>
                  </div>

                  {authError && (
                    <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2 text-xs text-red-400">
                      <AlertCircle className="size-3.5 shrink-0" />
                      <span>{authError}</span>
                    </div>
                  )}

                  <form onSubmit={handleAuthSubmit} className="space-y-3 text-xs">
                    <div className="space-y-1">
                      <label className="text-[11px] text-slate-300">
                        {isZh ? "账号 / 用户名" : "Username"}
                      </label>
                      <Input
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        placeholder="username"
                        className="h-8.5 text-xs bg-slate-950 border-slate-800 font-mono"
                      />
                    </div>

                    <div className="space-y-1">
                      <label className="text-[11px] text-slate-300">
                        {isZh ? "登录密码" : "Password"}
                      </label>
                      <Input
                        type="password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        placeholder="••••••••"
                        className="h-8.5 text-xs bg-slate-950 border-slate-800"
                      />
                    </div>

                    <Button
                      type="submit"
                      disabled={isAuthLoading || !username.trim() || !password}
                      className="w-full h-9 bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90 text-xs mt-2"
                    >
                      {isAuthLoading ? (
                        <>
                          <Loader2 className="mr-2 size-3.5 animate-spin" />
                          {isZh ? "正在验证..." : "Signing in..."}
                        </>
                      ) : authMode === "login" ? (
                        <>
                          <LogIn className="mr-1.5 size-3.5" />
                          {isZh ? "登录并继续接受邀请" : "Sign In & Continue"}
                        </>
                      ) : (
                        <>
                          <UserPlus className="mr-1.5 size-3.5" />
                          {isZh ? "注册账号并加入团队" : "Register & Join Team"}
                        </>
                      )}
                    </Button>
                  </form>
                </div>
              )}
            </div>
          </Card>
        )}
      </div>
    </main>
  );
}
