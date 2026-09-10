"use client";

import {
  Archive,
  Boxes,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  Clipboard,
  FileText,
  LoaderCircle,
  MoreHorizontal,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Server,
  Settings,
  Square,
  TerminalSquare,
  WalletCards,
} from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { defaultConfiguration, providerManifests, type ConfigurationField, type ProviderManifestFixture } from "@/lib/provider-fixtures";
import { type InstanceState, type PrototypeInstance, usePrototype } from "@/lib/prototype-store";
import { cn } from "@/lib/utils";

export function PageHeader({ title, actions }: { title: string; actions?: React.ReactNode }) {
  return <div className="page-bar"><h1>{title}</h1>{actions ? <div className="page-actions">{actions}</div> : null}</div>;
}

function StateBadge({ state }: { state: InstanceState }) {
  const label = { running: "运行中", stopped: "已停止", starting: "部署中", failed: "失败" }[state];
  return <span className={cn("state-badge", `state-${state}`)}><span />{label}</span>;
}

function ScenarioAlert() {
  const scenario = useSearchParams().get("state");
  if (!scenario) return null;
  const messages: Record<string, { title: string; body: string }> = {
    forbidden: { title: "没有操作权限", body: "当前角色不能修改此资源。权限来自工作区角色绑定。" },
    failed: { title: "操作失败", body: "区域执行器未能完成任务。可在操作记录中查看失败步骤并重试。" },
    stale: { title: "状态可能已过期", body: "区域最近一次上报已超过 90 秒，页面保留最后观测结果。" },
    unsupported: { title: "Provider 不支持此能力", body: "当前游戏版本没有声明该配置或操作，界面已安全禁用。" },
    credit: { title: "余额不足", body: "无法创建或启动实例。平台管理员需要先发放测试额度。" },
  };
  const message = messages[scenario];
  return message ? <div className="inline-alert" role="alert"><CircleAlert size={17} /><div><strong>{message.title}</strong><span>{message.body}</span></div></div> : null;
}

export function InstanceListPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { instances } = usePrototype();
  return <>
    <PageHeader title="实例" actions={<Button asChild><Link href={`/w/${workspaceSlug}/instances/new`}><Plus size={16} />创建实例</Link></Button>} />
    <ScenarioAlert />
    <section className="table-panel" aria-label="实例列表">
      <table className="dense-table">
        <thead><tr><th>名称</th><th>状态</th><th>游戏</th><th>Endpoint</th><th>资源</th><th>区域</th><th><span className="sr-only">操作</span></th></tr></thead>
        <tbody>{instances.map((instance) => <InstanceRow instance={instance} key={instance.id} workspaceSlug={workspaceSlug} />)}</tbody>
      </table>
    </section>
  </>;
}

function InstanceRow({ instance, workspaceSlug }: { instance: PrototypeInstance; workspaceSlug: string }) {
  const { setInstanceState } = usePrototype();
  return <tr className={instance.state === "stopped" ? "muted-row" : undefined}>
    <td><Link className="resource-name" href={`/w/${workspaceSlug}/instances/${instance.id}`}>{instance.name}</Link>{instance.stale ? <span className="stale-tag">状态延迟</span> : null}</td>
    <td><StateBadge state={instance.state} /></td>
    <td><span className={cn("provider-tag", instance.supportsMods && "provider-modded")}>{instance.provider} {instance.version}</span></td>
    <td><EndpointSummary instance={instance} /></td>
    <td><strong>{instance.memoryMiB / 1024} GB</strong><span className="cell-secondary"> · {instance.cpuMilli / 1000} vCPU</span></td>
    <td>{instance.region}</td>
    <td><div className="row-actions">
      {instance.state === "stopped" ? <button aria-label="启动" onClick={() => setInstanceState(instance.id, "starting")} type="button"><Play size={16} /></button> : <button aria-label="重启" onClick={() => setInstanceState(instance.id, "starting")} type="button"><RefreshCw size={16} /></button>}
      {instance.state !== "stopped" ? <button aria-label="停止" onClick={() => setInstanceState(instance.id, "stopped")} type="button"><Square size={15} /></button> : null}
      <Link aria-label="设置" href={`/w/${workspaceSlug}/instances/${instance.id}?tab=configuration`}><Settings size={17} /></Link>
      <Link aria-label="打开实例" href={`/w/${workspaceSlug}/instances/${instance.id}`}><ChevronRight size={17} /></Link>
    </div></td>
  </tr>;
}

