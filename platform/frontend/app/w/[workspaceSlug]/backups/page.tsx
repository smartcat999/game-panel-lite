import { AppShell } from "@/components/app-shell";
import { BackupList } from "@/features/backups/backup-list";

export default async function BackupsPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><BackupList workspaceSlug={workspaceSlug} /></AppShell>;
}
