"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Download, Loader2, RefreshCw } from "lucide-react";
import { Button, Card } from "@/components/ui";
import { PageHeader } from "@/components/page-header";
import { listGames, prepareRuntimeImage } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { providerDisplayName } from "@/lib/provider-display";
import { formatRuntimeInstallError } from "@/lib/runtime-errors";
import { isRuntimeImagePreparing, runtimeImageLabelKey, runtimeImageTone } from "@/lib/runtime-image";
import { cn } from "@/lib/utils";
import { GameAssetsSubNav } from "@/components/sub-nav";
import { usePermissions } from "@/lib/permissions";
import type { ProviderCatalog, ProviderKey, RuntimeImageStatus } from "@/lib/types";

const imageVersionGridColumns = "md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_6.5rem]";

export default function VersionsPage() {
  const { t } = useI18n();
  const { canManageSystem } = usePermissions();
  const queryClient = useQueryClient();
  const gamesQuery = useQuery({
    queryKey: ["games"],
    queryFn: listGames,
    retry: false,
    refetchInterval: (query) => hasActiveImageTask(query.state.data?.flatMap((game) => game.providers)) ? 1000 : false
  });
  const prepareMutation = useMutation({
    mutationFn: ({ providerKey, version }: { providerKey: ProviderKey; version?: string }) => prepareRuntimeImage(providerKey, version),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ["games"] })
  });
  const providers = (gamesQuery.data ?? []).flatMap((game) => game.providers).sort(compareProviderPriority);
  const supportedProviders = providers.filter((provider) => provider.runtimeImage?.status !== "unsupported");
  const unsupportedProviders = providers.filter((provider) => provider.runtimeImage?.status === "unsupported");

  return (
    <>
      <PageHeader title={t("versionManagementTitle")} />
      <GameAssetsSubNav />
      {gamesQuery.isError ? (
        <Card className="flex items-start gap-3 p-4 text-sm text-panel-gold">
          <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
          <span>{t("versionManagementLoadFailed")}</span>
        </Card>
      ) : null}
      {gamesQuery.isLoading ? <ImageVersionSkeleton /> : null}
      {!gamesQuery.isLoading && !gamesQuery.isError ? (
        <div className="space-y-4">
          {supportedProviders.length > 0 ? (
            <Card className="overflow-hidden p-0">
              <div className={cn("flex justify-end gap-4 border-b border-panel-line bg-slate-950/30 px-4 py-2.5 text-xs font-medium text-slate-500 md:grid md:items-center", imageVersionGridColumns)}>
                <span className="hidden md:block">{t("versionManagementProvider")}</span>
                <span className="hidden md:block">{t("version")}</span>
                <span className="hidden md:block">{t("versionManagementImageStatus")}</span>
                <Button
                  type="button"
                  variant="ghost"
                  className="size-7 justify-self-end"
                  aria-label={t("refresh")}
                  title={t("refresh")}
                  onClick={() => gamesQuery.refetch()}
                  disabled={gamesQuery.isFetching}
                >
                  <RefreshCw aria-hidden="true" className={cn("size-3.5", gamesQuery.isFetching && "animate-spin motion-reduce:animate-none")} />
                </Button>
              </div>
              <div className="divide-y divide-panel-line">
                {supportedProviders.map((provider) => (
                  <ImageVersionRow
                    key={provider.key}
                    provider={provider}
                    busy={prepareMutation.isPending && prepareMutation.variables?.providerKey === provider.key}
                    canInstall={canManageSystem}
                    error={prepareMutation.isError && prepareMutation.variables?.providerKey === provider.key ? formatRuntimeInstallError(prepareMutation.error, t) : ""}
                    onPrepare={() => prepareMutation.mutate({ providerKey: provider.key, version: provider.recommendedVersion })}
                  />
                ))}
              </div>
            </Card>
          ) : (
            <Card className="p-8 text-center text-sm text-slate-500">{t("versionManagementNoSupportedProviders")}</Card>
          )}

          {unsupportedProviders.length > 0 ? (
            <Card className="px-5 py-4">
              <p className="text-sm font-medium text-slate-300">{t("versionManagementUnsupportedGroup", { count: unsupportedProviders.length })}</p>
              <p className="mt-1 text-xs text-slate-500">
                {unsupportedProviders.map((provider) => providerDisplayName(provider.key, provider.name, t)).join("、")}
              </p>
            </Card>
          ) : null}
        </div>
      ) : null}
    </>
  );
}