function EndpointSummary({ instance }: { instance: PrototypeInstance }) {
  if (!instance.endpoint) return <span className="muted-text">分配中</span>;
  return <span className="endpoint-summary"><code>{instance.endpoint}</code><span>{instance.transports.join("/")} · {instance.endpointStability === "may-change" ? "可能变化" : "固定"}</span></span>;
}

type WizardValues = Record<string, string | number | boolean | string[]>;

export function CreateInstancePage({ workspaceSlug }: { workspaceSlug: string }) {
  const router = useRouter();
  const { balanceMinor, createInstance } = usePrototype();
  const [manifestId, setManifestId] = useState(providerManifests[0].id);
  const manifest = providerManifests.find((item) => item.id === manifestId) ?? providerManifests[0];
  const steps = useMemo(() => ["基础", "资源", "游戏配置", ...(manifest.capabilities.includes("mods") ? ["模组"] : []), "确认"], [manifest]);
  const [step, setStep] = useState(0);
  const [name, setName] = useState("terraria-new-01");
  const [region, setRegion] = useState("华东 1");
  const [cpuMilli, setCpuMilli] = useState(2000);
  const [memoryMiB, setMemoryMiB] = useState(4096);
  const [diskGiB, setDiskGiB] = useState(20);
  const [configuration, setConfiguration] = useState<WizardValues>(() => defaultConfiguration(manifest));
  const [modText, setModText] = useState("CalamityMod\nInfernumMode");
  const hourlyMinor = Math.round(cpuMilli / 1000 * 28 + memoryMiB / 1024 * 16 + diskGiB * 0.35);

  const changeManifest = (id: string) => {
    const next = providerManifests.find((item) => item.id === id) ?? providerManifests[0];
    setManifestId(id);
    setConfiguration(defaultConfiguration(next));
    setStep(0);
  };

  const deploy = () => {
    if (balanceMinor < hourlyMinor) return;
    const instance = createInstance({
      name,
      provider: manifest.game,
      version: manifest.version,
      region,
      cpuMilli,
      memoryMiB,
      diskGiB,
      supportsMods: manifest.capabilities.includes("mods"),
      serverName: String(configuration.serverName ?? name),
      maxPlayers: Number(configuration.maxPlayers ?? 16),
      password: String(configuration.password ?? ""),
      modIds: modText.split("\n").map((item) => item.trim()).filter(Boolean),
      players: undefined,
      stale: false,
    });
    router.push(`/w/${workspaceSlug}/operations/op_create_${instance.id}?instance=${instance.id}`);
  };

  return <>
    <PageHeader title="创建实例" />
    <section className="wizard-panel">
      <ol className="wizard-steps">{steps.map((label, index) => <li className={cn(index === step && "active", index < step && "done")} key={label}><span>{index < step ? <Check size={13} /> : index + 1}</span>{label}</li>)}</ol>
      <div className="wizard-body">
        {step === 0 ? <div className="form-grid">
          <Field label="实例名称"><input onChange={(event) => setName(event.target.value)} value={name} /></Field>
          <Field label="游戏与版本"><select onChange={(event) => changeManifest(event.target.value)} value={manifestId}>{providerManifests.map((item) => <option key={item.id} value={item.id}>{item.game} · {item.version}</option>)}</select></Field>
          <Field label="区域"><select onChange={(event) => setRegion(event.target.value)} value={region}><option>华东 1</option><option>华北 1</option></select></Field>
        </div> : null}
        {step === 1 ? <div className="form-grid three-columns">
          <Field label="vCPU"><select onChange={(event) => setCpuMilli(Number(event.target.value))} value={cpuMilli}><option value={1000}>1 vCPU</option><option value={2000}>2 vCPU</option><option value={3000}>3 vCPU</option><option value={4000}>4 vCPU</option></select></Field>
          <Field label="内存"><select onChange={(event) => setMemoryMiB(Number(event.target.value))} value={memoryMiB}><option value={2048}>2 GB</option><option value={4096}>4 GB</option><option value={6144}>6 GB</option><option value={8192}>8 GB</option></select></Field>
          <Field label="磁盘"><div className="input-suffix"><input min={10} onChange={(event) => setDiskGiB(Number(event.target.value))} type="number" value={diskGiB} /><span>GB</span></div></Field>
          <div className="form-note span-all">公网地址与端口由系统部署时自动分配，协议由游戏 Provider 声明。</div>
        </div> : null}
        {steps[step] === "游戏配置" ? <ConfigurationRenderer manifest={manifest} onChange={setConfiguration} values={configuration} /> : null}
        {steps[step] === "模组" ? <div className="form-grid"><Field label="Workshop ID / 模组标识" hint="每行一个，部署时由运行环境联网下载"><textarea onChange={(event) => setModText(event.target.value)} rows={6} value={modText} /></Field></div> : null}
        {steps[step] === "确认" ? <div className="review-grid">
          <Review label="实例" value={name} /><Review label="游戏" value={`${manifest.game} ${manifest.version}`} /><Review label="区域" value={region} />
          <Review label="规格" value={`${cpuMilli / 1000} vCPU · ${memoryMiB / 1024} GB · ${diskGiB} GB`} />
          <Review label="预计费用" value={`¥${(hourlyMinor / 100).toFixed(2)} / 小时`} /><Review label="可用余额" value={`¥${(balanceMinor / 100).toFixed(2)}`} />
          {balanceMinor < hourlyMinor ? <div className="inline-alert span-all"><CircleAlert size={17} /><div><strong>余额不足</strong><span>请联系平台管理员发放测试额度。</span></div></div> : null}
        </div> : null}
      </div>
      <div className="wizard-footer">
        {step > 0 ? <Button onClick={() => setStep((current) => current - 1)} variant="secondary"><ChevronLeft size={16} />上一步</Button> : <span />}
        {step < steps.length - 1 ? <Button onClick={() => setStep((current) => current + 1)}>下一步<ChevronRight size={16} /></Button> : <Button disabled={balanceMinor < hourlyMinor} onClick={deploy}><Server size={16} />创建并部署</Button>}
      </div>
    </section>
  </>;
}

