import { AppShell } from "@/components/app-shell";
import { RegionPage } from "@/features/prototype/platform-pages";

export default async function RegionTasks({ params }: { params: Promise<{ regionId: string }> }) {
  const { regionId } = await params;
  return <AppShell area="region" scope={regionId}><RegionPage section="tasks" /></AppShell>;
}
