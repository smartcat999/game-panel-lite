import { AppShell } from "@/components/app-shell";
import { MembersPage as Members } from "@/features/prototype/workspace-pages";

export default async function MembersPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><Members /></AppShell>;
}
