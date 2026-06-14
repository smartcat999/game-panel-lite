"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { Button, Input } from "@/components/ui";
import { listServers } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const filters = [
  { key: "all", labelKey: "filterAll" },
  { key: "running", labelKey: "filterRunning" },
  { key: "stopped", labelKey: "filterStopped" },
  { key: "vanilla", labelKey: "filterVanilla" },
  { key: "modded", labelKey: "filterModded" }
] as const;
type Filter = (typeof filters)[number]["key"];

export default function ServersPage() {
  const query = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const { t } = useI18n();
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const servers = query.data ?? [];
  const filteredServers = useMemo(() => {
    const term = search.trim().toLowerCase();
    return servers.filter((server) => {
      const matchesSearch = !term || [server.name, server.world, String(server.port)].some((value) => value.toLowerCase().includes(term));
      const matchesFilter =
        filter === "all" ||
        (filter === "running" && server.status === "running") ||
        (filter === "stopped" && server.status === "stopped") ||
        (filter === "vanilla" && server.mode === "vanilla") ||
        (filter === "modded" && server.mode === "tmodloader");
      return matchesSearch && matchesFilter;
    });
  }, [filter, search, servers]);
  return (
    <AppShell>
      <PageHeader
        title={t("serversTitle")}
        description={t("serversDescription")}
      />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Input className="max-w-sm" placeholder={t("searchServers")} value={search} onChange={(event) => setSearch(event.target.value)} />
        {filters.map((item) => (
          <Button
            key={item.key}
            variant="secondary"
            className={cn("px-3 py-2", filter === item.key && "border-panel-green bg-panel-green/10 text-panel-green")}
            onClick={() => setFilter(item.key)}
          >
            {t(item.labelKey)}
          </Button>
        ))}
      </div>
      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiServersUnavailable")}</p>}
      <div className="grid gap-4 xl:grid-cols-2">
        {filteredServers.map((server) => <ServerCard key={server.id} server={server} />)}
      </div>
      {filteredServers.length === 0 && <p className="mt-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("noServersMatch")}</p>}
    </AppShell>
  );
}
