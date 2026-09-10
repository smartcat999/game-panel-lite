import { AppShell } from "@/components/app-shell";
import { CreateInstancePage } from "@/features/production/workspace-pages";

export default async function NewInstancePage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><CreateInstancePage workspaceSlug={workspaceSlug} /></AppShell>;
}
