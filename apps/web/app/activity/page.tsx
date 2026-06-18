"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity as ActivityIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { listActivity, listGames, listServers } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type ActivityGameFilter = "all" | string;

export default function ActivityPage() {
  const { locale, t } = useI18n();
  const query = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const [gameFilter, setGameFilter] = useState<ActivityGameFilter>("all");
  const events = query.data ?? [];
  const servers = serversQuery.data ?? [];
  const serverById = useMemo(() => new Map(servers.map((server) => [server.id, server])), [servers]);
  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), events.map((event) => event.instanceId ? serverById.get(event.instanceId)?.gameKey : undefined)),
    [events, gamesQuery.data, serverById, t]
  );
  const filteredEvents = useMemo(() => {
    if (gameFilter === "all") return events;
    return events.filter((event) => event.instanceId && serverById.get(event.instanceId)?.gameKey === gameFilter);
  }, [events, gameFilter, serverById]);
  return (
    <>
      <PageHeader title={t("activityTitle")} description={t("activityDescription")} />
      {(query.isError || serversQuery.isError || gamesQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiActivityUnavailable")}</p>}
      <Card className="mb-4 p-3">
        <div className="flex flex-wrap gap-3">
          <FilterGroup label={t("filterGame")} options={gameFilters} value={gameFilter} onChange={setGameFilter} t={t} />
        </div>
      </Card>
      <Card className="overflow-hidden">
        {filteredEvents.length === 0 ? (
          <div className="flex min-h-48 items-center justify-center p-6 text-center text-sm text-slate-400">
            {query.isLoading ? t("loading") : t("noActivityYet")}
          </div>
        ) : (
          <div className="divide-y divide-panel-line">
            {filteredEvents.map((event) => {
              const display = formatActivityEvent(event, locale);
              const server = event.instanceId ? serverById.get(event.instanceId) : undefined;
              return (
                <div key={event.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-panel-green/15 text-panel-green">
                      <ActivityIcon aria-hidden="true" className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <p className="font-medium text-white">{display.message}</p>
                      <p className="mt-1 text-xs text-slate-500">{server?.name ?? event.instanceId ?? t("none")}</p>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2 text-xs text-slate-400">
                    <span className="rounded bg-slate-800 px-2 py-1">{display.typeLabel}</span>
                    <span className="rounded bg-slate-800 px-2 py-1">{localizeRelativeTime(event.created, locale)}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>
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
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[];
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
            type="button"
            variant="ghost"
            className={cn("h-8 px-2.5 py-1 text-xs hover:bg-slate-800", value === item.key && "bg-panel-green/10 text-panel-green hover:bg-panel-green/15")}
            onClick={() => onChange(item.key)}
          >
            {item.labelKey ? t(item.labelKey) : item.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
