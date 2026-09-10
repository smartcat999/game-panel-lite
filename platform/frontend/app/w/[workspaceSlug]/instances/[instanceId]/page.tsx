import { AppShell } from "@/components/app-shell";
import { InstanceDetailPage } from "@/features/production/workspace-pages";

export default async function InstancePage({ params }: { params: Promise<{ workspaceSlug: string; instanceId: string }> }) {
  const { workspaceSlug, instanceId } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><InstanceDetailPage workspaceSlug={workspaceSlug} instanceId={instanceId} /></AppShell>;
}
