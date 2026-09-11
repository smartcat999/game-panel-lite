"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Check, ChevronLeft, ChevronRight, Clipboard, FileText, LoaderCircle, Play, Plus, RefreshCw, RotateCcw, Save, Server, Settings, Square, TerminalSquare } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { api, idempotencyKey, type Backup, type ConfigurationField, type Instance, type InstanceListItem, type LogEntry, type Operation, type ProviderManifest, type ProviderSummary, type Region, type RegionCatalog, type Revision, type Wallet, type Workspace } from "@/lib/api";
import { cn } from "@/lib/utils";

type Values = Record<string, unknown>;

export function PageHeader({ title, actions }: { title: string; actions?: React.ReactNode }) {
  return <div className="page-bar"><h1>{title}</h1>{actions ? <div className="page-actions">{actions}</div> : null}</div>;
}

function useWorkspace(slug: string) {
  const router = useRouter();
  const query = useQuery({ queryKey: ["workspaces"], queryFn: () => api<Workspace[]>("/workspaces") });
  useEffect(() => { if (query.error && (query.error as { status?: number }).status === 401) router.replace("/login"); }, [query.error, router]);
  return { ...query, workspace: query.data?.find((item) => item.slug === slug) };
}

function Loading() { return <section className="detail-panel compact-empty"><LoaderCircle className="spin" size={18} />正在加载</section>; }
function ErrorNotice({ text }: { text: string }) { return <div className="inline-alert" role="alert"><strong>{text}</strong></div>; }

function StateBadge({ state }: { state: string }) {
  const normalized = state === "ready" || state === "running" ? "running" : state === "stopped" ? "stopped" : state === "failed" ? "failed" : "starting";
  const labels: Record<string, string> = { running: "运行中", stopped: "已停止", pending: "等待部署", starting: "启动中", stopping: "停止中", failed: "失败", unknown: "状态未知" };
  return <span className={cn("state-badge", `state-${normalized}`)}><span />{labels[state] ?? labels[normalized]}</span>;
}

function primaryEndpoint(instance: Pick<Instance, "endpoints">) { return instance.endpoints?.find((item) => item.primary) ?? instance.endpoints?.[0]; }

function EndpointSummary({ instance }: { instance: Pick<Instance, "endpoints" | "observedState"> }) {
  const endpoint = primaryEndpoint(instance);
  if (!endpoint) return <span className="muted-text">{instance.observedState === "pending" || instance.observedState === "starting" ? "等待分配" : "未分配"}</span>;
  const additional = instance.endpoints.length - 1;
  return <span className="endpoint-summary"><span className="endpoint-primary"><code>{endpoint.displayAddress}</code>{additional > 0 ? <span className="endpoint-count">+{additional}</span> : null}</span><span className="endpoint-meta">{endpoint.transports.join("/").toUpperCase()} · {endpoint.stability === "stable" ? "固定" : "可能变化"}</span></span>;
}

