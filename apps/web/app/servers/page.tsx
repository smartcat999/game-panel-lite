"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Input } from "@/components/ui";
import { listBackups, listServers } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { MessageKey } from "@/lib/i18n";
import { filterServers, type ServerGameFilter, type ServerStatusFilter, type ServerTypeFilter } from "@/lib/server-filters";
import { attachLatestBackupTimes } from "@/lib/server-metrics";
import { cn } from "@/lib/utils";

const gameFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "terraria", labelKey: "gameTerraria" }
] as const satisfies readonly { key: ServerGameFilter; labelKey: MessageKey }[];

const statusFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "running", labelKey: "filterRunning" },
  { key: "stopped", labelKey: "filterStopped" }
] as const satisfies readonly { key: ServerStatusFilter; labelKey: MessageKey }[];

const typeFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "vanilla", labelKey: "filterVanilla" },
  { key: "modded", labelKey: "filterModded" }
] as const satisfies readonly { key: ServerTypeFilter; labelKey: MessageKey }[];

export default function ServersPage() {
  const query = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false, refetchInterval: 5000 });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const { t } = useI18n();
  const [gameFilter, setGameFilter] = useState<ServerGameFilter>("all");
  const [statusFilter, setStatusFilter] = useState<ServerStatusFilter>("all");
  const [typeFilter, setTypeFilter] = useState<ServerTypeFilter>("all");
  const [search, setSearch] = useState("");
  const servers = useMemo(() => attachLatestBackupTimes(query.data ?? [], backupsQuery.data ?? []), [backupsQuery.data, query.data]);
  useEffect(() => {
    setSearch(new URLSearchParams(window.location.search).get("search") ?? "");
  }, []);
  const filteredServers = useMemo(() => {
    return filterServers(servers, { game: gameFilter, query: search, status: statusFilter, type: typeFilter });
  }, [gameFilter, search, servers, statusFilter, typeFilter]);
  return (
    <>
      <PageHeader
        title={t("serversTitle")}
        description={t("serversDescription")}
      />
      <div className="mb-4 rounded-lg border border-panel-line bg-panel-card p-3">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <Input className="xl:max-w-xs" placeholder={t("searchServers")} value={search} onChange={(event) => setSearch(event.target.value)} />
          <div className="flex flex-wrap gap-3">
            <FilterGroup label={t("filterGame")} options={gameFilters} value={gameFilter} onChange={setGameFilter} t={t} />
            <FilterGroup label={t("filterStatus")} options={statusFilters} value={statusFilter} onChange={setStatusFilter} t={t} />
            <FilterGroup label={t("filterType")} options={typeFilters} value={typeFilter} onChange={setTypeFilter} t={t} />
          </div>
        </div>
      </div>
      {(query.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{query.isError ? t("apiServersUnavailable") : t("apiBackupsUnavailable")}</p>}
      <div className="grid gap-4 xl:grid-cols-2">
        {filteredServers.map((server) => <ServerCard key={server.id} server={server} />)}
      </div>
      {filteredServers.length === 0 && <p className="mt-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("noServersMatch")}</p>}
    </>
  );
}

function FilterGroup<T extends string>({
  label,
  onChange,
  options,
  t,
  value
}: {
  label: string;
  onChange: (value: T) => void;
  options: readonly { key: T; labelKey: MessageKey }[];
  t: (key: MessageKey) => string;
  value: T;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs font-medium text-slate-500">{label}</span>
      <div className="flex rounded-md border border-panel-line bg-slate-950/50 p-0.5">
        {options.map((item) => (
          <Button
            key={item.key}
            variant="ghost"
            className={cn("h-8 px-2.5 py-1 text-xs hover:bg-slate-800", value === item.key && "bg-panel-green/10 text-panel-green hover:bg-panel-green/15")}
            onClick={() => onChange(item.key)}
          >
            {t(item.labelKey)}
          </Button>
        ))}
      </div>
    </div>
  );
}
