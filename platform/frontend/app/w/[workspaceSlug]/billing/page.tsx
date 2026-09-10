import { AppShell } from "@/components/app-shell";
import { OrderList } from "@/features/orders/order-list";

export default async function BillingPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = await params;
  return <AppShell area="workspace" workspaceSlug={workspaceSlug}><OrderList workspaceSlug={workspaceSlug} /></AppShell>;
}
