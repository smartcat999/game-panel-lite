import { AppShell } from "@/components/app-shell";

export default async function RegionOperations({ params }: { params: Promise<{ regionId: string }> }) {
  const { regionId } = await params;
  return <AppShell area="region" scope={regionId} />;
}
