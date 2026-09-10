"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { usePreferences } from "@/components/providers";
import { Button } from "@/components/ui/button";
import { controlPlane } from "@/lib/control-plane";
import { queryKeys } from "@/lib/query-keys";
import { useWorkspace } from "@/features/workspaces/use-workspace";

export function CreateInstanceForm({ workspaceSlug }: { workspaceSlug: string }) {
  const { locale, t } = usePreferences();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { workspace } = useWorkspace(workspaceSlug);
  const plans = useQuery({ queryKey: queryKeys.plans, queryFn: controlPlane.plans });
  const regions = useQuery({ queryKey: queryKeys.regions, queryFn: controlPlane.regions });
  const [name, setName] = useState("");
  const [planVersionId, setPlanVersionId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [gameVersion, setGameVersion] = useState("1.4.5.6");
  const checkout = useMutation({
    mutationFn: () => controlPlane.createInstance({
      workspaceId: workspace!.id, planVersionId, regionId, name, gameKey: "terraria", gameVersion,
      configuration: { maxPlayers: 8 },
    }, crypto.randomUUID()),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workspace(workspace!.id) });
      router.push(`/w/${workspaceSlug}/instances/${result.instance.id}`);
    },
  });
  const selectedPlan = plans.data?.find((plan) => plan.id === planVersionId);
  const availableRegions = regions.data?.filter((region) => region.available && (!selectedPlan || selectedPlan.regionIds.includes(region.id)));

  return <section className="form-section" aria-labelledby="create-heading"><div className="section-intro"><h2 id="create-heading">{t("create.title")}</h2><p>{t("create.description")}</p></div>
    <form onSubmit={(event) => { event.preventDefault(); checkout.mutate(); }}>
      <div className="form-field"><label htmlFor="instance-name">{t("create.name")}</label><input id="instance-name" required maxLength={80} value={name} onChange={(event) => setName(event.target.value)} /></div>
      <div className="form-field"><label htmlFor="instance-plan">{t("create.plan")}</label><select id="instance-plan" required value={planVersionId} onChange={(event) => { setPlanVersionId(event.target.value); setRegionId(""); }}><option value="" />{plans.data?.map((plan) => <option key={plan.id} value={plan.id}>{plan.name} · {(plan.priceMinor / 100).toLocaleString(locale, { style: "currency", currency: plan.currency })}</option>)}</select></div>
      <div className="form-field"><label htmlFor="instance-region">{t("create.region")}</label><select id="instance-region" required value={regionId} onChange={(event) => setRegionId(event.target.value)}><option value="" />{availableRegions?.map((region) => <option key={region.id} value={region.id}>{region.name}</option>)}</select><small>{t("create.regionHelp")}</small></div>
      <div className="form-field"><label htmlFor="game-version">{t("create.gameVersion")}</label><input id="game-version" required value={gameVersion} onChange={(event) => setGameVersion(event.target.value)} /></div>
      {checkout.isError ? <p className="form-error" role="alert">{t("create.failed")}</p> : null}
      <Button disabled={checkout.isPending || !workspace} type="submit">{checkout.isPending ? t("create.submitting") : t("create.submit")}</Button>
    </form>
  </section>;
}
