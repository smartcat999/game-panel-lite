"use client";

import { Check, ChevronRight, CircleDollarSign, Plus } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { PageHeader } from "@/features/prototype/workspace-pages";
import { usePrototype } from "@/lib/prototype-store";

export function RegionsPage() {
  return <><PageHeader title="区域" /><section className="table-panel"><table className="dense-table"><thead><tr><th>区域</th><th>状态</th><th>执行器</th><th>容量</th><th /></tr></thead><tbody><tr><td><strong>华东 1</strong><span className="cell-secondary"> · cn-east-1</span></td><td><span className="state-badge state-running"><span />正常</span></td><td>3 在线</td><td>可用</td><td><Link href="/platform/regions/cn-east-1/nodes"><ChevronRight size={17} /></Link></td></tr><tr><td><strong>华北 1</strong><span className="cell-secondary"> · cn-north-1</span></td><td><span className="state-badge state-running"><span />正常</span></td><td>2 在线</td><td>紧张</td><td><ChevronRight size={17} /></td></tr></tbody></table></section></>;
}

export function CreditGrantsPage() {
  const { balanceMinor, grantCredit, exhaustWallet } = usePrototype();
  const [amount, setAmount] = useState(10000);
  const [done, setDone] = useState(false);
  return <><PageHeader title="额度发放" /><section className="split-panel"><form className="detail-panel grant-form" onSubmit={(event) => { event.preventDefault(); grantCredit(amount); setDone(true); }}><h2>发放测试额度</h2><label className="field"><span>工作区</span><select><option>Ember Realms</option></select></label><label className="field"><span>金额</span><div className="input-suffix"><input min="1" onChange={(event) => setAmount(Math.round(Number(event.target.value) * 100))} type="number" value={amount / 100} /><span>CNY</span></div></label><label className="field"><span>原因</span><input defaultValue="Beta 测试额度" /></label><Button type="submit"><Plus size={16} />发放额度</Button>{done ? <p className="form-success"><Check size={15} />额度已写入账本</p> : null}</form><aside className="detail-panel balance-panel"><span>Ember Realms 可用余额</span><strong>¥{(balanceMinor / 100).toFixed(2)}</strong><p>额度变更仅通过不可变账本记录。</p><Button onClick={exhaustWallet} variant="secondary"><CircleDollarSign size={16} />预览余额耗尽</Button></aside></section></>;
}

export function PlatformTablePage({ resource }: { resource: "instances" | "workspaces" | "users" | "price-books" | "audit" }) {
  const content = {
    instances: { title: "实例", columns: ["实例", "工作区", "区域", "状态"], rows: [["terraria-hardcore-01", "Ember Realms", "华东 1", "运行中"], ["calamity-infernum-03", "Ember Realms", "华东 1", "运行中"]] },
    workspaces: { title: "工作区", columns: ["工作区", "所有者", "状态", "创建时间"], rows: [["Ember Realms", "Peng Wu", "正常", "2026-09-07"]] },
    users: { title: "用户", columns: ["用户", "登录方式", "状态", "最近登录"], rows: [["Peng Wu", "GitHub", "正常", "今天 09:12"], ["Lin Chen", "本地账户", "正常", "昨天 18:40"]] },
    "price-books": { title: "价格表", columns: ["区域", "版本", "币种", "生效时间"], rows: [["华东 1", "rev-7", "CNY", "2026-09-01"], ["华北 1", "rev-4", "CNY", "2026-09-01"]] },
    audit: { title: "审计", columns: ["时间", "操作人", "动作", "资源"], rows: [["今天 10:18", "Peng Wu", "credit.grant", "Ember Realms"], ["今天 09:42", "Peng Wu", "instance.restart", "terraria-hardcore-01"]] },
  }[resource];
  return <><PageHeader title={content.title} /><section className="table-panel"><table className="dense-table"><thead><tr>{content.columns.map((column) => <th key={column}>{column}</th>)}</tr></thead><tbody>{content.rows.map((row) => <tr key={row.join(":")}>{row.map((cell, index) => <td key={cell}>{index === 0 ? <strong>{cell}</strong> : cell}</td>)}</tr>)}</tbody></table></section></>;
}

export function RegionPage({ section }: { section: string }) {
  return <><PageHeader title={{ nodes: "节点", deployments: "部署", tasks: "任务", capacity: "容量", storage: "存储", monitoring: "监控" }[section] ?? "区域"} /><section className="table-panel"><table className="dense-table"><thead><tr><th>资源</th><th>状态</th><th>更新时间</th></tr></thead><tbody><tr><td><strong>{section === "nodes" ? "um773-worker-01" : section === "deployments" ? "terraria-hardcore-01" : `${section}-cn-east-1`}</strong></td><td><span className="state-badge state-running"><span />正常</span></td><td>刚刚</td></tr></tbody></table></section></>;
}