export function InstanceListPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { workspace, isLoading, error: workspaceError } = useWorkspace(workspaceSlug);
  const instances = useQuery({ queryKey: ["instances", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<InstanceListItem[]>(`/workspaces/${workspace!.id}/instances`) });
  return <>
    <PageHeader title="实例" actions={<Button asChild><Link href={`/w/${workspaceSlug}/instances/new`}><Plus size={16} />创建实例</Link></Button>} />
    {workspaceError || !isLoading && !workspace ? <ErrorNotice text="工作区不存在或暂时不可用" /> : instances.error ? <ErrorNotice text="实例列表暂时不可用" /> : null}
    <section className="table-panel instance-list-panel" aria-busy={isLoading || instances.isLoading} aria-label="实例列表"><table className="dense-table instance-table"><thead><tr><th>名称</th><th>状态</th><th>游戏与版本</th><th>Endpoint</th><th>规格</th><th>区域</th><th><span className="sr-only">进入详情</span></th></tr></thead><tbody>
      {isLoading || instances.isLoading ? <InstanceListSkeleton /> : null}
      {instances.data?.map((instance) => <tr className={cn("instance-row", instance.observedState === "stopped" && "muted-row")} key={instance.id}><td className="instance-name-cell"><Link aria-label={`打开实例 ${instance.name}`} className="resource-name instance-row-link" href={`/w/${workspaceSlug}/instances/${instance.id}`} title={instance.name}>{instance.name}</Link></td><td><StateBadge state={instance.observedState} /></td><td><span className="game-summary"><strong>{instance.game.displayName}</strong><code>{instance.game.version}</code></span></td><td><EndpointSummary instance={instance} /></td><td className="resource-spec"><strong>{formatMemory(instance.resourceSpec.memoryMiB)}</strong><span>{formatCPU(instance.resourceSpec.cpuMilli)}</span></td><td className="region-name">{instance.region.displayName}</td><td className="row-chevron"><ChevronRight aria-hidden="true" size={17} /></td></tr>)}
      {!isLoading && !instances.isLoading && instances.data?.length === 0 ? <tr><td className="compact-empty" colSpan={7}>暂无实例</td></tr> : null}
    </tbody></table></section>
  </>;
}

function formatMemory(memoryMiB: number) {
  const gibibytes = memoryMiB / 1024;
  return `${Number.isInteger(gibibytes) ? gibibytes : gibibytes.toFixed(1)} GB`;
}

function formatCPU(cpuMilli: number) {
  const cpu = cpuMilli / 1000;
  return `${Number.isInteger(cpu) ? cpu : cpu.toFixed(1)} vCPU`;
}

function InstanceListSkeleton() {
  return <>{[0, 1, 2].map((row) => <tr className="skeleton-row" key={row} aria-hidden="true">{["wide", "short", "medium", "wide", "short", "medium", "icon"].map((width, column) => <td key={`${row}-${column}`}><span className={`skeleton skeleton-${width}`} /></td>)}</tr>)}</>;
}