function ConfigurationRenderer({ manifest, values, onChange }: { manifest: ProviderManifestFixture; values: WizardValues; onChange: (values: WizardValues) => void }) {
  return <div className="schema-form">{manifest.configuration.map((section) => <section key={section.id}><h2>{section.title}</h2><div className="form-grid">{Object.entries(section.fields).map(([key, field]) => <ConfigurationInput field={field} key={key} onChange={(value) => onChange({ ...values, [key]: value })} value={values[key]} />)}</div></section>)}</div>;
}

function ConfigurationInput({ field, value, onChange }: { field: ConfigurationField; value: WizardValues[string]; onChange: (value: WizardValues[string]) => void }) {
  if (field.type === "boolean") return <label className="switch-row"><span><strong>{field.title}</strong>{field.description ? <small>{field.description}</small> : null}</span><input checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)} type="checkbox" /></label>;
  if (field.type === "enum") return <Field label={field.title} hint={field.description}><select onChange={(event) => onChange(event.target.value)} value={String(value)}>{field.options?.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>;
  if (field.type === "integer" || field.type === "number") return <Field label={field.title} hint={field.description}><input max={field.maximum} min={field.minimum} onChange={(event) => onChange(Number(event.target.value))} type="number" value={Number(value)} /></Field>;
  if (field.type === "string-list") return <Field label={field.title} hint={field.description}><textarea onChange={(event) => onChange(event.target.value.split("\n"))} rows={3} value={Array.isArray(value) ? value.join("\n") : ""} /></Field>;
  return <Field label={field.title} hint={field.description}><input onChange={(event) => onChange(event.target.value)} type={field.type === "secret" ? "password" : "text"} value={String(value)} /></Field>;
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="field"><span>{label}</span>{children}{hint ? <small>{hint}</small> : null}</label>;
}

