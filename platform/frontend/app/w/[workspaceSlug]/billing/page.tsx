import { AppShell } from "@/components/app-shell";
import { BillingPage as Billing } from "@/features/production/workspace-pages";

export default async function BillingPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><Billing workspaceSlug={workspaceSlug} /></AppShell>;
}
