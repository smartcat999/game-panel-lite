"use client";

import { ArrowRight, Check, Github, KeyRound, ShieldCheck } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { usePrototype } from "@/lib/prototype-store";

export function LoginPage() {
  const router = useRouter();
  const [passwordOpen, setPasswordOpen] = useState(false);
  return <main className="auth-shell"><section className="auth-card">
    <div className="auth-brand"><Image alt="GamePanel" height={32} src="/icon.svg" width={32} /><strong>GamePanel</strong></div>
    <div className="auth-heading"><h1>登录</h1><p>管理游戏服务器实例</p></div>
    <Button asChild className="github-button"><Link href="/invite/demo"><Github size={17} />使用 GitHub 登录</Link></Button>
    <div className="auth-divider"><span>或</span></div>
    {passwordOpen ? <form className="auth-form" onSubmit={(event) => { event.preventDefault(); router.push("/w/ember/instances"); }}><label>用户名<input autoFocus defaultValue="pengwu" /></label><label>密码<input type="password" /></label><Button type="submit">登录<ArrowRight size={16} /></Button></form> : <Button className="auth-secondary" onClick={() => setPasswordOpen(true)} variant="secondary"><KeyRound size={16} />使用用户名和密码</Button>}
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