export function CreateInstancePage({ workspaceSlug }: { workspaceSlug: string }) {
  const router = useRouter();
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug);
  const providers = useQuery({ queryKey: ["providers", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<ProviderSummary[]>(`/providers/releases?workspaceId=${workspace!.id}`) });
  const regions = useQuery({ queryKey: ["regions", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Region[]>(`/regions?workspaceId=${workspace!.id}`) });
  const [providerID, setProviderID] = useState("");
  const [regionID, setRegionID] = useState("");
  const currentProviders = useMemo(() => latestProviderSummaries(providers.data ?? []), [providers.data]);
  useEffect(() => { if (!providerID && currentProviders[0]) setProviderID(currentProviders[0].id); }, [currentProviders, providerID]);
  useEffect(() => { if (!regionID && regions.data?.[0]) setRegionID(regions.data[0].id); }, [regionID, regions.data]);
  const manifest = useQuery({ queryKey: ["manifest", workspace?.id, providerID], enabled: Boolean(workspace && providerID), queryFn: () => api<ProviderManifest>(`/providers/releases/${providerID}/manifest?workspaceId=${workspace!.id}`) });
  const catalog = useQuery({ queryKey: ["catalog", workspace?.id, regionID], enabled: Boolean(workspace && regionID), queryFn: () => api<RegionCatalog>(`/regions/${regionID}/catalog?workspaceId=${workspace!.id}`) });
  const wallet = useQuery({ queryKey: ["wallet", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Wallet>(`/workspaces/${workspace!.id}/wallet`) });
  const [name, setName] = useState("terraria-server-01");
  const [cpuMilli, setCPU] = useState(2000); const [memoryMiB, setMemory] = useState(4096); const [diskGiB, setDisk] = useState(20);
  const [values, setValues] = useState<Values>({}); const [mods, setMods] = useState<Record<string, string>>({}); const [step, setStep] = useState(0); const [error, setError] = useState("");
  useEffect(() => { if (manifest.data) { setValues(defaultValues(manifest.data)); setMods({}); setStep(0); } }, [manifest.data]);
  useEffect(() => { const bounds = catalog.data?.resourceBounds; if (bounds) { setCPU(clampStep(2000, bounds.cpuMilli)); setMemory(clampStep(4096, bounds.memoryMiB)); setDisk(clampStep(20, bounds.diskGiB)); } }, [catalog.data]);
  const steps = useMemo(() => ["基础", "资源", "游戏配置", ...(manifest.data?.capabilities.includes("mods") ? ["模组"] : []), "确认"], [manifest.data]);
  const requiredMods = useMemo(() => resolveRequiredMods(manifest.data, mods), [manifest.data, mods]);
  const deploy = useMutation({ mutationFn: async () => {
    if (!workspace || !manifest.data) throw new Error("not ready");
    const quote = await api<{ id: string }>(`/workspaces/${workspace.id}/quotes`, { method: "POST", body: JSON.stringify({ regionId: regionID, providerReleaseId: providerID, resourceSpec: { cpuMilli, memoryMiB, diskGiB } }) });
    return api<{ instance: Instance; operation: Operation }>(`/workspaces/${workspace.id}/instances`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("create") }, body: JSON.stringify({ name, providerReleaseId: providerID, gameVersion: manifest.data.gameVersions[0], configuration: values, modSelections: Object.entries(mods).map(([modId, version]) => ({ modId, version })), quoteId: quote.id }) });
  }, onSuccess: ({ instance, operation }) => router.push(`/w/${workspaceSlug}/operations/${operation.id}?instance=${instance.id}`), onError: () => setError("创建请求未被接受，请检查配置与余额") });
  if (workspaceLoading || providers.isLoading || regions.isLoading || manifest.isLoading || catalog.isLoading) return <Loading />;
  if (workspaceError || providers.error || regions.error || manifest.error || catalog.error || !workspace || !manifest.data || !catalog.data) return <ErrorNotice text="创建所需的区域、规格或游戏配置暂时不可用" />;
  return <><PageHeader title="创建实例" /><section className="wizard-panel">
    <ol className="wizard-steps">{steps.map((label, index) => <li className={cn(index === step && "active", index < step && "done")} key={label}><span>{index < step ? <Check size={13} /> : index + 1}</span>{label}</li>)}</ol>
    <div className="wizard-body">
      {step === 0 ? <div className="form-grid"><Field label="实例名称"><input onChange={(event) => setName(event.target.value)} value={name} /></Field><Field label="游戏与版本"><select onChange={(event) => setProviderID(event.target.value)} value={providerID}>{currentProviders.map((item) => <option key={item.id} value={item.id}>{item.displayName} · {item.gameVersions[0]}</option>)}</select></Field><Field label="区域"><select onChange={(event) => setRegionID(event.target.value)} value={regionID}>{regions.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field></div> : null}
      {step === 1 ? <div className="form-grid three-columns"><NumberField label="vCPU (m)" range={catalog.data.resourceBounds.cpuMilli} set={setCPU} value={cpuMilli} /><NumberField label="内存 (MiB)" range={catalog.data.resourceBounds.memoryMiB} set={setMemory} value={memoryMiB} /><NumberField label="磁盘 (GiB)" range={catalog.data.resourceBounds.diskGiB} set={setDisk} value={diskGiB} /><div className="form-note span-all">公网地址、端口与协议由系统和游戏 Provider 自动确定。</div></div> : null}
      {steps[step] === "游戏配置" ? <ConfigurationRenderer manifest={manifest.data} onChange={setValues} values={values} /> : null}
      {steps[step] === "模组" ? <div className="mod-catalog">{manifest.data.modCatalog?.entries.map((mod) => { const direct = mod.modId in mods; const required = requiredMods.has(mod.modId) && !direct; return <label className="mod-row" key={mod.modId}><span><input checked={direct || required} disabled={required} onChange={(event) => setMods((current) => { const next = { ...current }; if (event.target.checked) next[mod.modId] = mod.versions[0].version; else delete next[mod.modId]; return next; })} type="checkbox" /><strong>{mod.displayName}</strong>{required ? <small>自动依赖</small> : null}</span><select disabled={!direct} onChange={(event) => setMods((current) => ({ ...current, [mod.modId]: event.target.value }))} value={mods[mod.modId] ?? mod.versions[0].version}>{mod.versions.map((version) => <option key={version.version}>{version.version}</option>)}</select></label>; })}</div> : null}
      {steps[step] === "确认" ? <div className="review-grid"><Review label="实例" value={name} /><Review label="游戏" value={`${manifest.data.displayName} ${manifest.data.gameVersions[0]}`} /><Review label="区域" value={regions.data?.find((item) => item.id === regionID)?.name ?? regionID} /><Review label="规格" value={`${cpuMilli / 1000} vCPU · ${memoryMiB / 1024} GB · ${diskGiB} GB`} /><Review label="Endpoint" value="部署时自动分配" /><Review label="可用余额" value={`¥${((wallet.data?.availableMinor ?? 0) / 100).toFixed(2)}`} /></div> : null}
      {error ? <ErrorNotice text={error} /> : null}
    </div><div className="wizard-footer">{step > 0 ? <Button onClick={() => setStep((value) => value - 1)} variant="secondary"><ChevronLeft size={16} />上一步</Button> : <span />}{step < steps.length - 1 ? <Button onClick={() => setStep((value) => value + 1)}>下一步<ChevronRight size={16} /></Button> : <Button disabled={deploy.isPending} onClick={() => deploy.mutate()}><Server size={16} />{deploy.isPending ? "提交中" : "创建并部署"}</Button>}</div>
  </section></>;
}

export function OperationPage({ workspaceSlug, operationId }: { workspaceSlug: string; operationId: string }) {
  const instanceID = useSearchParams().get("instance");
  const operation = useQuery({ queryKey: ["operation", operationId], queryFn: () => api<Operation>(`/operations/${operationId}`), refetchInterval: (query) => ["succeeded", "failed"].includes(query.state.data?.status ?? "") ? false : 1000 });
  if (!operation.data) return <Loading />;
  return <><PageHeader title="操作详情" actions={operation.data.status === "succeeded" && instanceID ? <Button asChild><Link href={`/w/${workspaceSlug}/instances/${instanceID}`}>查看实例<ChevronRight size={16} /></Link></Button> : undefined} /><section className="operation-panel"><div className="operation-meta"><div><span>操作</span><code>{operation.data.id}</code></div><div><span>类型</span><strong>{operation.data.kind}</strong></div><div><span>状态</span><strong className={operation.data.status === "succeeded" ? "success-text" : ""}>{operation.data.status}</strong></div></div><ol className="operation-steps">{operation.data.steps.map((item) => <li key={item.key}><span className={cn("step-icon", item.status === "succeeded" && "complete", item.status === "running" && "current")}>{item.status === "succeeded" ? <Check size={15} /> : item.status === "running" ? <LoaderCircle className="spin" size={15} /> : null}</span><div><strong>{item.label}</strong><small>{item.detail ?? item.status}</small></div></li>)}</ol></section></>;
}

export function InstanceDetailPage({ workspaceSlug, instanceId }: { workspaceSlug: string; instanceId: string }) {
  const router = useRouter(); const search = useSearchParams(); const client = useQueryClient(); const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug);
  const instance = useQuery({ queryKey: ["instance", workspace?.id, instanceId], enabled: Boolean(workspace), queryFn: () => api<Instance>(`/workspaces/${workspace!.id}/instances/${instanceId}`), refetchInterval: 3000 });
  const manifest = useQuery({ queryKey: ["manifest", workspace?.id, instance.data?.providerReleaseId], enabled: Boolean(workspace && instance.data), queryFn: () => api<ProviderManifest>(`/providers/releases/${instance.data!.providerReleaseId}/manifest?workspaceId=${workspace!.id}`) });
  const act = useMutation({ mutationFn: (action: string) => api<Operation>(`/workspaces/${workspace!.id}/instances/${instanceId}:${action}`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey(action) }, body: "{}" }), onSuccess: () => client.invalidateQueries({ queryKey: ["instance", workspace?.id, instanceId] }) });
  if (workspaceLoading || instance.isLoading || manifest.isLoading) return <Loading />;
  if (workspaceError || instance.error || manifest.error || !workspace || !instance.data || !manifest.data) return <ErrorNotice text="实例不存在或暂时不可用" />;
  const tab = search.get("tab") ?? "overview"; const capabilities = manifest.data.capabilities;
  const tabs = [{ id: "overview", label: "概览", icon: Server }, ...(capabilities.includes("console") ? [{ id: "console", label: "终端控制台", icon: TerminalSquare }] : []), ...(capabilities.includes("logs") ? [{ id: "logs", label: "实时日志", icon: FileText }] : []), ...(capabilities.includes("backup") ? [{ id: "backups", label: "备份", icon: Archive }] : []), ...(capabilities.includes("configuration") ? [{ id: "configuration", label: "配置", icon: Settings }] : [])];
  const endpoint = primaryEndpoint(instance.data);
  return <><section className="instance-header"><Link className="breadcrumb" href={`/w/${workspaceSlug}/instances`}><ChevronLeft size={15} />实例</Link><div className="instance-title-row"><div><h1>{instance.data.name}</h1><StateBadge state={instance.data.observedState} /><EndpointSummary instance={instance.data} /></div><div className="header-actions">{endpoint ? <button aria-label="复制地址" onClick={() => navigator.clipboard?.writeText(endpoint.displayAddress)} type="button"><Clipboard size={17} /></button> : null}<button aria-label="重启" disabled={act.isPending || instance.data.desiredState !== "running"} onClick={() => act.mutate("restart")} type="button"><RefreshCw size={17} /></button>{instance.data.desiredState === "stopped" ? <button aria-label="启动" disabled={act.isPending} onClick={() => act.mutate("start")} type="button"><Play size={17} /></button> : <button aria-label="停止" disabled={act.isPending} onClick={() => act.mutate("stop")} type="button"><Square size={16} /></button>}</div></div><nav className="detail-tabs">{tabs.map((item) => { const Icon = item.icon; return <button className={tab === item.id ? "active" : undefined} key={item.id} onClick={() => router.push(`?tab=${item.id}`)} type="button"><Icon size={16} />{item.label}</button>; })}</nav></section>
    {act.error ? <ErrorNotice text="操作未被接受" /> : null}
    {tab === "overview" ? <section className="detail-panel"><div className="compact-definition-grid"><Review label="游戏" value={`${manifest.data.displayName} ${instance.data.gameVersion}`} /><Review label="区域" value={instance.data.regionId} /><Review label="规格" value={`${instance.data.resourceSpec.cpuMilli / 1000} vCPU · ${instance.data.resourceSpec.memoryMiB / 1024} GB · ${instance.data.resourceSpec.diskGiB} GB`} /></div></section> : null}
    {tab === "console" ? <Console workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "logs" ? <Logs workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "backups" ? <Backups workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "configuration" ? <Configuration workspaceID={workspace.id} instance={instance.data} manifest={manifest.data} /> : null}
  </>;
}