function Review({ label, value }: { label: string; value: string }) {
  return <div className="review-item"><span>{label}</span><strong>{value}</strong></div>;
}

export function OperationPage({ workspaceSlug, operationId }: { workspaceSlug: string; operationId: string }) {
  const params = useSearchParams();
  const router = useRouter();
  const { instances, setInstanceState } = usePrototype();
  const instanceId = params.get("instance") ?? "lin_terraria01";
  const [progress, setProgress] = useState(1);
  const steps = ["记录请求", "预留容量", "创建运行环境", "分配 Endpoint", "启动并检查"];
  useEffect(() => {
    if (progress >= steps.length) return;
    const timer = window.setTimeout(() => setProgress((current) => current + 1), 650);
    return () => window.clearTimeout(timer);
  }, [progress, steps.length]);
  useEffect(() => {
    const instance = instances.find((item) => item.id === instanceId);
    if (progress >= steps.length && instance?.state === "starting") setInstanceState(instanceId, "running");
  }, [instanceId, instances, progress, setInstanceState, steps.length]);
  return <>
    <PageHeader title="操作详情" actions={progress >= steps.length ? <Button onClick={() => router.push(`/w/${workspaceSlug}/instances/${instanceId}`)}>查看实例<ChevronRight size={16} /></Button> : undefined} />
    <section className="operation-panel">
      <div className="operation-meta"><div><span>操作</span><code>{operationId}</code></div><div><span>类型</span><strong>创建实例</strong></div><div><span>状态</span><strong className="success-text">{progress >= steps.length ? "已完成" : "执行中"}</strong></div></div>
      <ol className="operation-steps">{steps.map((label, index) => <li key={label}><span className={cn("step-icon", index < progress && "complete", index === progress && "current")}>{index < progress ? <Check size={15} /> : index === progress ? <LoaderCircle className="spin" size={15} /> : index + 1}</span><div><strong>{label}</strong><small>{index < progress ? "完成" : index === progress ? "正在处理" : "等待"}</small></div></li>)}</ol>
    </section>
  </>;
}

export function InstanceDetailPage({ workspaceSlug, instanceId }: { workspaceSlug: string; instanceId: string }) {
  const search = useSearchParams();
  const router = useRouter();
  const { instances, setInstanceState, createBackup, backups, restoreBackup } = usePrototype();
  const instance = instances.find((item) => item.id === instanceId) ?? instances[0];
  const tab = search.get("tab") ?? "overview";
  const tabs = [
    { id: "overview", label: "概览", icon: Server },
    { id: "console", label: "终端控制台", icon: TerminalSquare },
    { id: "logs", label: "实时日志", icon: FileText },
    { id: "backups", label: "备份", icon: Archive },
    { id: "configuration", label: "配置", icon: Settings },
    ...(instance.supportsMods ? [{ id: "mods", label: "模组", icon: Boxes }] : []),
  ];
  return <>
    <section className="instance-header">
      <Link className="breadcrumb" href={`/w/${workspaceSlug}/instances`}><ChevronLeft size={15} />实例</Link>
      <div className="instance-title-row"><div><h1>{instance.name}</h1><StateBadge state={instance.state} /><EndpointSummary instance={instance} /></div><div className="header-actions">
        <button aria-label="复制地址" onClick={() => navigator.clipboard?.writeText(instance.endpoint ?? "")} type="button"><Clipboard size={17} /></button>
        <button aria-label="重启" onClick={() => setInstanceState(instance.id, "starting")} type="button"><RefreshCw size={17} /></button>
        {instance.state === "stopped" ? <button aria-label="启动" onClick={() => setInstanceState(instance.id, "running")} type="button"><Play size={17} /></button> : <button aria-label="停止" onClick={() => setInstanceState(instance.id, "stopped")} type="button"><Square size={16} /></button>}
      </div></div>
      <nav className="detail-tabs">{tabs.map((item) => { const Icon = item.icon; return <button className={tab === item.id ? "active" : undefined} key={item.id} onClick={() => router.push(`?tab=${item.id}`)} type="button"><Icon size={16} />{item.label}</button>; })}</nav>
    </section>
    <ScenarioAlert />
    {tab === "overview" ? <OverviewTab instance={instance} /> : null}
    {tab === "console" ? <ConsoleTab /> : null}
    {tab === "logs" ? <LogsTab /> : null}
    {tab === "backups" ? <InstanceBackups backups={backups.filter((item) => item.instanceId === instance.id)} create={() => createBackup(instance.id)} restore={restoreBackup} /> : null}
    {tab === "configuration" ? <ConfigurationTab instance={instance} /> : null}
    {tab === "mods" ? <ModsTab /> : null}
  </>;
}

