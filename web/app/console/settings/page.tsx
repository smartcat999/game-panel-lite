"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Laptop, LogOut, RefreshCw, ShieldOff, Smartphone } from "lucide-react";
import { useConsole } from "../ConsoleProvider";
import { clearSession, errorMessage } from "../lib/session";
import { formatDateTime, relativeTime } from "../lib/format";
import { Avatar } from "../../components/console/TopBar";
import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, PageHeader, Segmented, Skeleton } from "../../components/console/ui";

interface PwErrors {
  current?: string;
  next?: string;
  confirm?: string;
}

interface SessionItem {
  metadata: {
    name: string;
    creationTimestamp?: string;
    annotations?: Record<string, string>;
  };
  user?: string;
  defaultTenant?: string;
  expiresAt?: string;
}

function describeUserAgent(ua?: string): { label: string; isMobile: boolean } {
  const raw = (ua || "").trim();
  if (!raw) return { label: "Web 控制台会话", isMobile: false };
  const isMobile = /Mobile|Android|iPhone|iPad/i.test(raw);
  let browser = "浏览器";
  if (/Edg\//i.test(raw)) browser = "Edge";
  else if (/Chrome\//i.test(raw) && !/Chromium/i.test(raw)) browser = "Chrome";
  else if (/Firefox\//i.test(raw)) browser = "Firefox";
  else if (/Safari\//i.test(raw) && !/Chrome\//i.test(raw)) browser = "Safari";
  else if (/curl\//i.test(raw)) browser = "CLI / curl";

  let os = "";
  if (/Mac OS X|Macintosh/i.test(raw)) os = "macOS";
  else if (/Windows NT/i.test(raw)) os = "Windows";
  else if (/iPhone|iPad/i.test(raw)) os = "iOS";
  else if (/Android/i.test(raw)) os = "Android";
  else if (/Linux/i.test(raw)) os = "Linux";

  return {
    label: os ? `${browser} · ${os}` : browser,
    isMobile,
  };
}

export default function SettingsPage() {
  const router = useRouter();
  const { me, userName, loading, apiFetch, toast, reload, logout } = useConsole();
  const [tab, setTab] = useState<"profile" | "password" | "sessions">("profile");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    const t = sp.get("tab");
    if (t === "password" || t === "sessions") setTab(t);
  }, []);

  function changeTab(next: "profile" | "password" | "sessions") {
    setTab(next);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (next === "profile") sp.delete("tab");
    else sp.set("tab", next);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  }

  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [emailError, setEmailError] = useState<string | null>(null);
  const [savingProfile, setSavingProfile] = useState(false);

  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<PwErrors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  const [sessions, setSessions] = useState<SessionItem[]>([]);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const loadSessions = useCallback(async () => {
    setLoadingSessions(true);
    try {
      const res = await apiFetch("/apis/iam.gamepanel.io/v1/sessions");
      if (res.ok) {
        const body = await res.json().catch(() => ({}));
        setSessions(Array.isArray(body?.items) ? body.items : []);
      }
    } finally {
      setLoadingSessions(false);
    }
  }, [apiFetch]);

  useEffect(() => {
    if (tab === "sessions") {
      void loadSessions();
    }
  }, [tab, loadSessions]);

  useEffect(() => {
    if (me) {
      setDisplayName(me.spec?.displayName || "");
      setEmail(me.spec?.email || "");
    }
  }, [me]);

  const username = me?.metadata.name || userName;
  const profileDirty =
    !!me &&
    (displayName.trim() !== (me.spec?.displayName || "").trim() ||
      email.trim() !== (me.spec?.email || "").trim());

  async function handleSaveProfile(ev: FormEvent) {
    ev.preventDefault();
    if (!username || !profileDirty || savingProfile) return;
    const trimmedEmail = email.trim();
    if (!trimmedEmail || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmedEmail)) {
      setEmailError("请输入有效的邮箱地址");
      return;
    }
    setEmailError(null);
    setSavingProfile(true);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(username)}`, {
        method: "PATCH",
        body: JSON.stringify({
          spec: {
            displayName: displayName.trim(),
            email: trimmedEmail,
          },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "保存个人资料失败"));
      toast("success", "个人资料已保存");
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存个人资料失败");
    } finally {
      setSavingProfile(false);
    }
  }

  function validate(): PwErrors {
    const e: PwErrors = {};
    if (!current) e.current = "请输入当前密码";
    if (!next) e.next = "请输入新密码";
    else if (next.length < 8) e.next = "新密码至少 8 位";
    else if (current && next === current) e.next = "新密码不能与当前密码相同";
    if (!confirm) e.confirm = "请再次输入新密码";
    else if (next && confirm !== next) e.confirm = "两次输入的新密码不一致";
    return e;
  }

  async function handleChangePassword(ev: FormEvent) {
    ev.preventDefault();
    setFormError(null);
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length || !username) return;

    setSaving(true);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(username)}`, {
        method: "PATCH",
        body: JSON.stringify({ spec: { currentPassword: current, password: next } }),
      });
      if (res.ok) {
        toast("success", "密码已更新，请重新登录");
        clearSession();
        router.replace("/login");
        return;
      }
      if (res.status === 403) {
        setErrors({ current: "当前密码不正确" });
        setSaving(false);
        return;
      }
      setFormError(await errorMessage(res, `修改密码失败（${res.status}），请稍后重试`));
      setSaving(false);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "修改密码失败，请稍后重试");
      setSaving(false);
    }
  }

  async function handleLogout() {
    setLoggingOut(true);
    await logout();
  }

  async function handleRevokeSession(sid: string) {
    setRevokingId(sid);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/sessions/${encodeURIComponent(sid)}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error(await errorMessage(res, "下线会话失败"));
      toast("success", "已将该设备下线");
      await loadSessions();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "下线会话失败");
    } finally {
      setRevokingId(null);
    }
  }

  async function handleRevokeOtherSessions() {
    setRevokingId("others");
    try {
      const currentSess = sessions.find((s) => s.metadata.annotations?.["iam.gamepanel.io/current"] === "true");
      if (currentSess?.metadata.name) {
        const selector = `metadata.name!=${currentSess.metadata.name}`;
        const res = await apiFetch(`/apis/iam.gamepanel.io/v1/sessions?fieldSelector=${encodeURIComponent(selector)}`, {
          method: "DELETE",
        });
        if (!res.ok) throw new Error(await errorMessage(res, "下线其他设备失败"));
      } else {
        for (const s of sessions) {
          const res = await apiFetch(`/apis/iam.gamepanel.io/v1/sessions/${encodeURIComponent(s.metadata.name)}`, {
            method: "DELETE",
          });
          if (!res.ok) throw new Error(await errorMessage(res, "下线其他设备失败"));
        }
      }
      toast("success", "已下线其他登录设备");
      await loadSessions();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "下线其他设备失败");
    } finally {
      setRevokingId(null);
    }
  }

  const otherSessionCount = sessions.filter((s) => s.metadata.annotations?.["iam.gamepanel.io/current"] !== "true").length;

  return (
    <>
      <PageHeader
        title="账号设置"
        description={username ? `当前账号：${username}` : undefined}
        actions={
          <div className="flex flex-wrap items-center gap-2.5">
            <Segmented<"profile" | "password" | "sessions">
              label="账号设置分类"
              value={tab}
              onChange={changeTab}
              items={[
                { value: "profile", label: "基本资料" },
                { value: "password", label: "修改密码" },
                { value: "sessions", label: "登录设备" },
              ]}
            />
            <Button
              variant="secondary"
              icon={<LogOut className="h-4 w-4" />}
              loading={loggingOut}
              onClick={() => void handleLogout()}
            >
              退出登录
            </Button>
          </div>
        }
      />

      {tab === "profile" ? (
        <Card>
          <CardHeader title="基本资料" />
          <div className="px-6 py-5">
            {loading && !me ? (
              <div className="space-y-3">
                <Skeleton className="h-12 w-60" />
                <Skeleton className="h-10" />
              </div>
            ) : (
              <form onSubmit={handleSaveProfile} noValidate className="space-y-5">
                <div className="flex items-center gap-4">
                  {username && <Avatar name={username} size={48} round />}
                  <div className="min-w-0">
                    <div className="truncate text-base font-semibold text-[#14161F]">
                      {displayName.trim() || me?.spec.displayName || username || "—"}
                    </div>
                    <div className="truncate text-sm text-[#5B6075]">{email.trim() || me?.spec.email || "未设置邮箱"}</div>
                  </div>
                </div>
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field label="用户名" htmlFor="prof-username">
                    <Input id="prof-username" value={username || ""} disabled />
                  </Field>
                  <Field label="显示名称" htmlFor="prof-display">
                    <Input
                      id="prof-display"
                      value={displayName}
                      placeholder={username || "输入显示名称"}
                      onChange={(e) => setDisplayName(e.target.value)}
                    />
                  </Field>
                  <Field label="联系邮箱" htmlFor="prof-email" error={emailError ?? undefined}>
                    <Input
                      id="prof-email"
                      type="email"
                      value={email}
                      placeholder="name@example.com"
                      onChange={(e) => {
                        setEmail(e.target.value);
                        if (emailError) setEmailError(null);
                      }}
                    />
                  </Field>
                </div>
                <div className="flex justify-end">
                  <Button type="submit" variant="primary" loading={savingProfile} disabled={!profileDirty || savingProfile}>
                    保存资料
                  </Button>
                </div>
              </form>
            )}
          </div>
        </Card>
      ) : tab === "password" ? (
        <Card>
          <CardHeader title="修改密码" />
          <form onSubmit={handleChangePassword} noValidate className="space-y-5 px-6 py-5">
            <input type="text" name="username" autoComplete="username" value={username} readOnly hidden />
            <Field label="当前密码" htmlFor="pw-current" error={errors.current}>
              <Input
                id="pw-current"
                type="password"
                autoComplete="current-password"
                value={current}
                aria-invalid={!!errors.current}
                onChange={(e) => setCurrent(e.target.value)}
              />
            </Field>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label="新密码" htmlFor="pw-new" error={errors.next} hint="至少 8 位；修改后需重新登录">
                <Input
                  id="pw-new"
                  type="password"
                  autoComplete="new-password"
                  value={next}
                  aria-invalid={!!errors.next}
                  onChange={(e) => setNext(e.target.value)}
                />
              </Field>
              <Field label="确认新密码" htmlFor="pw-confirm" error={errors.confirm}>
                <Input
                  id="pw-confirm"
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  aria-invalid={!!errors.confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </Field>
            </div>
            {formError && (
              <p role="alert" className="rounded-2xl border border-[#F4C7C3] bg-[#FEF3F2] px-4 py-3 text-sm text-[#B42318]">
                {formError}
              </p>
            )}
            <div className="flex justify-end">
              <Button
                type="submit"
                variant="primary"
                loading={saving}
                disabled={!current.trim() || !next.trim() || !confirm.trim() || saving}
              >
                更新密码
              </Button>
            </div>
          </form>
        </Card>
      ) : (
        <Card>
          <CardHeader
            title="已登录设备与活跃会话"
            actions={
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  icon={<RefreshCw className="h-3.5 w-3.5" />}
                  loading={loadingSessions}
                  onClick={() => void loadSessions()}
                >
                  刷新
                </Button>
                {otherSessionCount > 0 && (
                  <Button
                    size="sm"
                    variant="secondary"
                    icon={<ShieldOff className="h-3.5 w-3.5" />}
                    loading={revokingId === "others"}
                    disabled={revokingId !== null}
                    onClick={() => void handleRevokeOtherSessions()}
                  >
                    下线其他设备
                  </Button>
                )}
              </div>
            }
          />
          {loadingSessions && sessions.length === 0 ? (
            <div className="space-y-3 px-6 py-5">
              <Skeleton className="h-14" />
              <Skeleton className="h-14" />
            </div>
          ) : sessions.length === 0 ? (
            <EmptyState image={null} title="暂无活跃会话" body="当前没有查询到已记录的登录会话。" />
          ) : (
            <ul className="divide-y divide-[#EEF0F7]">
              {sessions.map((s) => {
                const isCurrent = s.metadata.annotations?.["iam.gamepanel.io/current"] === "true";
                const uaInfo = describeUserAgent(s.metadata.annotations?.["iam.gamepanel.io/user-agent"]);
                const clientIP = s.metadata.annotations?.["iam.gamepanel.io/client-ip"];
                const shortID = s.metadata.name.slice(-8);
                return (
                  <li key={s.metadata.name} className="flex items-center justify-between gap-4 px-6 py-4 hover:bg-[#F7F8FC]/60">
                    <div className="flex min-w-0 items-center gap-3.5">
                      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#EEF2FF] text-[#2B3AE8]">
                        {uaInfo.isMobile ? <Smartphone className="h-5 w-5" /> : <Laptop className="h-5 w-5" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate text-sm font-semibold text-[#14161F]">{uaInfo.label}</span>
                          {isCurrent ? (
                            <Badge tone="accent">当前设备</Badge>
                          ) : (
                            <Badge tone="success">在线</Badge>
                          )}
                        </div>
                        <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-[#5B6075]">
                          {clientIP ? <span className="font-mono">IP: {clientIP}</span> : null}
                          <span className="font-mono text-[#8A90A6]">会话 #{shortID}</span>
                          <span>登录于 {relativeTime(s.metadata.creationTimestamp)}</span>
                          {s.expiresAt ? <span className="text-[#8A90A6]">有效期至 {formatDateTime(s.expiresAt)}</span> : null}
                        </div>
                      </div>
                    </div>
                    <div className="shrink-0">
                      {isCurrent ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={loggingOut}
                          disabled={loggingOut || revokingId !== null}
                          onClick={() => void handleLogout()}
                        >
                          退出当前设备
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={revokingId === s.metadata.name}
                          disabled={revokingId !== null}
                          onClick={() => void handleRevokeSession(s.metadata.name)}
                        >
                          下线
                        </Button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>
      )}
    </>
  );
}
