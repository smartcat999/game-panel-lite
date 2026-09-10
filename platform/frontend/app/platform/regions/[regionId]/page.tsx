import { redirect } from "next/navigation";

export default async function RegionOverviewPage({ params }: { params: Promise<{ regionId: string }> }) {
  const { regionId } = await params;
  redirect(`/platform/regions/${regionId}/nodes`);
}
