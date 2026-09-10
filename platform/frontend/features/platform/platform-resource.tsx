"use client";

import { useQuery } from "@tanstack/react-query";

import { usePreferences } from "@/components/providers";
import { controlPlane } from "@/lib/control-plane";
import type { MessageKey } from "@/lib/messages";
import { queryKeys } from "@/lib/query-keys";

type Resource = "workspaces" | "plans" | "orders" | "instances" | "regions";
type DisplayRecord = { id: string; label: string; state?: string; stateKey?: MessageKey; amountMinor?: number; currency?: string };

const titles: Record<Resource, MessageKey> = {
  workspaces: "platform.resource.workspaces", plans: "platform.resource.plans", orders: "platform.resource.orders",
  instances: "platform.resource.instances", regions: "platform.resource.regions",
};

export function PlatformResource({ resource }: { resource: Resource }) {
  const { locale, t } = usePreferences();
  const query = useQuery<DisplayRecord[]>({
    queryKey: queryKeys.platform(resource),
    queryFn: async () => {
      switch (resource) {
        case "workspaces": return (await controlPlane.platformWorkspaces()).map((item) => ({ id: item.id, label: item.name, state: item.slug }));
        case "plans": return (await controlPlane.platformPlans()).map((item) => ({ id: item.id, label: item.name, amountMinor: item.priceMinor, currency: item.currency }));
        case "orders": return (await controlPlane.platformOrders()).map((item) => ({ id: item.id, label: item.logicalInstanceId, stateKey: `orders.status.${item.status}` as MessageKey }));
        case "instances": return (await controlPlane.platformInstances()).map((item) => ({ id: item.id, label: item.name, stateKey: (item.stale ? "instances.status.stale" : `instances.status.${item.deploymentState}`) as MessageKey }));
        case "regions": return (await controlPlane.platformRegions()).map((item) => ({ id: item.id, label: item.name, stateKey: `platform.state.${item.operationalState}` as MessageKey }));
      }
    },
  });
  const records = query.data;
  return <section className="resource-section" aria-labelledby="platform-resource-heading"><div className="section-intro"><h2 id="platform-resource-heading">{t(titles[resource])}</h2><p>{t("platform.resource.description")}</p></div>
    {query.isLoading ? <div className="table-skeleton"><i /><i /><i /></div> : null}
    {records?.length === 0 ? <div className="inline-state">{t("platform.resource.empty")}</div> : null}
    {records && records.length > 0 ? <div className="data-table-wrap"><table><thead><tr><th>ID</th><th>{t(titles[resource])}</th><th>{t("instances.state")}</th></tr></thead><tbody>{records.map((record) => <tr key={record.id}><td><code>{record.id}</code></td><td>{record.label}</td><td>{record.amountMinor !== undefined && record.currency ? new Intl.NumberFormat(locale, { style: "currency", currency: record.currency }).format(record.amountMinor / 100) : record.stateKey ? t(record.stateKey) : record.state}</td></tr>)}</tbody></table></div> : null}
  </section>;
}
