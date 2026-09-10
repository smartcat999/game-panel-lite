import { AppShell } from "@/components/app-shell";
import { BackupsPage as Backups } from "@/features/prototype/workspace-pages";

export default async function BackupsPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><Backups /></AppShell>;
}