function Console({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const [command, setCommand] = useState(""); const [accepted, setAccepted] = useState<string[]>([]);
  const mutation = useMutation({ mutationFn: () => api<Operation>(`/workspaces/${workspaceID}/instances/${instanceID}/console-commands`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("console") }, body: JSON.stringify({ command }) }), onSuccess: () => { setAccepted((items) => [...items, `> ${command}`, "命令已进入执行队列"]); setCommand(""); } });
  return <section className="terminal-panel"><pre>{accepted.length ? accepted.join("\n") : "控制台已连接。命令将发送到当前实例运行进程。"}</pre><form onSubmit={(event) => { event.preventDefault(); if (command.trim()) mutation.mutate(); }}><span>&gt;</span><input aria-label="控制台命令" onChange={(event) => setCommand(event.target.value)} value={command} /><button disabled={mutation.isPending} type="submit">发送</button></form></section>;
}

function Logs({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const logs = useQuery({ queryKey: ["logs", workspaceID, instanceID], queryFn: () => api<LogEntry[]>(`/workspaces/${workspaceID}/instances/${instanceID}/logs?limit=200`), refetchInterval: 3000 });
  return <section className="terminal-panel logs"><pre>{logs.data?.map((entry) => `${new Date(entry.observedAt).toLocaleTimeString()} [${entry.stream}] ${entry.message}`).join("\n") || "暂无日志"}</pre></section>;
}

function Backups({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const client = useQueryClient(); const query = useQuery({ queryKey: ["backups", workspaceID], queryFn: () => api<Backup[]>(`/workspaces/${workspaceID}/backups`), refetchInterval: 3000 });
  const create = useMutation({ mutationFn: () => api(`/workspaces/${workspaceID}/backups`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("backup") }, body: JSON.stringify({ logicalInstanceId: instanceID }) }), onSuccess: () => client.invalidateQueries({ queryKey: ["backups", workspaceID] }) });
  const restore = useMutation({ mutationFn: (id: string) => api(`/workspaces/${workspaceID}/backups/${id}:restore`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("restore") }, body: "{}" }) });
  const items = query.data?.filter((item) => item.logicalInstanceId === instanceID) ?? [];
  return <section className="detail-panel"><div className="panel-toolbar"><Button disabled={create.isPending} onClick={() => create.mutate()}><Plus size={16} />创建备份</Button></div>{items.map((backup) => <div className="backup-row" key={backup.id}><div><strong>{backup.id}</strong><span>{new Date(backup.createdAt).toLocaleString()} · {formatBytes(backup.sizeBytes ?? 0)} · {backup.status}</span></div><Button disabled={backup.status !== "completed" || restore.isPending} onClick={() => restore.mutate(backup.id)} variant="secondary"><RotateCcw size={15} />恢复</Button></div>)}{items.length === 0 ? <div className="compact-empty">暂无备份</div> : null}</section>;
}

