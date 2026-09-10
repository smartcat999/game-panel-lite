import { AppShell } from "@/components/app-shell";
import { CreateInstanceForm } from "@/features/instances/create-instance-form";

export default async function NewInstancePage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><CreateInstanceForm workspaceSlug={workspaceSlug} /></AppShell>;
}