function ImageVersionRow({
  busy,
  canInstall,
  error,
  onPrepare,
  provider
}: {
  busy: boolean;
  canInstall: boolean;
  error: string;
  onPrepare: () => void;
  provider: ProviderCatalog;
}) {
  const { t } = useI18n();
  const status = provider.runtimeImage;
  const preparing = busy || isRuntimeImagePreparing(status);
  const displayStatus: RuntimeImageStatus | undefined = preparing
    ? { ...status, image: status?.image ?? provider.key, status: "preparing" }
    : status;
  const actionable = !preparing && status?.status !== "ready" && status?.status !== "unsupported";
  const actionLabel = status?.status === "update_available"
    ? t("versionManagementUpdateAction")
    : status?.status === "failed"
      ? t("versionManagementRetryAction")
      : t("versionManagementInstallAction");

  return (
    <div className="px-4 py-2.5">
      <div className={cn("grid gap-4 md:items-center", imageVersionGridColumns)}>
        <div className="min-w-0">
          <p className="font-medium text-slate-100">{providerDisplayName(provider.key, provider.name, t)}</p>
          <p className="mt-0.5 truncate font-mono text-[11px] text-slate-500" title={status?.image}>{status?.image || "—"}</p>
        </div>
        <ImageVersionComparison
          current={status?.installedVersion || "—"}
          target={status?.targetVersion || provider.recommendedVersion || "—"}
        />
        <div>
          <span className="mb-1 block text-xs text-slate-500 md:hidden">{t("versionManagementImageStatus")}</span>
          <RuntimeImageBadge status={displayStatus} />
        </div>
        <div className="flex justify-end">
          {canInstall && (actionable || preparing) ? (
            <Button
              type="button"
              variant={status?.status === "update_available" ? "primary" : "secondary"}
              className="h-8 w-full whitespace-nowrap px-2.5 text-xs md:w-auto md:min-w-20"
              disabled={preparing}
              onClick={onPrepare}
            >
              {preparing ? <Loader2 aria-hidden="true" className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Download aria-hidden="true" className="size-3.5" />}
              {preparing ? t("versionManagementInstallingAction") : actionLabel}
            </Button>
          ) : null}
        </div>
      </div>
      {preparing && typeof displayStatus?.progress === "number" ? (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-800" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={displayStatus.progress}>
          <div className="h-full rounded-full bg-panel-green transition-[width] duration-200 motion-reduce:transition-none" style={{ width: `${Math.max(0, Math.min(100, displayStatus.progress))}%` }} />
        </div>
      ) : null}
      {error ? <p className="mt-3 text-xs text-red-300" role="alert">{error}</p> : null}
    </div>
  );
}

function ImageVersionComparison({ current, target }: { current: string; target: string }) {
  const changed = current !== "—" && target !== "—" && current !== target;
  return (
    <div className="min-w-0 font-mono text-sm">
      <span className="text-slate-300">{current}</span>
      {changed ? <span className="ml-2 text-panel-gold">→ {target}</span> : null}
      {current === "—" && target !== "—" ? <span className="ml-2 text-slate-500">→ {target}</span> : null}
    </div>
  );
}

function RuntimeImageBadge({ status }: { status?: RuntimeImageStatus }) {
  const { t } = useI18n();
  const tone = runtimeImageTone(status);
  return (
    <span className={cn(
      "inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium",
      tone === "success" && "bg-panel-green/15 text-panel-green",
      tone === "info" && "bg-sky-500/15 text-sky-300",
      tone === "warning" && "bg-panel-gold/15 text-panel-gold",
      tone === "neutral" && "bg-slate-800 text-slate-400"
    )}>
      <span className={cn("size-1.5 rounded-full bg-current", status?.status === "preparing" && "animate-pulse motion-reduce:animate-none")} />
      {t(runtimeImageLabelKey(status))}
    </span>
  );
}

function ImageVersionSkeleton() {
  return (
    <Card className="space-y-3 p-5" aria-label="Loading image versions">
      <div className="h-12 animate-pulse rounded-md bg-slate-800/70 motion-reduce:animate-none" />
      <div className="h-12 animate-pulse rounded-md bg-slate-800/50 motion-reduce:animate-none" />
      <div className="h-12 animate-pulse rounded-md bg-slate-800/40 motion-reduce:animate-none" />
    </Card>
  );
}

function hasActiveImageTask(providers?: ProviderCatalog[]) {
  return Boolean(providers?.some((provider) => isRuntimeImagePreparing(provider.runtimeImage)));
}

function compareProviderPriority(left: ProviderCatalog, right: ProviderCatalog) {
  const priority = (provider: ProviderCatalog) => {
    switch (provider.runtimeImage?.status) {
      case "update_available": return 0;
      case "failed": return 1;
      case "missing": return 2;
      case "preparing": return 3;
      case "ready": return 4;
      default: return 5;
    }
  };
  return priority(left) - priority(right) || left.name.localeCompare(right.name);
}
