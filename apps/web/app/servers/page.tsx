"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { Server as ServerIcon, ChevronLeft, ChevronRight, Columns3, Ellipsis, Filter, Play, Plus, RefreshCw, RotateCcw, Search, Square, Trash2, X } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { ServerManagementTable, type ServerTableColumn, type ServerTableSort } from "@/components/server-management-table";
import { Button, ToastNotice } from "@/components/ui";
import { gameServerStatus } from "@/lib/game-server-resource";
import { getObservabilityMetrics, getSettings, gameServerAction, listComputeNodes, listGameServersPage, listGames } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { useI18n } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import type { GameServerResource } from "@/lib/types";
import { cn } from "@/lib/utils";

import { usePermissions } from "@/lib/permissions";

const pageSizes = [20, 50, 100] as const;
const optionalColumns: ServerTableColumn[] = ["players", "resources", "address", "activity", "version"];
const defaultColumns: ServerTableColumn[] = ["players", "resources", "address", "activity"];

export default function ServersPage() {
  return <Suspense fallback={<ServerListSkeleton />}><ServersPageContent /></Suspense>;
}

function ServersPageContent() {
  const { t } = useI18n();
  const { canCreateServer, isViewer } = usePermissions();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const page = positiveInteger(searchParams.get("page"), 1);
  const pageSize = pageSizeValue(searchParams.get("pageSize"));
  const search = searchParams.get("search") ?? "";
  const game = searchParams.get("game") ?? "all";
  const provider = searchParams.get("provider") ?? "all";
  const status = searchParams.get("status") ?? "all";
  const node = searchParams.get("node") ?? "all";
  const sort = sortValue(searchParams.get("sort"));
  const direction = searchParams.get("direction") === "asc" ? "asc" : "desc";
  const [draftSearch, setDraftSearch] = useState(search);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [visibleColumns, setVisibleColumns] = useState<Set<ServerTableColumn>>(new Set(defaultColumns));
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [notice, setNotice] = useState<{ tone: "success" | "error" | "info"; message: string } | null>(null);

  const updateParams = useCallback((updates: Record<string, string | null>) => {
    const next = new URLSearchParams(searchParams.toString());
    Object.entries(updates).forEach(([key, value]) => value ? next.set(key, value) : next.delete(key));
    router.replace(`${pathname}${next.size ? `?${next.toString()}` : ""}`, { scroll: false });
  }, [pathname, router, searchParams]);
  const serversQuery = useQuery({
    queryKey: ["game-servers-page", page, pageSize, search, game, provider, status, sort, direction],
    queryFn: () => listGameServersPage({ page, pageSize, search, game, provider, status, sort, direction }),
    retry: false,
    refetchInterval: 5000,
    placeholderData: (previous) => previous
  });
  const nodesQuery = useQuery({ queryKey: ["compute-nodes"], queryFn: listComputeNodes, retry: false, staleTime: 10000 });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 5000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 5 * 60 * 1000 });
  const rawServers = serversQuery.data?.items ?? [];
  const nodes = nodesQuery.data ?? [];
  const servers = useMemo(() => {
    if (node === "all") return rawServers;
    return rawServers.filter((s) => (node === "node-local" && (!s.nodeId || s.nodeId === "node-local")) || s.nodeId === node);
  }, [rawServers, node]);
  const gameFilters = useMemo(() => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), [], t), [gamesQuery.data, t]);
  const providerFilters = useMemo(() => providerFilterOptions(gamesQuery.data ?? [], t("filterAll"), [], game), [game, gamesQuery.data, t]);
  const selectedServers = servers.filter((server) => selectedIds.has(server.id));

  useEffect(() => setDraftSearch(search), [search]);
  useEffect(() => {
    const stored = window.localStorage.getItem("gamepanel.server-columns");
    if (!stored) return;
    try {
      const parsed = JSON.parse(stored) as string[];
      setVisibleColumns(new Set(parsed.filter((item): item is ServerTableColumn => optionalColumns.includes(item as ServerTableColumn))));
    } catch { /* keep defaults */ }
  }, []);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (draftSearch !== search) updateParams({ search: draftSearch || null, page: "1" });
    }, 300);
    return () => window.clearTimeout(timer);
  }, [draftSearch, search, updateParams]);
  useEffect(() => setSelectedIds(new Set()), [page, pageSize, search, game, provider, status]);
  useEffect(() => {
    const totalPages = serversQuery.data?.totalPages ?? 0;
    if (totalPages > 0 && page > totalPages) updateParams({ page: String(totalPages) });
  }, [page, serversQuery.data?.totalPages, updateParams]);

  const bulkMutation = useMutation({
    mutationFn: async (action: "start" | "stop" | "restart" | "delete") => {
      const targets = eligibleServers(selectedServers, action);
      const results = await Promise.allSettled(targets.map((server) => gameServerAction(server.id, action)));
      return { action, attempted: targets.length, failed: results.filter((result) => result.status === "rejected").length };
    },
    onSuccess: async ({ action, attempted, failed }) => {
      setDeleteConfirmOpen(false);
      setSelectedIds(new Set());
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["game-servers-page"] }), queryClient.invalidateQueries({ queryKey: ["game-servers"] })]);
      setNotice(failed > 0
        ? { tone: "error", message: t("batchActionPartial", { succeeded: attempted - failed, failed }) }
        : { tone: "success", message: t("batchActionQueued", { count: attempted, action: bulkActionLabel(action, t) }) });
    },
    onError: () => setNotice({ tone: "error", message: t("batchActionFailed") })
  });

  const setFilter = (key: "game" | "provider" | "status", value: string) => updateParams({ [key]: value === "all" ? null : value, page: "1" });
  const clearFilters = () => updateParams({ search: null, game: null, provider: null, status: null, page: "1" });
  const toggleColumn = (column: ServerTableColumn) => {
    const next = new Set(visibleColumns);
    if (next.has(column)) next.delete(column); else next.add(column);
    setVisibleColumns(next);
    window.localStorage.setItem("gamepanel.server-columns", JSON.stringify(Array.from(next)));
  };
  const handleSort = (nextSort: ServerTableSort) => updateParams({ sort: nextSort, direction: sort === nextSort && direction === "asc" ? "desc" : "asc", page: "1" });
  const activeFilters = [
    search ? { key: "search", label: search } : null,
    game !== "all" ? { key: "game", label: optionLabel(gameFilters, game) } : null,
    provider !== "all" ? { key: "provider", label: optionLabel(providerFilters, provider) } : null,
    status !== "all" ? { key: "status", label: statusLabel(status, t) } : null
  ].filter((item): item is { key: string; label: string } => Boolean(item));
  const deletableSelectedServers = eligibleServers(selectedServers, "delete");
  const canDeleteSelection = selectedServers.length > 0 && deletableSelectedServers.length === selectedServers.length;

  return (
    <>
      <PageHeader title={t("serversTitle")} />

      {/* 节点快速切换筛选条 (支持水平平滑滑动与多节点自适应) */}
      {nodes.length > 1 && (
        <div className="mb-3 flex items-center gap-1.5 rounded-lg border border-panel-line bg-panel-card px-3 py-2 text-xs overflow-x-auto scrollbar-none">
          <span className="text-slate-400 font-medium shrink-0 flex items-center gap-1">
            <ServerIcon className="size-3.5 text-panel-green" />
            节点:
          </span>
          <button
            type="button"
            onClick={() => updateParams({ node: null, page: "1" })}
            className={cn(
              "rounded px-2.5 py-1 transition font-medium shrink-0",
              node === "all"
                ? "bg-panel-green text-slate-950 font-bold shadow-xs"
                : "bg-slate-900 text-slate-300 hover:text-white border border-slate-800"
            )}
          >
            全部节点 ({rawServers.length})
          </button>
          {nodes.map((n) => {
            const isSelected = (node === n.id) || (node === "node-local" && n.isLocal);
            const count = rawServers.filter(s => (n.isLocal && (!s.nodeId || s.nodeId === "node-local")) || s.nodeId === n.id).length;
            return (
              <button
                key={n.id}
                type="button"
                onClick={() => updateParams({ node: n.id, page: "1" })}
                className={cn(
                  "flex items-center gap-1.5 rounded px-2.5 py-1 transition font-medium shrink-0 whitespace-nowrap",
                  isSelected
                    ? "bg-panel-green text-slate-950 font-bold shadow-xs"
                    : "bg-slate-900 text-slate-300 hover:text-white border border-slate-800"
                )}
              >
                <span className={cn("size-1.5 rounded-full", n.status === "online" ? "bg-emerald-400" : "bg-slate-500")} />
                <span>{n.name}</span>
                {n.region ? <span className="opacity-75 text-[10px]">({n.region})</span> : null}
                <span className={cn("rounded-full px-1.5 py-0.2 text-[10px]", isSelected ? "bg-slate-900/40 text-slate-950 font-bold" : "bg-slate-800 text-slate-400")}>
                  {count}
                </span>
              </button>
            );
          })}
        </div>
      )}

      <section className="mb-3 rounded-lg border border-panel-line bg-panel-card px-3 py-2.5" aria-label={t("serverManagementToolbar")}>
        <div className="flex flex-col gap-2.5 xl:flex-row xl:items-center xl:justify-between">
          <div className="flex flex-wrap items-center gap-1.5">
            {canCreateServer && (
              <Link href="/servers/new"><Button className="h-9 px-3"><Plus className="size-4" />{t("createServer")}</Button></Link>
            )}
            {!isViewer && (
              <>
                <Button className="h-9 px-3" variant="secondary" disabled={!eligibleServers(selectedServers, "start").length || bulkMutation.isPending} onClick={() => bulkMutation.mutate("start")}><Play className="size-4" />{t("actionStart")}</Button>
                <Button className="h-9 px-3" variant="secondary" disabled={!eligibleServers(selectedServers, "stop").length || bulkMutation.isPending} onClick={() => bulkMutation.mutate("stop")}><Square className="size-4" />{t("actionStop")}</Button>
                <details className="group relative">
                  <summary className={cn(toolbarIconClass, selectedIds.size === 0 && "pointer-events-none opacity-45")}><Ellipsis className="size-4" /><span className="hidden sm:inline">{t("moreActions")}</span></summary>
                  <div className="absolute left-0 top-11 z-30 w-44 rounded-md border border-panel-line bg-slate-950 p-1 shadow-[0_6px_12px_rgba(0,0,0,0.35)]">
                    <MenuButton disabled={!eligibleServers(selectedServers, "restart").length || bulkMutation.isPending} onClick={() => bulkMutation.mutate("restart")} icon={<RotateCcw className="size-4" />} label={t("actionRestart")} />
                    <div className="my-1 border-t border-panel-line" />
                    <MenuButton danger disabled={!canDeleteSelection || bulkMutation.isPending} title={!canDeleteSelection && selectedIds.size ? t("deleteRequiresStopped") : undefined} onClick={() => setDeleteConfirmOpen(true)} icon={<Trash2 className="size-4" />} label={t("deleteSelectedServers")} />
                  </div>
                </details>
                {selectedIds.size ? <span className="ml-1 text-xs text-slate-400">{t("selectedCount", { count: selectedIds.size })}</span> : null}
              </>
            )}
            {isViewer && (
              <span className="rounded-md border border-slate-800 bg-slate-900/80 px-2.5 py-1 text-xs text-slate-400 font-mono">
                只读访客模式 (Viewer Mode)
              </span>
            )}
          </div>
          <div className="flex min-w-0 flex-1 items-center gap-1.5 xl:max-w-2xl xl:justify-end">
            <label className="relative min-w-0 flex-1 xl:max-w-xs">
              <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
              <input className="h-9 w-full rounded-md border border-panel-line bg-slate-950/60 pl-9 pr-8 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-panel-green" value={draftSearch} onChange={(event) => setDraftSearch(event.target.value)} placeholder={t("searchServers")} />
              {draftSearch ? <button aria-label={t("clearSearch")} className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-white" onClick={() => setDraftSearch("")} type="button"><X className="size-4" /></button> : null}
            </label>
            <details className="group relative">
              <summary className={toolbarIconClass} title={t("filters")}><Filter className="size-4" /><span className="hidden sm:inline">{t("filters")}</span>{activeFilters.length ? <span className="rounded-full bg-panel-green/15 px-1.5 text-[11px] text-panel-green">{activeFilters.length}</span> : null}</summary>
              <div className="absolute right-0 top-11 z-30 w-72 space-y-3 rounded-md border border-panel-line bg-slate-950 p-3 shadow-[0_6px_12px_rgba(0,0,0,0.35)]">
                <FilterSelect label={t("filterGame")} options={gameFilters} value={game} onChange={(value) => setFilter("game", value)} />
                <FilterSelect label={t("filterStatus")} options={[{ key: "all", label: t("filterAll") }, { key: "running", label: t("filterRunning") }, { key: "stopped", label: t("filterStopped") }, { key: "errored", label: t("statusErrored") }]} value={status} onChange={(value) => setFilter("status", value)} />
                <FilterSelect label={t("filterRunMode")} options={providerFilters.map((option) => ({ key: option.key, label: option.label ?? option.key }))} value={provider} onChange={(value) => setFilter("provider", value)} />
              </div>
            </details>
            <button className={toolbarSquareClass} aria-label={t("refresh")} title={t("refresh")} onClick={() => void serversQuery.refetch()} type="button"><RefreshCw className={cn("size-4", serversQuery.isFetching && "animate-spin")} /></button>
            <details className="group relative">
              <summary className={toolbarSquareClass} title={t("columnSettings")}><Columns3 className="size-4" /></summary>
              <div className="absolute right-0 top-11 z-30 w-52 rounded-md border border-panel-line bg-slate-950 p-1.5 shadow-[0_6px_12px_rgba(0,0,0,0.35)]">
                {optionalColumns.map((column) => <label key={column} className="flex cursor-pointer items-center justify-between rounded px-2.5 py-2 text-sm text-slate-300 hover:bg-slate-800"><span>{columnLabel(column, t)}</span><input className="accent-panel-green" type="checkbox" checked={visibleColumns.has(column)} onChange={() => toggleColumn(column)} /></label>)}
              </div>
            </details>
          </div>
        </div>
        {activeFilters.length ? <div className="mt-2.5 flex flex-wrap items-center gap-1.5 border-t border-panel-line pt-2.5">{activeFilters.map((filter) => <button key={filter.key} className="inline-flex h-7 items-center gap-1.5 rounded-md border border-panel-line bg-slate-900 px-2 text-xs text-slate-300 hover:border-slate-600" onClick={() => filter.key === "search" ? setDraftSearch("") : setFilter(filter.key as "game" | "provider" | "status", "all")} type="button">{filter.label}<X className="size-3" /></button>)}<button className="ml-1 text-xs text-slate-500 hover:text-white" onClick={clearFilters} type="button">{t("clearFilters")}</button></div> : null}
      </section>

      {serversQuery.isError ? <p className="mb-4 text-sm text-panel-gold">{t("apiServersUnavailable")}</p> : null}
      {servers.length ? <ServerManagementTable servers={servers} nodes={nodes} metrics={metricsQuery.data?.servers} publicHost={settingsQuery.data?.publicHost} selectedIds={selectedIds} visibleColumns={visibleColumns} sort={sort} direction={direction} onSelectionChange={setSelectedIds} onSort={handleSort} onAddressCopied={() => setNotice({ tone: "success", message: t("serverAddressCopied") })} /> : null}
      {!servers.length ? <div className="rounded-lg border border-panel-line bg-panel-card px-5 py-12 text-center text-sm text-slate-400">{serversQuery.isLoading ? t("loading") : t("noServersMatch")}</div> : null}
      <Pagination page={page} pageSize={pageSize} total={serversQuery.data?.total ?? 0} totalPages={serversQuery.data?.totalPages ?? 0} onChange={(updates) => updateParams(updates)} t={t} />

      {notice ? <div className="fixed right-4 top-20 z-[80]"><ToastNotice closeLabel={t("close")} message={notice.message} tone={notice.tone} onClose={() => setNotice(null)} /></div> : null}
      <ConfirmDialog open={deleteConfirmOpen} eyebrow={t("highRiskOperation")} title={t("deleteSelectedServers")} description={t("batchDeleteDescription", { count: selectedIds.size })} detail={<span>{selectedServers.map((server) => server.name).join("、")}</span>} cancelLabel={t("cancel")} confirmLabel={bulkMutation.isPending ? t("actionDeleting") : t("deleteSelectedServers")} busy={bulkMutation.isPending} onCancel={() => setDeleteConfirmOpen(false)} onConfirm={() => bulkMutation.mutate("delete")} />
    </>
  );
}