function OverviewTab({ instance }: { instance: PrototypeInstance }) {
  return <section className="detail-panel"><div className="compact-definition-grid">
    <Review label="游戏" value={`${instance.provider} ${instance.version}`} />
    <Review label="区域" value={instance.region} />
    <Review label="规格" value={`${instance.cpuMilli / 1000} vCPU · ${instance.memoryMiB / 1024} GB · ${instance.diskGiB} GB`} />
    {instance.players ? <Review label="玩家" value={`${instance.players.current} / ${instance.players.maximum}`} /> : <Review label="玩家" value="Provider 未提供" />}
  </div></section>;
}

function ConsoleTab() {
  const [command, setCommand] = useState("");
  const [lines, setLines] = useState(["> status", "Server is running", "Players: 12/16"]);
  return <section className="terminal-panel"><pre>{lines.join("\n")}</pre><form onSubmit={(event) => { event.preventDefault(); if (!command.trim()) return; setLines((current) => [...current, `> ${command}`, "Command accepted"]); setCommand(""); }}><span>&gt;</span><input aria-label="控制台命令" onChange={(event) => setCommand(event.target.value)} value={command} /><button type="submit">发送</button></form></section>;
}

function LogsTab() {
  return <section className="terminal-panel logs"><pre>{`09:42:18 [info] Logical instance lin_terraria01 attached\n09:42:19 [info] Runtime attempt rta_9f21 started\n09:42:21 [info] Listening on 0.0.0.0:7777\n09:42:24 [info] Server started\n09:51:08 [join] pengwu joined`}</pre></section>;
}

function ConfigurationTab({ instance }: { instance: PrototypeInstance }) {
  const manifest = providerManifests.find((item) => item.game === instance.provider) ?? providerManifests[0];
  const [values, setValues] = useState<WizardValues>(() => defaultConfiguration(manifest));
  const [saved, setSaved] = useState(false);
  return <section className="detail-panel"><ConfigurationRenderer manifest={manifest} onChange={(next) => { setValues(next); setSaved(false); }} values={values} /><div className="panel-footer"><Button onClick={() => setSaved(true)}><Save size={16} />{saved ? "已保存" : "保存配置"}</Button></div></section>;
}

function ModsTab() {
  return <section className="detail-panel"><div className="mod-row"><div><strong>Calamity Mod</strong><span>Workshop 2824688072</span></div><span className="state-badge state-running"><span />已启用</span></div><div className="mod-row"><div><strong>Infernum Mode</strong><span>Workshop 2670628346</span></div><span className="state-badge state-running"><span />已启用</span></div></section>;
}

function InstanceBackups({ backups, create, restore }: { backups: ReturnType<typeof usePrototype>["backups"]; create: () => void; restore: (id: string) => void }) {
  return <section className="detail-panel"><div className="panel-toolbar"><Button onClick={create}><Plus size={16} />创建备份</Button></div>{backups.length ? backups.map((backup) => <div className="backup-row" key={backup.id}><div><strong>{backup.label}</strong><span>{backup.createdAt} · {backup.size}</span></div><Button disabled={backup.state === "restoring"} onClick={() => restore(backup.id)} variant="secondary"><RotateCcw size={15} />{backup.state === "restoring" ? "恢复中" : "恢复"}</Button></div>) : <div className="compact-empty">暂无备份</div>}</section>;
}

