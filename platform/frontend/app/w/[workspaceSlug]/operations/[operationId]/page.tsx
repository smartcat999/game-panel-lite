import { AppShell } from "@/components/app-shell";
import { OperationPage } from "@/features/production/workspace-pages";

export default async function Operation({ params }: { params: Promise<{ workspaceSlug: string; operationId: string }> }) {
  const { workspaceSlug, operationId } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><OperationPage operationId={operationId} workspaceSlug={workspaceSlug} /></AppShell>;
}