const toolbarIconClass = "flex h-9 cursor-pointer list-none items-center gap-1.5 rounded-md border border-panel-line bg-slate-950/40 px-2.5 text-sm text-slate-300 transition hover:border-slate-600 hover:bg-slate-800 hover:text-white focus:outline-none focus-visible:ring-1 focus-visible:ring-panel-green [&::-webkit-details-marker]:hidden";
const toolbarSquareClass = "flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/40 text-slate-400 transition hover:border-slate-600 hover:bg-slate-800 hover:text-white focus:outline-none focus-visible:ring-1 focus-visible:ring-panel-green disabled:cursor-not-allowed disabled:opacity-40";

function MenuButton({ danger = false, disabled, icon, label, onClick, title }: { danger?: boolean; disabled?: boolean; icon: React.ReactNode; label: string; onClick: () => void; title?: string }) { return <button className={cn("flex h-9 w-full items-center gap-2 rounded px-2.5 text-left text-sm text-slate-300 hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-40", danger && "text-red-300 hover:bg-red-500/10")} disabled={disabled} onClick={onClick} title={title} type="button">{icon}{label}</button>; }
function FilterSelect({ label, options, value, onChange }: { label: string; options: { key: string; label: string }[]; value: string; onChange: (value: string) => void }) { return <label className="block text-xs text-slate-500"><span className="mb-1.5 block">{label}</span><select className="h-9 w-full rounded-md border border-panel-line bg-slate-900 px-2.5 text-sm text-slate-100 outline-none focus:border-panel-green" value={value} onChange={(event) => onChange(event.target.value)}>{options.map((option) => <option key={option.key} value={option.key}>{option.label}</option>)}</select></label>; }
function Pagination({ page, pageSize, total, totalPages, onChange, t }: { page: number; pageSize: 20 | 50 | 100; total: number; totalPages: number; onChange: (updates: Record<string, string>) => void; t: ReturnType<typeof useI18n>["t"] }) { return <div className="mt-3 flex flex-col gap-2 text-sm text-slate-500 sm:flex-row sm:items-center sm:justify-between"><span>{t("serverTotalCount", { count: total })}</span><div className="flex items-center gap-2"><label className="flex items-center gap-2">{t("rowsPerPage")}<select className="h-8 rounded-md border border-panel-line bg-panel-card px-2 text-slate-200 outline-none focus:border-panel-green" value={pageSize} onChange={(event) => onChange({ pageSize: event.target.value, page: "1" })}>{pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}</select></label><span className="min-w-20 text-center">{t("pageOf", { page: totalPages ? page : 0, total: totalPages })}</span><button className={toolbarSquareClass} disabled={page <= 1} onClick={() => onChange({ page: String(page - 1) })} type="button"><ChevronLeft className="size-4" /></button><button className={toolbarSquareClass} disabled={!totalPages || page >= totalPages} onClick={() => onChange({ page: String(page + 1) })} type="button"><ChevronRight className="size-4" /></button></div></div>; }
function ServerListSkeleton() { return <><PageHeader title="服务器" /><div className="h-14 animate-pulse rounded-lg border border-panel-line bg-panel-card" /><div className="mt-3 h-64 animate-pulse rounded-lg border border-panel-line bg-panel-card" /></>; }
function positiveInteger(value: string | null, fallback: number) { const parsed = Number(value); return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback; }
function pageSizeValue(value: string | null): 20 | 50 | 100 { const parsed = Number(value); return pageSizes.includes(parsed as 20 | 50 | 100) ? parsed as 20 | 50 | 100 : 20; }
function sortValue(value: string | null): ServerTableSort { return value === "name" || value === "status" ? value : "updatedAt"; }
function optionLabel(options: { key: string; label?: string }[], value: string) { return options.find((option) => option.key === value)?.label ?? value; }
function statusLabel(status: string, t: ReturnType<typeof useI18n>["t"]) { return status === "running" ? t("filterRunning") : status === "stopped" ? t("filterStopped") : status === "errored" ? t("statusErrored") : t("filterAll"); }
function eligibleServers(servers: GameServerResource[], action: "start" | "stop" | "restart" | "delete") { return servers.filter((server) => { const status = gameServerStatus(server); if (action === "start") return status === "stopped" || status === "errored"; if (action === "delete") return status === "stopped" || status === "errored"; if (action === "restart") return status === "running" || status === "stopped" || status === "errored"; return status === "running"; }); }
function bulkActionLabel(action: string, t: ReturnType<typeof useI18n>["t"]) { return action === "start" ? t("actionStart") : action === "stop" ? t("actionStop") : action === "restart" ? t("actionRestart") : t("delete"); }
function columnLabel(column: ServerTableColumn, t: ReturnType<typeof useI18n>["t"]) { return column === "players" ? t("players") : column === "resources" ? t("resources") : column === "address" ? t("serverAddress") : column === "activity" ? t("recentActivity") : t("version"); }
