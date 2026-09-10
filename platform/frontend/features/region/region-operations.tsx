"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";

import { usePreferences } from "@/components/providers";
import { Button } from "@/components/ui/button";
import type { MessageKey } from "@/lib/messages";
import { queryKeys } from "@/lib/query-keys";
import { regionControl, type RegionalDeployment } from "@/lib/region-control";

export type RegionResource = "overview" | "nodes" | "deployments" | "tasks" | "capacity" | "storage" | "monitoring";

export function RegionOperations({ regionId, resource }: { regionId: string; resource: RegionResource }) {
  if (resource === "overview") return <Overview regionId={regionId} />;
  if (resource === "nodes") return <Nodes regionId={regionId} />;
  if (resource === "deployments") return <Deployments regionId={regionId} />;
  if (resource === "tasks") return <Tasks regionId={regionId} />;
  if (resource === "capacity") return <Capacity regionId={regionId} />;
  return <StorageMonitoring regionId={regionId} resource={resource} />;
}

function StorageMonitoring({ regionId, resource }: { regionId: string; resource: "storage" | "monitoring" }) {
  const { t } = usePreferences();
  const query = useQuery<Record<string, boolean | number>>({ queryKey: queryKeys.region(regionId, resource), queryFn: async () => resource === "storage" ? regionControl.storage(regionId) : regionControl.monitoring(regionId) });
  return <Section title={t(`region.resource.${resource}` as MessageKey)} description={t(`region.resource.${resource}Description` as MessageKey)}>{query.data ? <dl className="ops-definition">{Object.entries(query.data).map(([key, value]) => <div key={key}><dt>{t(`region.signal.${key}` as MessageKey)}</dt><dd>{typeof value === "boolean" ? t(value ? "common.yes" : "common.no") : value}</dd></div>)}</dl> : <Loading />}</Section>;
}

function Overview({ regionId }: { regionId: string }) {
  const { t } = usePreferences();
  const query = useQuery({ queryKey: queryKeys.region(regionId, "overview"), queryFn: () => regionControl.overview(regionId) });
  const data = query.data;
  return <Section title={t("region.resource.overview")} description={t("region.resource.overviewDescription")}>{data ? <div className="metric-strip"><Metric label={t("region.metric.readyNodes")} value={data.readyNodes} /><Metric label={t("region.metric.deployments")} value={data.deploymentCount} /><Metric label={t("region.metric.pendingTasks")} value={data.pendingTasks} /><Metric label={t("region.metric.unschedulable")} value={data.unschedulableCount} /></div> : <Loading />}</Section>;
}

function Nodes({ regionId }: { regionId: string }) {
  const { t } = usePreferences();
  const query = useQuery({ queryKey: queryKeys.region(regionId, "nodes"), queryFn: () => regionControl.nodes(regionId) });
  return <Section title={t("region.resource.nodes")} description={t("region.resource.nodesDescription")}>{query.data ? <div className="data-table-wrap"><table><thead><tr><th>{t("region.column.node")}</th><th>{t("instances.state")}</th><th>{t("region.column.games")}</th><th>{t("region.column.capacity")}</th></tr></thead><tbody>{query.data.map(node => <tr key={node.id}><td><strong>{node.name}</strong><code>{node.id}</code></td><td><span className={`status-badge status-${node.state}`}>{t(`region.state.${node.state}` as MessageKey)}</span></td><td>{node.games.join(", ")}</td><td>{node.reservedCpu}/{node.cpuCapacity} CPU · {node.reservedMemoryMb}/{node.memoryCapacityMb} MB</td></tr>)}</tbody></table></div> : <Loading />}</Section>;
}

function Deployments({ regionId }: { regionId: string }) {
  const { t } = usePreferences();
  const query = useQuery({ queryKey: queryKeys.region(regionId, "deployments"), queryFn: () => regionControl.deployments(regionId) });
  return <Section title={t("region.resource.deployments")} description={t("region.resource.deploymentsDescription")}><div className="ownership-note"><strong>{t("region.ownership.title")}</strong><span>{t("region.ownership.description")}</span></div>{query.data ? <div className="deployment-list">{query.data.map(item => <DeploymentRow key={item.id} regionId={regionId} deployment={item} />)}</div> : <Loading />}</Section>;
}

