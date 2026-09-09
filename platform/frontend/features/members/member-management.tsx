"use client";

import { useQuery } from "@tanstack/react-query";

import { usePreferences } from "@/components/providers";
import { controlPlane, type WorkspaceMember } from "@/lib/control-plane";
import type { MessageKey } from "@/lib/messages";
import { queryKeys } from "@/lib/query-keys";

const roleLabels: Record<WorkspaceMember["role"], MessageKey> = {
  owner: "members.owner",
  administrator: "members.administrator",
  operator: "members.operator",
  billing: "members.billing",
  viewer: "members.viewer",
};

export function MemberManagement({ workspaceSlug }: { workspaceSlug: string }) {
  const { t } = usePreferences();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: controlPlane.workspaces });
  const workspace = workspaces.data?.find((item) => item.slug === workspaceSlug);
  const members = useQuery({
    queryKey: queryKeys.workspaceMembers(workspace?.id ?? "pending"),
    queryFn: () => controlPlane.workspaceMembers(workspace!.id),
    enabled: Boolean(workspace),
  });

  return (
    <section className="member-section" aria-labelledby="members-heading">
      <div className="section-intro">
        <h2 id="members-heading">{t("members.heading")}</h2>
        <p>{t("members.description")}</p>
      </div>
      {workspaces.isLoading || members.isLoading ? <p className="data-message">{t("members.loading")}</p> : null}
      {workspaces.isError || members.isError ? <p className="data-message">{t("members.unavailable")}</p> : null}
      <div className="member-table-wrap">
        <table>
          <thead><tr><th>{t("members.person")}</th><th>{t("members.role")}</th></tr></thead>
          <tbody>{members.data?.map((member) => <tr key={member.membershipId}><td><strong>{member.user.displayName}</strong><span>{member.user.email}</span></td><td>{t(roleLabels[member.role])}</td></tr>)}</tbody>
        </table>
      </div>
    </section>
  );
}
