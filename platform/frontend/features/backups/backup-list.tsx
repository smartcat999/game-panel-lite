"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, RotateCcw } from "lucide-react";
import { useState } from "react";

import { usePreferences } from "@/components/providers";
import { Button } from "@/components/ui/button";
import { useWorkspace } from "@/features/workspaces/use-workspace";
import { controlPlane, type BackupRequest } from "@/lib/control-plane";
import type { MessageKey } from "@/lib/messages";
import { queryKeys } from "@/lib/query-keys";

export function BackupList({ workspaceSlug }: { workspaceSlug: string }) {
  const { locale, t } = usePreferences();
  const { workspace } = useWorkspace(workspaceSlug);
  const queryClient = useQueryClient();
  const workspaceID = workspace?.id ?? "pending";
  const instances = useQuery({ queryKey: queryKeys.workspaceInstances(workspaceID), queryFn: () => controlPlane.workspaceInstances(workspaceID), enabled: Boolean(workspace) });
  const backups = useQuery({ queryKey: queryKeys.workspaceBackups(workspaceID), queryFn: () => controlPlane.workspaceBackups(workspaceID), enabled: Boolean(workspace) });
  const [instanceID, setInstanceID] = useState("");
  const create = useMutation({ mutationFn: (input: { kind: "backup" | "restore"; source?: BackupRequest }) => {
    const selectedID = input.source?.logicalInstanceId ?? instanceID;
    return controlPlane.createBackup(workspaceID, { logicalInstanceId: selectedID, regionId: input.source?.regionId ?? "reg_asia_east", kind: input.kind, sourceBackupRequestId: input.source?.id }, `backup_${input.kind}_${Date.now()}`);
  }, onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.workspaceBackups(workspaceID) }) });
  const date = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" });
  return <section className="resource-section" aria-labelledby="backups-heading">
    <div className="section-intro"><h2 id="backups-heading">{t("backups.heading")}</h2><p>{t("backups.description")}</p></div>
    <form className="backup-toolbar" onSubmit={event => { event.preventDefault(); create.mutate({ kind: "backup" }); }}>
      <label><span>{t("backups.instance")}</span><select required value={instanceID} onChange={event => setInstanceID(event.target.value)}><option value="">{t("backups.chooseInstance")}</option>{instances.data?.filter(item => item.billingState === "active").map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <Button disabled={create.isPending || !instanceID} type="submit"><Archive size={16} />{t("backups.create")}</Button>
    </form>
    <div className="ownership-note"><strong>{t("backups.directTitle")}</strong><span>{t("backups.directDescription")}</span></div>
    {create.isError ? <p role="alert" className="form-error">{t("backups.failed")}</p> : null}
    {backups.isLoading ? <div className="table-skeleton"><i /><i /></div> : null}
    {backups.data?.length === 0 ? <div className="empty-inline"><strong>{t("backups.empty")}</strong></div> : null}
    {backups.data?.length ? <div className="data-table-wrap"><table><thead><tr><th>{t("backups.request")}</th><th>{t("backups.instance")}</th><th>{t("backups.kind")}</th><th>{t("backups.status")}</th><th>{t("backups.created")}</th><th>{t("backups.action")}</th></tr></thead><tbody>{backups.data.map(item => <tr key={item.id}><td><code>{item.id}</code></td><td><code>{item.logicalInstanceId}</code></td><td>{t(`backups.kind.${item.kind}` as MessageKey)}</td><td><span className={`status-badge status-${item.status}`}>{t(`backups.status.${item.status}` as MessageKey)}</span></td><td>{date.format(new Date(item.createdAt))}</td><td>{item.kind === "backup" && item.status === "completed" ? <Button variant="quiet" disabled={create.isPending} onClick={() => create.mutate({ kind: "restore", source: item })}><RotateCcw size={15} />{t("backups.restore")}</Button> : "—"}</td></tr>)}</tbody></table></div> : null}
  </section>;
}