function DeploymentRow({ regionId, deployment }: { regionId: string; deployment: RegionalDeployment }) {
  const { t } = usePreferences();
  const queryClient = useQueryClient();
  const [nodeId, setNodeId] = useState(deployment.nodeId ?? "");
  const [reason, setReason] = useState("");
  const mutation = useMutation({ mutationFn: () => regionControl.overridePlacement(regionId, deployment.id, nodeId, reason), onSuccess: () => { setReason(""); queryClient.invalidateQueries({ queryKey: queryKeys.region(regionId, "deployments") }); } });
  const placement = deployment.nodeId || (deployment.unschedulableReason ? t(`region.reason.${deployment.unschedulableReason}` as MessageKey) : t("region.state.pending"));
  return <article className="deployment-row"><div className="deployment-context"><div><span>{t("region.ownership.regional")}</span><strong>{deployment.id}</strong><small>{placement}</small></div><div><span>{t("region.ownership.global")}</span><Link href={`/w/${deployment.workspaceId === "ws_northstar" ? "northstar" : "ember"}/instances/${deployment.logicalInstanceId}`}>{deployment.logicalInstanceId}</Link><small>{deployment.workspaceId}</small></div><span className={`status-badge status-${deployment.observedState}`}>{t(`region.state.${deployment.observedState}` as MessageKey)}</span></div><form className="override-form" onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><label><span>{t("region.override.node")}</span><input required value={nodeId} onChange={event => setNodeId(event.target.value)} /></label><label><span>{t("region.override.reason")}</span><input required value={reason} onChange={event => setReason(event.target.value)} /></label><Button disabled={mutation.isPending} type="submit">{t("region.override.submit")}</Button>{mutation.isError ? <small role="alert">{t("region.override.failed")}</small> : null}</form></article>;
}

function Tasks({ regionId }: { regionId: string }) { const { t } = usePreferences(); const query = useQuery({ queryKey: queryKeys.region(regionId, "tasks"), queryFn: () => regionControl.tasks(regionId) }); return <Section title={t("region.resource.tasks")} description={t("region.resource.tasksDescription")}>{query.data ? <div className="data-table-wrap"><table><thead><tr><th>ID</th><th>{t("region.column.deployment")}</th><th>{t("region.column.kind")}</th><th>{t("instances.state")}</th></tr></thead><tbody>{query.data.map(task => <tr key={task.id}><td><code>{task.id}</code></td><td><code>{task.regionalDeploymentId}</code></td><td>{t(`region.task.${task.kind}` as MessageKey)}</td><td>{t(`region.task.${task.status}` as MessageKey)}</td></tr>)}</tbody></table></div> : <Loading />}</Section>; }

function Capacity({ regionId }: { regionId: string }) { const { t } = usePreferences(); const query = useQuery({ queryKey: queryKeys.region(regionId, "capacity"), queryFn: () => regionControl.capacity(regionId) }); const data = query.data; return <Section title={t("region.resource.capacity")} description={t("region.resource.capacityDescription")}>{data ? <div className="capacity-lines"><CapacityLine label="CPU" used={data.cpuReserved} total={data.cpuCapacity} /><CapacityLine label={t("region.capacity.memory")} used={data.memoryReservedMb} total={data.memoryCapacityMb} /></div> : <Loading />}</Section>; }
function CapacityLine({ label, used, total }: { label: string; used: number; total: number }) { return <div><div><strong>{label}</strong><span>{used} / {total}</span></div><progress max={total} value={used} /></div>; }
function Metric({ label, value }: { label: string; value: number }) { return <div><strong>{value}</strong><span>{label}</span></div>; }
function Section({ title, description, children }: { title: string; description: string; children: React.ReactNode }) { return <section className="resource-section"><div className="section-intro"><h2>{title}</h2><p>{description}</p></div>{children}</section>; }
function Loading() { const { t } = usePreferences(); return <div className="table-skeleton" aria-label={t("common.loading")}><i /><i /><i /></div>; }
