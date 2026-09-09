import { AppShell } from "@/components/app-shell";

export default async function WorkspaceConsole({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug} />;
}
