import { AppShell } from "@/components/app-shell";
import { SettingsPage } from "@/features/prototype/workspace-pages";

export default async function Settings({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><SettingsPage /></AppShell>;
}
