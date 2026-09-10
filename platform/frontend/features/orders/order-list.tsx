"use client";

import { useQuery } from "@tanstack/react-query";

import { usePreferences } from "@/components/providers";
import { controlPlane } from "@/lib/control-plane";
import { queryKeys } from "@/lib/query-keys";
import { useWorkspace } from "@/features/workspaces/use-workspace";

export function OrderList({ workspaceSlug }: { workspaceSlug: string }) {
  const { locale, t } = usePreferences();
  const { workspace } = useWorkspace(workspaceSlug);
  const orders = useQuery({ queryKey: queryKeys.workspaceOrders(workspace?.id ?? "pending"), queryFn: () => controlPlane.workspaceOrders(workspace!.id), enabled: Boolean(workspace) });
  const date = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" });
  return <section className="resource-section" aria-labelledby="orders-heading"><div className="section-intro"><h2 id="orders-heading">{t("orders.heading")}</h2><p>{t("orders.description")}</p></div>
    {orders.isLoading ? <div className="table-skeleton"><i /><i /></div> : null}
    {orders.data ? <div className="data-table-wrap"><table><thead><tr><th>{t("orders.order")}</th><th>{t("orders.instance")}</th><th>{t("orders.status")}</th><th>{t("orders.expires")}</th></tr></thead><tbody>{orders.data.map((order) => <tr key={order.id}><td><code>{order.id}</code></td><td><code>{order.logicalInstanceId}</code></td><td><span className={`status-badge status-${order.status}`}>{t(`orders.status.${order.status}`)}</span></td><td>{date.format(new Date(order.expiresAt))}</td></tr>)}</tbody></table></div> : null}
  </section>;
}
