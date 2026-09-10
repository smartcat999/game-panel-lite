"use client";

import { useQuery } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import Link from "next/link";

import { usePreferences } from "@/components/providers";
import { Button } from "@/components/ui/button";
import { ControlPlaneError, controlPlane } from "@/lib/control-plane";
import { queryKeys } from "@/lib/query-keys";
import { InstanceStatus } from "./instance-status";
import { useWorkspace } from "../workspaces/use-workspace";

export function InstanceList({ workspaceSlug }: { workspaceSlug: string }) {
  const { locale, t } = usePreferences();
  const { workspace, isLoading: workspaceLoading, isError: workspaceError } = useWorkspace(workspaceSlug);
  const instances = useQuery({
    queryKey: queryKeys.workspaceInstances(workspace?.id ?? "pending"),
    queryFn: () => controlPlane.workspaceInstances(workspace!.id),
    enabled: Boolean(workspace),
  });
  const forbidden = instances.error instanceof ControlPlaneError && instances.error.status === 403;
  const date = new Intl.DateTimeFormat(locale, { dateStyle: "medium" });

  return (
    <section className="resource-section" aria-labelledby="instances-heading">
      <div className="resource-toolbar">
        <div className="section-intro"><h2 id="instances-heading">{t("instances.heading")}</h2><p>{t("instances.description")}</p></div>
        <Button asChild><Link href={`/w/${workspaceSlug}/instances/new`}><Plus size={16} aria-hidden="true" />{t("instances.create")}</Link></Button>
      </div>
      {workspaceLoading || instances.isLoading ? <div className="table-skeleton" aria-label={t("instances.loading")}><i /><i /><i /></div> : null}
      {forbidden ? <div className="inline-state state-forbidden">{t("instances.forbidden")}</div> : null}
      {(workspaceError || instances.isError) && !forbidden ? <div className="inline-state">{t("instances.unavailable")}</div> : null}
      {instances.data?.length === 0 ? <div className="inline-state">{t("instances.empty")}</div> : null}
      {instances.data && instances.data.length > 0 ? (
        <div className="data-table-wrap"><table><thead><tr><th>{t("instances.name")}</th><th>{t("instances.game")}</th><th>{t("instances.state")}</th><th>{t("instances.created")}</th></tr></thead>
          <tbody>{instances.data.map((instance) => <tr key={instance.id}><td><Link href={`/w/${workspaceSlug}/instances/${instance.id}`}>{instance.name}</Link><code>{instance.id}</code></td><td>{instance.gameKey}</td><td><InstanceStatus state={instance.deploymentState} stale={instance.stale} t={t} /></td><td>{date.format(new Date(instance.createdAt))}</td></tr>)}</tbody></table></div>
      ) : null}
    </section>
  );
}
