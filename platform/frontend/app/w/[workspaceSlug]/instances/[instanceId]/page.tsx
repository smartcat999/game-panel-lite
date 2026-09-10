import { AppShell } from "@/components/app-shell";
import { InstanceDetail } from "@/features/instances/instance-detail";

export default async function InstancePage({ params }: { params: Promise<{ workspaceSlug: string; instanceId: string }> }) {
  const { workspaceSlug, instanceId } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><InstanceDetail workspaceSlug={workspaceSlug} instanceId={instanceId} /></AppShell>;
}
