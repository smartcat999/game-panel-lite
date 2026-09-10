import { AppShell } from "@/components/app-shell";
import { RegionOperations } from "@/features/region/region-operations";

export default async function RegionStorage({ params }: { params: Promise<{ regionId: string }> }) {
  const { regionId } = await params;
  return <AppShell area="region" scope={regionId}><RegionOperations regionId={regionId} resource="storage" /></AppShell>;
}
