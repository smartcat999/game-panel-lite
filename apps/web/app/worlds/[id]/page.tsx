"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Download, FileArchive, Server as ServerIcon, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteWorld, downloadWorldFile, listGameServers, listWorlds } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { getWorldSourceServerId } from "@/lib/server-detail-resources";
import { getTerrariaLanguageLabel } from "@/lib/terraria-language";
import type { World } from "@/lib/types";

function worldModeLabel(world: World, vanillaLabel: string) {
  if (world.providerKey === "terraria-tmodloader") return "tModLoader";
  return vanillaLabel;
}

function difficultyLabel(value: string, t: ReturnType<typeof useI18n>["t"]) {
  return {
    journey: t("tagJourney"),
    classic: t("tagClassic"),
    expert: t("tagExpert"),
    master: t("tagMaster")
  }[value] ?? value;
}

function worldSizeLabel(value: string, t: ReturnType<typeof useI18n>["t"]) {
  return {
    small: t("tagSmallWorld"),
    medium: t("tagMediumWorld"),
    large: t("tagLargeWorld")
  }[value] ?? value;
}

function stringConfigValue(config: World["config"], key: string, fallback = "") {
  const value = config?.[key];
  return typeof value === "string" ? value : fallback;
}

function numberConfigValue(config: World["config"], key: string, fallback = 0) {
  const value = config?.[key];
  return typeof value === "number" ? value : fallback;
}

export default function WorldDetailPage() {
  if (!showWorldAndBackupFeatures) return <HiddenFeaturePage />;
  return <EnabledWorldDetailPage />;
}

function EnabledWorldDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const client = useQueryClient();
  const id = params.id;
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false });
  const [pendingDelete, setPendingDelete] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const world = useMemo(() => (worldsQuery.data ?? []).find((item) => item.id === id), [id, worldsQuery.data]);
  const servers = serversQuery.data ?? [];
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const sourceServerId = world ? getWorldSourceServerId(world) ?? "" : "";
  const sourceServerName = sourceServerId ? serverNameById.get(sourceServerId) ?? sourceServerId : "";
  const usingServers = useMemo(() => (world ? servers.filter((server) => server.spec.sourceWorldId === world.id) : []), [servers, world]);

  const remove = useMutation({
    mutationFn: deleteWorld,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["worlds"] });
      router.push("/worlds");
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : "";
      setErrorMessage(message.includes("world template is used") ? t("unableDeleteWorldInUse") : message.includes("active world") ? t("unableDeleteActiveWorld") : message || t("unableDeleteWorld"));
    }
  });

  const download = async () => {
    if (!world) return;
    setDownloading(true);
    setErrorMessage("");
    try {
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : t("unableDownloadWorld"));
    } finally {
      setDownloading(false);
    }
  };

  if (worldsQuery.isLoading) {
    return <p className="text-sm text-slate-400">{t("loading")}</p>;
  }

  if (worldsQuery.isError || !world) {
    return (
      <>
        <Link href="/worlds" className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-slate-400 transition hover:text-white">
          <ArrowLeft aria-hidden="true" className="size-4" />
          {t("backToWorlds")}
        </Link>
        <Card className="p-6">
          <p className="text-sm text-panel-gold">{worldsQuery.isError ? t("apiWorldsUnavailable") : t("worldTemplateNotFound")}</p>
        </Card>
      </>
    );
  }

  const fileName = world.size.endsWith(".wld") ? world.size : `${world.name}.wld`;
  const config = world.config;

  return (
    <>
      <Link href="/worlds" className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-slate-400 transition hover:text-white">
        <ArrowLeft aria-hidden="true" className="size-4" />
        {t("backToWorlds")}
      </Link>
      <PageHeader title={world.name} description={t("worldTemplateDetailDescription")} />
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-11 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                <FileArchive aria-hidden="true" className="size-5" />
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold text-white">{t("worldTemplate")}</h2>
                <p className="mt-1 truncate text-sm text-slate-500">{fileName}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <DetailTile label={t("serverType")} value={worldModeLabel(world, t("modeVanilla"))} />
              <DetailTile label={t("size")} value={world.bytes} />
              <DetailTile label={t("modified")} value={localizeRelativeTime(world.modified, locale)} />
              <DetailTile label={t("worldFile")} value={fileName} />
            </div>
          </Card>

          <Card className="p-4">
            <h2 className="font-semibold">{t("worldTemplateConfig")}</h2>
            {config ? (
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                <DetailTile label={t("worldName")} value={stringConfigValue(config, "worldName", stringConfigValue(config, "saveName", stringConfigValue(config, "clusterName", world.name)))} />
                <DetailTile label={t("worldSize")} value={worldSizeLabel(stringConfigValue(config, "worldSize", ""), t)} />
                <DetailTile label={t("difficulty")} value={difficultyLabel(stringConfigValue(config, "difficulty", ""), t)} />
                <DetailTile label={t("maxPlayers")} value={String(numberConfigValue(config, "maxPlayers"))} />
                <DetailTile label={t("languageSetting")} value={getTerrariaLanguageLabel(stringConfigValue(config, "language", "en-US"), t)} />
                <DetailTile label={t("motd")} value={stringConfigValue(config, "motd") || t("none")} />
              </div>
            ) : (
              <p className="mt-4 text-sm text-slate-500">{t("worldTemplateConfigUnavailable")}</p>
            )}
          </Card>
        </div>

        <div className="space-y-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("actions")}</h2>
            <div className="mt-4 grid gap-2">
              <Link
                className="inline-flex items-center justify-center gap-2 rounded-md bg-panel-green px-3 py-2 text-sm font-medium text-slate-950 transition hover:bg-panel-green/90 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                href={`/servers/new?worldId=${encodeURIComponent(world.id)}`}
              >
                <ServerIcon aria-hidden="true" className="size-4" />
                {t("createServerFromWorld")}
              </Link>
              <Button variant="secondary" onClick={() => void download()} disabled={downloading}>
                <Download aria-hidden="true" />
                {downloading ? t("downloading") : t("download")}
              </Button>
              <Button variant="danger" onClick={() => setPendingDelete(true)} disabled={remove.isPending}>
                <Trash2 aria-hidden="true" />
                {t("delete")}
              </Button>
            </div>
          </Card>

          <Card className="p-4">
            <h2 className="font-semibold">{t("relatedServers")}</h2>
            <div className="mt-4 space-y-3">
              <RelatedServerRow label={t("sourceServer")} serverId={sourceServerId} serverName={sourceServerName} fallback={t("imported")} />
              <div>
                <p className="text-xs text-slate-500">{t("activeServer")}</p>
                {usingServers.length > 0 ? (
                  <div className="mt-2 space-y-2">
                    <p className="text-sm font-medium text-slate-100">{t("usingServersCount", { count: usingServers.length })}</p>
                    <div className="space-y-1">
                      {usingServers.map((server) => (
                        <Link key={server.id} href={`/servers/${server.id}`} className="block truncate text-sm font-medium text-panel-green hover:underline">
                          {server.name}
                        </Link>
                      ))}
                    </div>
                  </div>
                ) : (
                  <p className="mt-1 truncate text-sm font-medium text-slate-500">{t("notInUse")}</p>
                )}
              </div>
            </div>
          </Card>
        </div>
      </div>

      <ConfirmDialog
        open={pendingDelete}
        eyebrow={t("destructiveAction")}
        title={t("deleteWorldConfirm", { name: world.name })}
        description={t("confirmDeleteWorldDescription", { name: world.name })}
        detail={(
          <p>
            <span className="text-slate-500">{t("world")}: </span>
            <span className="font-medium text-white">{world.name}</span>
          </p>
        )}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        confirmVariant="danger"
        busy={remove.isPending}
        onCancel={() => setPendingDelete(false)}
        onConfirm={() => remove.mutate(world.id)}
      />
    </>
  );
}

function HiddenFeaturePage() {
  return (
    <Card className="p-6">
      <h1 className="text-xl font-semibold text-white">Page not found</h1>
      <p className="mt-2 text-sm text-slate-400">The requested GamePanel Lite page does not exist.</p>
      <Link className="mt-4 inline-flex text-sm font-medium text-panel-green hover:underline" href="/dashboard">
        Back to dashboard
      </Link>
    </Card>
  );
}

function DetailTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-slate-100">{value}</p>
    </div>
  );
}

function RelatedServerRow({ fallback, label, serverId, serverName }: { fallback: string; label: string; serverId: string; serverName: string }) {
  return (
    <div>
      <p className="text-xs text-slate-500">{label}</p>
      {serverId ? (
        <Link href={`/servers/${serverId}`} className="mt-1 block truncate text-sm font-medium text-panel-green hover:underline">
          {serverName}
        </Link>
      ) : (
        <p className="mt-1 truncate text-sm font-medium text-slate-500">{fallback}</p>
      )}
    </div>
  );
}