export function BackupsPage() {
  const { backups, instances, createBackup, restoreBackup } = usePrototype();
  const [instanceId, setInstanceId] = useState(instances[0].id);
  return <><PageHeader title="备份" actions={<div className="inline-controls"><select onChange={(event) => setInstanceId(event.target.value)} value={instanceId}>{instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}</select><Button onClick={() => createBackup(instanceId)}><Plus size={16} />创建备份</Button></div>} /><section className="table-panel"><table className="dense-table"><thead><tr><th>备份</th><th>实例</th><th>创建时间</th><th>大小</th><th /></tr></thead><tbody>{backups.map((backup) => <tr key={backup.id}><td><strong>{backup.label}</strong></td><td>{instances.find((item) => item.id === backup.instanceId)?.name}</td><td>{backup.createdAt}</td><td>{backup.size}</td><td><Button disabled={backup.state === "restoring"} onClick={() => restoreBackup(backup.id)} variant="secondary"><RotateCcw size={15} />{backup.state === "restoring" ? "恢复中" : "恢复"}</Button></td></tr>)}</tbody></table></section></>;
}

export function BillingPage() {
  const { balanceMinor } = usePrototype();
  return <><PageHeader title="账单" /><section className="billing-strip"><div><span>可用余额</span><strong>¥{(balanceMinor / 100).toFixed(2)}</strong></div><div><span>本月资源用量</span><strong>¥36.28</strong></div><div><span>计费方式</span><strong>按小时 · 资源规格</strong></div></section><section className="table-panel"><table className="dense-table"><thead><tr><th>时间</th><th>类型</th><th>资源</th><th>金额</th><th>余额</th></tr></thead><tbody><tr><td>今天 10:00</td><td>资源用量</td><td>terraria-hardcore-01</td><td>-¥0.99</td><td>¥{(balanceMinor / 100).toFixed(2)}</td></tr><tr><td>09-08 12:20</td><td>测试额度</td><td>平台发放</td><td className="success-text">+¥100.00</td><td>¥129.39</td></tr></tbody></table></section><p className="page-footnote"><WalletCards size={15} />Beta 期间仅支持平台管理员发放测试额度，不提供充值入口。</p></>;
}

export function MembersPage() {
  return <><PageHeader title="成员" actions={<Button><Plus size={16} />邀请成员</Button>} /><section className="table-panel"><table className="dense-table"><thead><tr><th>成员</th><th>登录方式</th><th>角色</th><th>加入时间</th><th /></tr></thead><tbody><tr><td><strong>Peng Wu</strong><span className="cell-secondary"> · pengwu</span></td><td>GitHub</td><td><span className="role-tag">工作区所有者</span></td><td>2026-09-07</td><td><MoreHorizontal size={17} /></td></tr><tr><td><strong>Lin Chen</strong><span className="cell-secondary"> · linchen</span></td><td>邀请</td><td><span className="role-tag">工作区运维</span></td><td>2026-09-09</td><td><MoreHorizontal size={17} /></td></tr></tbody></table></section></>;
}

export function OperationsPage({ workspaceSlug }: { workspaceSlug: string }) {
  return <><PageHeader title="操作" /><section className="table-panel"><table className="dense-table"><thead><tr><th>操作</th><th>资源</th><th>状态</th><th>发起时间</th><th /></tr></thead><tbody><tr><td>重启实例</td><td>terraria-hardcore-01</td><td><span className="success-text">已完成</span></td><td>今天 09:42</td><td><Link href={`/w/${workspaceSlug}/operations/op_restart_01`}><ChevronRight size={17} /></Link></td></tr><tr><td>恢复备份</td><td>calamity-infernum-03</td><td><span className="success-text">已完成</span></td><td>昨天 18:06</td><td><ChevronRight size={17} /></td></tr></tbody></table></section></>;
}

export function SettingsPage() {
  return <><PageHeader title="工作区设置" /><section className="detail-panel"><div className="form-grid"><Field label="工作区名称"><input defaultValue="Ember Realms" /></Field><Field label="标识"><input defaultValue="ember" disabled /></Field></div><div className="panel-footer"><Button><Save size={16} />保存</Button></div></section></>;
}
