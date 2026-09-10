"use client";

import { useQuery } from "@tanstack/react-query";

import { usePreferences } from "@/components/providers";
import { controlPlane } from "@/lib/control-plane";
import { queryKeys } from "@/lib/query-keys";
import { useWorkspace } from "@/features/workspaces/use-workspace";
import { InstanceStatus } from "./instance-status";

export function InstanceDetail({ workspaceSlug, instanceId }: { workspaceSlug: string; instanceId: string }) {
  const { t } = usePreferences();
  const { workspace } = useWorkspace(workspaceSlug);
  const detail = useQuery({ queryKey: queryKeys.instance(workspace?.id ?? "pending", instanceId), queryFn: () => controlPlane.instance(workspace!.id, instanceId), enabled: Boolean(workspace) });
  if (detail.isLoading) return <div className="table-skeleton"><i /><i /><i /></div>;
  if (!detail.data) return <div className="inline-state">{t("instances.unavailable")}</div>;
  const { instance, revision, placement } = detail.data;
  return <section className="detail-section"><div className="detail-title"><div><h2>{instance.name}</h2><code>{instance.id}</code></div><InstanceStatus state={instance.deploymentState} stale={instance.stale} t={t} /></div>
    {instance.billingState === "pending_payment" ? <div className="billing-notice">{t("detail.orderPending")}</div> : null}
    <dl className="detail-list"><div><dt>{t("instances.game")}</dt><dd>{instance.gameKey}</dd></div><div><dt>{t("detail.version")}</dt><dd>{revision.gameVersion}</dd></div><div><dt>{t("detail.region")}</dt><dd><code>{placement.regionId}</code></dd></div><div><dt>{t("detail.configuration")}</dt><dd><code>{revision.id}</code></dd></div></dl>
  </section>;
}
