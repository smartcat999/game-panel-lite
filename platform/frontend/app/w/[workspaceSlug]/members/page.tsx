import { AppShell } from "@/components/app-shell";
import { MemberManagement } from "@/features/members/member-management";

export default async function MembersPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><MemberManagement workspaceSlug={workspaceSlug} /></AppShell>;
}
