import { AppShell } from "@/components/app-shell";
import { InstanceListPage } from "@/features/production/workspace-pages";

export default async function WorkspaceConsole({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><InstanceListPage workspaceSlug={workspaceSlug} /></AppShell>;
}
