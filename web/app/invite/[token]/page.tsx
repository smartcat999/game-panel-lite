"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import LogoMark from "../../components/landing/LogoMark";
import VoxelIsland from "../../components/landing/VoxelIsland";
import { Badge, Segmented } from "../../components/console/ui";
import {
  clearSession,
  localizeApiError,
  readToken,
  saveSession,
  TENANT_STORAGE_KEY,
  USER_STORAGE_KEY,
} from "../../console/lib/session";

const focusRing =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#2B3AE8] focus-visible:ring-offset-2 focus-visible:ring-offset-white";

const inputBaseClass =
  "block w-full rounded-full border bg-white px-5 py-3.5 text-[15px] text-[#14161F] placeholder:text-[#7A7F95] transition-[border-color,box-shadow] focus:outline-none focus:ring-4";
const inputClass = `${inputBaseClass} border-[#D5DAEE] hover:border-[#B8C0E6] focus:border-[#2B3AE8] focus:ring-[#2B3AE8]/15`;
const inputErrorClass = `${inputBaseClass} border-[#F04438] hover:border-[#F04438] focus:border-[#D92D20] focus:ring-[#D92D20]/15`;

const ROLE_META: Record<string, { label: string; desc: string }> = {
  "instance-operator": { label: "运维", desc: "可启停服务器、使用控制台和修改配置" },
  "tenant-viewer": { label: "只读", desc: "可查看服务器状态与连接地址" },
  "tenant-admin": { label: "管理员", desc: "拥有工作区完整管理权限" },
};

const primaryClass = `inline-flex h-12 w-full items-center justify-center rounded-full bg-[#2B3AE8] px-6 text-[15px] font-semibold text-white shadow-[0_14px_30px_-10px_rgba(43,58,232,0.7)] transition hover:bg-[#2330C8] disabled:opacity-60 ${focusRing}`;

const USERNAME_RE = /^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$/;
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function validateRegistration(username: string, email: string, password: string, confirm: string): string | null {
  if (!EMAIL_RE.test(email)) return "请输入有效的邮箱地址";
  if (!USERNAME_RE.test(username) || username.includes("..")) {
    return "用户名只能包含字母、数字和 . _ -，且以字母或数字开头结尾";
  }
  if (password.length < 8) return "密码至少 8 位";
  if (password !== confirm) return "两次输入的密码不一致";
  return null;
}

interface InviteInfo {
  metadata: { name: string; tenant: string };
  tenantDisplayName?: string;
  inviter?: string;
  role?: string;
  serverName?: string;
  expiresAt?: string;
}

function decodeTenantFromToken(rawToken: string): string | null {
  try {
    const payload = rawToken.split(".")[0];
    if (!payload) return null;
    const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64 + "===".slice((base64.length + 3) % 4);
    const parsed = JSON.parse(atob(padded)) as { t?: string };
    return parsed.t || null;
  } catch {
    return null;
  }
}

