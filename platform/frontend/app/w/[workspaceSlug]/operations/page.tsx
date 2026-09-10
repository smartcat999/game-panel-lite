import { AppShell } from "@/components/app-shell";
import { OperationsPage } from "@/features/prototype/workspace-pages";

export default async function Operations({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><OperationsPage workspaceSlug={workspaceSlug} /></AppShell>;
}
