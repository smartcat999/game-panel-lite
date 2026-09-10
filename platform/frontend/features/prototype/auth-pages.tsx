"use client";

import { ArrowRight, Check, Github, KeyRound, ShieldCheck } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { api, type Workspace } from "@/lib/api";
import { usePrototype } from "@/lib/prototype-store";

export function LoginPage() {
  const router = useRouter();
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [username, setUsername] = useState("");
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
    setBusy(true); setError("");
    try {
      const result = await api<{ mustChangePassword: boolean }>("/auth/password/sign-in", { method: "POST", body: JSON.stringify({ username, password }) });
      if (result.mustChangePassword) setMustChange(true); else await enterWorkspace();
    } catch { setError("用户名或密码不正确"); } finally { setBusy(false); }
  };
  const changePassword = async () => {
    setBusy(true); setError("");
    try { await api<void>("/auth/password", { method: "PUT", body: JSON.stringify({ password: newPassword }) }); await enterWorkspace(); }
    catch { setError("密码至少 12 位，请重新设置"); } finally { setBusy(false); }
  };
  const github = async () => {
    setBusy(true); setError("");
    try { const result = await api<{ authorizationUrl: string }>("/auth/github/start", { method: "POST", body: JSON.stringify({ returnPath: "/w/ember/instances" }) }); window.location.assign(result.authorizationUrl); }
    catch { setError("GitHub 登录尚未配置，请使用管理员创建的账号"); setBusy(false); }
  };
  return <main className="auth-shell"><section className="auth-card">
    <div className="auth-brand"><Image alt="GamePanel" height={32} src="/icon.svg" width={32} /><strong>GamePanel</strong></div>
    <div className="auth-heading"><h1>登录</h1><p>管理游戏服务器实例</p></div>
    <Button className="github-button" disabled={busy} onClick={github}><Github size={17} />使用 GitHub 登录</Button>
    <div className="auth-divider"><span>或</span></div>
    {mustChange ? <form className="auth-form" onSubmit={(event) => { event.preventDefault(); void changePassword(); }}><label>设置新密码<input autoFocus minLength={12} onChange={(event) => setNewPassword(event.target.value)} type="password" value={newPassword} /></label><Button disabled={busy} type="submit">保存并进入<ArrowRight size={16} /></Button></form> : passwordOpen ? <form className="auth-form" onSubmit={(event) => { event.preventDefault(); void signIn(); }}><label>用户名<input autoComplete="username" autoFocus onChange={(event) => setUsername(event.target.value)} value={username} /></label><label>密码<input autoComplete="current-password" onChange={(event) => setPassword(event.target.value)} type="password" value={password} /></label><Button disabled={busy} type="submit">{busy ? "登录中" : "登录"}<ArrowRight size={16} /></Button></form> : <Button className="auth-secondary" onClick={() => setPasswordOpen(true)} variant="secondary"><KeyRound size={16} />使用用户名和密码</Button>}
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    <p className="auth-footnote"><ShieldCheck size={14} />不开放密码注册；本地账户由管理员创建。</p>
  </section></main>;
}

export function InvitationPage() {
  const { acceptInvitation, invited } = usePrototype();
  return <main className="auth-shell"><section className="auth-card invitation-card">
    <div className="auth-brand"><Image alt="GamePanel" height={32} src="/icon.svg" width={32} /><strong>GamePanel</strong></div>
    <div className="invitation-icon"><Check size={22} /></div>
    <div className="auth-heading"><h1>{invited ? "已加入工作区" : "工作区邀请"}</h1><p>Peng Wu 邀请你加入 Ember Realms</p></div>
    <dl className="invitation-detail"><div><dt>角色</dt><dd>工作区运维</dd></div><div><dt>登录账号</dt><dd>GitHub · pengwu</dd></div></dl>
    <Button asChild onClick={acceptInvitation}><Link href="/w/ember/instances">{invited ? "进入工作区" : "接受邀请"}<ArrowRight size={16} /></Link></Button>
  </section></main>;
}