export default function InviteAcceptPage() {
  const params = useParams<{ token: string }>();
  const router = useRouter();
  const rawToken = useMemo(() => {
    try {
      return decodeURIComponent(params?.token || "");
    } catch {
      return params?.token || "";
    }
  }, [params?.token]);

  const tenantName = useMemo(() => decodeTenantFromToken(rawToken), [rawToken]);

  const [invite, setInvite] = useState<InviteInfo | null>(null);
  const [loadingInfo, setLoadingInfo] = useState<boolean>(true);
  const [infoError, setInfoError] = useState<string | null>(null);

  const [existingToken, setExistingToken] = useState<string | null>(null);
  const [existingUser, setExistingUser] = useState<string>("");
  const [useDifferentAccount, setUseDifferentAccount] = useState<boolean>(false);

  const [username, setUsername] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [mode, setMode] = useState<"register" | "login">("register");
  const [confirmPassword, setConfirmPassword] = useState<string>("");
  const [email, setEmail] = useState<string>("");
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  useEffect(() => {
    const t = readToken();
    const u = typeof window !== "undefined" ? localStorage.getItem(USER_STORAGE_KEY) || "" : "";
    setExistingToken(t);
    setExistingUser(u);
  }, []);

  useEffect(() => {
    if (!rawToken || !tenantName) {
      setInfoError("邀请链接格式无效或已损坏");
      setLoadingInfo(false);
      return;
    }
    let cancelled = false;
    (async () => {
      setLoadingInfo(true);
      setInfoError(null);
      try {
        const res = await fetch(
          `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenantName)}/invites/${encodeURIComponent(rawToken)}`,
        );
        const body = await res.json().catch(() => ({}));
        if (!res.ok) {
          throw new Error(localizeApiError(String(body?.message || ""), res.status, "邀请链接已失效或过期"));
        }
        if (!cancelled) {
          setInvite(body as InviteInfo);
        }
      } catch (err) {
        if (!cancelled) {
          setInfoError(err instanceof Error ? err.message : "加载邀请信息失败");
        }
      } finally {
        if (!cancelled) setLoadingInfo(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [rawToken, tenantName]);

  const roleMeta = ROLE_META[invite?.role || "instance-operator"] ?? { label: invite?.role || "成员", desc: "" };
  const passwordTooShort = mode === "register" && password.length > 0 && password.length < 8;
  const passwordMismatch = mode === "register" && confirmPassword.length > 0 && password !== confirmPassword;

  async function handleAccept(e?: FormEvent) {
    e?.preventDefault();
    if (!tenantName || !rawToken) return;
    if (!(existingToken && !useDifferentAccount) && mode === "register") {
      const err = validateRegistration(username.trim(), email.trim(), password, confirmPassword);
      if (err) {
        setSubmitError(err);
        return;
      }
    }
    setSubmitting(true);
    setSubmitError(null);
    try {
      const useCurrentSession = existingToken && !useDifferentAccount;
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (useCurrentSession) {
        headers.Authorization = `Bearer ${existingToken}`;
      }
      const res = await fetch(
        `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenantName)}/invites/${encodeURIComponent(rawToken)}`,
        {
          method: "POST",
          headers,
          body: JSON.stringify(
            useCurrentSession
              ? {}
              : {
                  username: username.trim(),
                  password,
                  mode: mode === "register" ? "Register" : "Login",
                  ...(mode === "register" ? { email: email.trim() } : {}),
                },
          ),
        },
      );
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(localizeApiError(String(body?.message || ""), res.status, "接受邀请失败"));
      }
      if (body?.sessionToken) {
        saveSession({
          token: String(body.sessionToken),
          userName: body?.username ? String(body.username) : undefined,
        });
      } else if (body?.username) {
        localStorage.setItem(USER_STORAGE_KEY, String(body.username));
      }
      localStorage.setItem(TENANT_STORAGE_KEY, tenantName);
      const targetServer = body?.serverName || invite?.serverName;
      const tenantQuery = `?tenant=${encodeURIComponent(tenantName)}`;
      router.replace(
        targetServer ? `/console/servers/${encodeURIComponent(targetServer)}${tenantQuery}` : `/console${tenantQuery}`,
      );
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "加入工作区失败，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="relative min-h-[100dvh] overflow-hidden bg-[linear-gradient(180deg,#C9D4FF_0%,#DCE3FF_50%,#F5F7FD_100%)] text-[#14161F] antialiased">
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
            href="/login"
            className={`shrink-0 whitespace-nowrap rounded-full px-4 py-2 text-sm font-medium text-[#3D4258] transition-colors hover:bg-white hover:text-[#14161F] sm:px-5 ${focusRing}`}
          >
            账号登录
          </Link>
        </div>
      </header>

      <main className="mx-auto grid min-h-[100dvh] max-w-[1240px] grid-cols-1 items-center gap-8 px-6 pb-10 pt-28 lg:grid-cols-2">
        <div className="relative hidden h-[620px] lg:block [mask-image:linear-gradient(to_bottom,#000_80%,transparent)]">
          <VoxelIsland labels={[]} />
        </div>

        <div className="mx-auto w-full max-w-[440px]">
          <div className="rounded-[28px] border border-white/80 bg-white/85 p-8 shadow-[0_30px_60px_-30px_rgba(43,58,232,0.45)] backdrop-blur-xl sm:p-10">
            {loadingInfo ? (
              <div className="py-10 text-center text-sm text-[#5B6075]">正在验证工作区邀请链接…</div>
            ) : infoError || !invite ? (
              <div className="space-y-5 text-center">
                <h1 className="text-2xl font-bold text-[#14161F]">邀请链接不可用</h1>
                <p className="text-sm text-[#5B6075]">{infoError || "该邀请链接已过期或失效"}</p>
                <Link
                  href="/console"
                  className={`inline-flex h-11 items-center justify-center rounded-full bg-[#2B3AE8] px-6 text-sm font-semibold text-white ${focusRing}`}
                >
                  前往控制台
                </Link>
              </div>
            ) : (
              <>
                <p className="text-[13px] text-[#5B6075]">{invite.inviter || "好友"} 邀请你加入</p>
                <h1 className="mt-1.5 text-[26px] font-bold leading-tight tracking-tight">
                  {invite.tenantDisplayName || invite.metadata.tenant}
                </h1>
                <div className="mt-3 flex flex-wrap items-center gap-2 text-[13px] text-[#5B6075]">
                  <Badge tone="accent">{roleMeta.label}</Badge>
                  <span>{roleMeta.desc}</span>
                </div>

                <div className="my-7 h-px bg-[#EEF0F7]" />

                {existingToken && !useDifferentAccount ? (
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <span className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[#EEF1FF] text-sm font-semibold uppercase text-[#2B3AE8]">
                        {(existingUser || "U").slice(0, 1)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-semibold text-[#14161F]">{existingUser || "当前用户"}</div>
                        <div className="text-xs text-[#8A90A6]">当前登录账号</div>
                      </div>
                      <button
                        type="button"
                        onClick={() => {
                          clearSession();
                          setExistingToken(null);
                          setUseDifferentAccount(true);
                        }}
                        className={`rounded-full px-3 py-1.5 text-[13px] text-[#5B6075] hover:bg-[#F4F5FA] hover:text-[#14161F] ${focusRing}`}
                      >
                        切换账号
                      </button>
                    </div>
                    {submitError && (
                      <p role="alert" className="rounded-2xl bg-[#FDECEC] px-4 py-3 text-sm text-[#B42318]">
                        {submitError}
                      </p>
                    )}
                    <button
                      type="button"
                      disabled={submitting}
                      onClick={() => void handleAccept()}
                      className={primaryClass}
                    >
                      {submitting ? "正在加入…" : "接受邀请"}
                    </button>
                  </div>
                ) : (
                  <form onSubmit={(e) => void handleAccept(e)} className="space-y-4">
                    <div className="[&>div]:grid [&>div]:w-full [&>div]:grid-cols-2 [&>div>button]:justify-center">
                      <Segmented<"register" | "login">
                        label="账号方式"
                        value={mode}
                        onChange={(v) => {
                          setMode(v);
                          setSubmitError(null);
                        }}
                        items={[
                          { value: "register", label: "新用户注册" },
                          { value: "login", label: "已有账号" },
                        ]}
                      />
                    </div>
                    {mode === "register" && (
                      <input
                        id="inv-email"
                        aria-label="邮箱"
                        type="email"
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                        className={inputClass}
                        placeholder="邮箱"
                        required
                        autoComplete="email"
                      />
                    )}
                    <input
                      id="inv-username"
                      aria-label={mode === "register" ? "用户名" : "用户名或邮箱"}
                      type="text"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      className={inputClass}
                      placeholder={mode === "register" ? "用户名（小写字母、数字，如 steve01）" : "用户名或邮箱"}
                      required
                      autoComplete="username"
                    />
                    <div>
                      <input
                        id="inv-password"
                        aria-label="密码"
                        aria-invalid={passwordTooShort || undefined}
                        type="password"
                        value={password}
                        onChange={(e) => {
                          setPassword(e.target.value);
                          if (submitError) setSubmitError(null);
                        }}
                        className={passwordTooShort ? inputErrorClass : inputClass}
                        placeholder={mode === "register" ? "设置密码（至少 8 位）" : "密码"}
                        required
                        minLength={mode === "register" ? 8 : undefined}
                        autoComplete={mode === "register" ? "new-password" : "current-password"}
                      />
                      {passwordTooShort && (
                        <p className="mt-1.5 pl-4 text-xs text-[#D92D20]">密码至少 8 位</p>
                      )}
                    </div>
                    {mode === "register" && (
                      <div>
                        <input
                          id="inv-password-confirm"
                          aria-label="确认密码"
                          aria-invalid={passwordMismatch || undefined}
                          type="password"
                          value={confirmPassword}
                          onChange={(e) => {
                            setConfirmPassword(e.target.value);
                            if (submitError) setSubmitError(null);
                          }}
                          className={passwordMismatch ? inputErrorClass : inputClass}
                          placeholder="再次输入密码"
                          required
                          autoComplete="new-password"
                        />
                        {passwordMismatch && (
                          <p role="alert" className="mt-1.5 pl-4 text-xs text-[#D92D20]">
                            两次输入的密码不一致
                          </p>
                        )}
                      </div>
                    )}

                    {submitError && (
                      <p role="alert" className="rounded-2xl bg-[#FDECEC] px-4 py-3 text-sm text-[#B42318]">
                        {submitError}
                      </p>
                    )}

                    <button
                      type="submit"
                      disabled={submitting || passwordTooShort || passwordMismatch}
                      className={primaryClass}
                    >
                      {submitting ? "正在加入…" : mode === "register" ? "注册并加入" : "登录并加入"}
                    </button>
                  </form>
                )}
              </>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