function Configuration({ workspaceID, instance, manifest }: { workspaceID: string; instance: Instance; manifest: ProviderManifest }) {
  const [draftID, setDraftID] = useState(""); const [values, setValues] = useState<Values>({}); const [mods, setMods] = useState<Array<{ modId: string; version: string }>>([]); const [message, setMessage] = useState("");
  const revision = useQuery({ queryKey: ["revision", workspaceID, instance.id, instance.configurationRevisionId], queryFn: () => api<Revision>(`/workspaces/${workspaceID}/instances/${instance.id}/revisions/${instance.configurationRevisionId}`) });
  const selectedMods = useMemo(() => Object.fromEntries(mods.map((item) => [item.modId, item.version])), [mods]);
  const requiredMods = useMemo(() => resolveRequiredMods(manifest, selectedMods), [manifest, selectedMods]);
  useEffect(() => { if (revision.data) { setValues(revision.data.configuration); setMods(revision.data.modLock.filter((item) => item.direct).map((item) => ({ modId: item.modId, version: item.version }))); } }, [revision.data]);
  const save = useMutation({ mutationFn: async () => { let id = draftID; if (!id) { const draft = await api<{ id: string }>(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts`, { method: "POST", body: "{}" }); id = draft.id; setDraftID(id); } await api(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts/${id}`, { method: "PUT", body: JSON.stringify({ schemaVersion: manifest.schemaVersion, values, modSelections: mods }) }); return api(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts/${id}:apply`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("config") }, body: "{}" }); }, onSuccess: () => setMessage("配置已提交"), onError: () => setMessage("配置未通过校验") });
  if (!revision.data) return <Loading />;
  return <section className="detail-panel"><ConfigurationRenderer manifest={manifest} onChange={(next) => { setValues(next); setMessage(""); }} values={values} />{manifest.capabilities.includes("mods") && manifest.modCatalog ? <div className="schema-form"><section><h2>模组</h2>{manifest.modCatalog.entries.map((mod) => { const selected = mods.find((item) => item.modId === mod.modId); const required = requiredMods.has(mod.modId) && !selected; return <label className="mod-row" key={mod.modId}><span><input checked={Boolean(selected) || required} disabled={required} onChange={(event) => setMods(event.target.checked ? [...mods, { modId: mod.modId, version: mod.versions[0].version }] : mods.filter((item) => item.modId !== mod.modId))} type="checkbox" /><strong>{mod.displayName}</strong>{required ? <small>自动依赖</small> : null}</span></label>; })}</section></div> : null}<div className="panel-footer"><span className="muted-text">{message}</span><Button disabled={save.isPending} onClick={() => save.mutate()}><Save size={16} />保存并应用</Button></div></section>;
}

function resolveRequiredMods(manifest: ProviderManifest | undefined, selected: Record<string, string>) {
  const required = new Set<string>();
  const queue = Object.entries(selected);
  while (queue.length) {
    const [modId, version] = queue.shift()!;
    const entry = manifest?.modCatalog?.entries.find((item) => item.modId === modId);
    const dependencies = entry?.versions.find((item) => item.version === version)?.dependencies ?? [];
    for (const dependency of dependencies) {
      if (dependency.modId in selected || required.has(dependency.modId)) continue;
      required.add(dependency.modId);
      queue.push([dependency.modId, dependency.version]);
    }
  }
  return required;
}

function latestProviderSummaries(providers: ProviderSummary[]) {
  const latest = new Map<string, ProviderSummary>();
  for (const provider of providers) {
    const key = `${provider.gameKey}:${provider.gameVersions.join(",")}`;
    const current = latest.get(key);
    if (!current || Number(provider.releaseVersion) > Number(current.releaseVersion)) latest.set(key, provider);
  }
  return [...latest.values()];
}

export function BackupsPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug); const instances = useQuery({ queryKey: ["instances", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Instance[]>(`/workspaces/${workspace!.id}/instances`) }); const [instanceID, setInstanceID] = useState("");
  useEffect(() => { if (!instanceID && instances.data?.[0]) setInstanceID(instances.data[0].id); }, [instanceID, instances.data]);
  if (workspaceLoading || instances.isLoading) return <><PageHeader title="备份" /><Loading /></>;
  if (workspaceError || instances.error || !workspace) return <><PageHeader title="备份" /><ErrorNotice text="备份列表暂时不可用" /></>;
  if (instances.data?.length === 0) return <><PageHeader title="备份" /><section className="detail-panel compact-empty">创建实例后可在这里管理备份</section></>;
  if (!instanceID) return <><PageHeader title="备份" /><Loading /></>;
  return <><PageHeader title="备份" actions={<select onChange={(event) => setInstanceID(event.target.value)} value={instanceID}>{instances.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>} /><Backups workspaceID={workspace.id} instanceID={instanceID} /></>;
}

export function BillingPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug); const wallet = useQuery({ queryKey: ["wallet", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Wallet>(`/workspaces/${workspace!.id}/wallet`) });
  if (workspaceLoading || wallet.isLoading) return <Loading />;
  if (workspaceError || wallet.error || !workspace || !wallet.data) return <ErrorNotice text="账单数据暂时不可用" />;
  return <><PageHeader title="账单" /><section className="billing-strip"><div><span>可用余额</span><strong>¥{(wallet.data.availableMinor / 100).toFixed(2)}</strong></div><div><span>测试额度</span><strong>¥{(wallet.data.promotionalMinor / 100).toFixed(2)}</strong></div><div><span>计费方式</span><strong>按小时 · 资源规格</strong></div></section></>;
}

function ConfigurationRenderer({ manifest, values, onChange }: { manifest: ProviderManifest; values: Values; onChange: (values: Values) => void }) {
  return <div className="schema-form">{[...manifest.uiSchema.sections].sort((a, b) => a.order - b.order).map((section) => <section key={section.id}><h2>{section.title}</h2><div className="form-grid">{Object.entries(manifest.configurationSchema.properties).filter(([key]) => { const ui = manifest.uiSchema.fields[key]; return ui?.section === section.id && (!ui.visibleWhen || values[ui.visibleWhen.field] === ui.visibleWhen.equals); }).sort(([a], [b]) => manifest.uiSchema.fields[a].order - manifest.uiSchema.fields[b].order).map(([key, field]) => <ConfigurationInput field={field} key={key} onChange={(value) => onChange({ ...values, [key]: value })} value={values[key]} />)}</div></section>)}</div>;
}

function ConfigurationInput({ field, value, onChange }: { field: ConfigurationField; value: unknown; onChange: (value: unknown) => void }) {
  if (field.type === "boolean") return <label className="switch-row"><span><strong>{field.title}</strong>{field.description ? <small>{field.description}</small> : null}</span><input checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)} type="checkbox" /></label>;
  if (field.type === "enum") return <Field label={field.title}><select onChange={(event) => onChange(event.target.value)} value={String(value ?? "")}>{field.enum?.map((option) => <option key={String(option)} value={String(option)}>{String(option)}</option>)}</select></Field>;
  if (field.type === "integer" || field.type === "number") return <Field label={field.title}><input max={field.maximum} min={field.minimum} onChange={(event) => onChange(Number(event.target.value))} type="number" value={Number(value ?? 0)} /></Field>;
  if (field.type === "string-list") return <Field label={field.title}><textarea onChange={(event) => onChange(event.target.value.split("\n"))} rows={3} value={Array.isArray(value) ? value.join("\n") : ""} /></Field>;
  return <Field label={field.title}><input onChange={(event) => onChange(event.target.value)} type={field.type === "secret" ? "password" : "text"} value={String(value ?? "")} /></Field>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="field"><span>{label}</span>{children}</label>; }
function NumberField({ label, range, value, set }: { label: string; range: { minimum: number; maximum: number; step: number }; value: number; set: (value: number) => void }) { return <Field label={label}><input max={range.maximum} min={range.minimum} onChange={(event) => set(Number(event.target.value))} step={range.step} type="number" value={value} /></Field>; }
function Review({ label, value }: { label: string; value: string }) { return <div className="review-item"><span>{label}</span><strong>{value}</strong></div>; }
function defaultValues(manifest: ProviderManifest) { return Object.fromEntries(Object.entries(manifest.configurationSchema.properties).map(([key, field]) => [key, field.default ?? (field.type === "boolean" ? false : field.type === "integer" || field.type === "number" ? 0 : field.type === "string-list" ? [] : "")])); }
function clampStep(value: number, range: { minimum: number; maximum: number; step: number }) { const clamped = Math.max(range.minimum, Math.min(range.maximum, value)); return range.minimum + Math.round((clamped - range.minimum) / range.step) * range.step; }
function formatBytes(value: number) { return value < 1024 * 1024 ? `${Math.round(value / 1024)} KB` : `${(value / 1024 / 1024).toFixed(1)} MB`; }
