import { AppShell } from "@/components/app-shell";
import { InstanceList } from "@/features/instances/instance-list";

export default async function InstancesPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><InstanceList workspaceSlug={workspaceSlug} /></AppShell>;
}
