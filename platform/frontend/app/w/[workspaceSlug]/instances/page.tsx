import { AppShell } from "@/components/app-shell";
import { InstanceListPage } from "@/features/prototype/workspace-pages";

export default async function InstancesPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><InstanceListPage workspaceSlug={workspaceSlug} /></AppShell>;
}
