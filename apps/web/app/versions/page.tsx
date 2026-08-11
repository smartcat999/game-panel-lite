"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Download, Loader2 } from "lucide-react";
import { Button, Card } from "@/components/ui";
import { PageHeader } from "@/components/page-header";
import { listGames, prepareRuntimeImage } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { providerDisplayName } from "@/lib/provider-display";
import { formatRuntimeInstallError } from "@/lib/runtime-errors";
import { isRuntimeImagePreparing, runtimeImageLabelKey, runtimeImageTone } from "@/lib/runtime-image";
import { cn } from "@/lib/utils";
import type { ProviderCatalog, ProviderKey, RuntimeImageStatus } from "@/lib/types";

const imageVersionGridColumns = "md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.15fr)_9rem]";

export default function VersionsPage() {
  const { locale, t } = useI18n();
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
      <PageHeader title={t("versionManagementTitle")} description={t("versionManagementDescription")} />
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
              <div className={cn("hidden gap-4 border-b border-panel-line bg-slate-950/30 px-5 py-3 text-xs font-medium text-slate-500 md:grid", imageVersionGridColumns)}>
                <span>{t("versionManagementProvider")}</span>
                <span>{t("versionManagementInstalledImageVersion")}</span>
                <span>{t("versionManagementTargetImageVersion")}</span>
                <span>{t("versionManagementImageStatus")}</span>
                <span>{t("versionManagementImageUpdatedAt")}</span>
                <span className="sr-only">{t("actions")}</span>
              </div>
              <div className="divide-y divide-panel-line">
                {supportedProviders.map((provider) => (
                  <ImageVersionRow
                    key={provider.key}
                    locale={locale}
                    provider={provider}
                    busy={prepareMutation.isPending && prepareMutation.variables?.providerKey === provider.key}
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
  error,
  locale,
  onPrepare,
  provider
}: {
  busy: boolean;
  error: string;
  locale: "zh" | "en";
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
    ? t("gameLibraryUpdate")
    : status?.status === "failed"
      ? t("versionManagementRetryImage")
      : t("gameLibraryInstall");

  return (
    <div className="px-5 py-4">
      <div className={cn("grid gap-4 md:items-center", imageVersionGridColumns)}>
        <div className="min-w-0">
          <p className="font-medium text-slate-100">{providerDisplayName(provider.key, provider.name, t)}</p>
          <p className="mt-1 truncate font-mono text-xs text-slate-500" title={status?.image}>{status?.image || "—"}</p>
        </div>
        <ImageVersionValue label={t("versionManagementInstalledImageVersion")} value={status?.installedVersion || "—"} />
        <ImageVersionValue label={t("versionManagementTargetImageVersion")} value={status?.targetVersion || provider.recommendedVersion || "—"} />
        <div>
          <span className="mb-1 block text-xs text-slate-500 md:hidden">{t("versionManagementImageStatus")}</span>
          <RuntimeImageBadge status={displayStatus} />
        </div>
        <ImageVersionValue label={t("versionManagementImageUpdatedAt")} value={formatImageTime(status?.updatedAt, locale, t("none"))} />
        <div className="flex justify-end">
          {actionable || preparing ? (
            <Button type="button" variant={status?.status === "failed" ? "secondary" : "primary"} className="w-full md:w-auto" disabled={preparing} onClick={onPrepare}>
              {preparing ? <Loader2 aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" /> : <Download aria-hidden="true" className="size-4" />}
              {preparing ? t("gameLibraryInstalling") : actionLabel}
            </Button>
          ) : (
            <span className="text-xs text-slate-500">{t("versionManagementNoImageAction")}</span>
          )}
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

function ImageVersionValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <span className="mb-1 block text-xs text-slate-500 md:hidden">{label}</span>
      <span className="block truncate text-sm text-slate-300" title={value}>{value}</span>
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

function formatImageTime(value: string | undefined, locale: "zh" | "en", fallback: string) {
  if (!value) return fallback;
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return fallback;
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", { dateStyle: "medium", timeStyle: "short" }).format(timestamp);
}
