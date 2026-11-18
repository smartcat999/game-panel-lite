"use client";

import { useEffect, useMemo, useState } from "react";
import { LayoutGrid, List, Plus, Search } from "lucide-react";
import { useConsole } from "./ConsoleProvider";
import { EmptyState, Input, LinkButton, PageHeader, Segmented, Card } from "../components/console/ui";
import { phaseOf } from "./lib/format";
import type { InstanceItem } from "./lib/types";
import { useServerActions } from "../components/console/servers/api";
import { DeleteServerDialog, gameOf } from "../components/console/servers/shared";
import {
  NewServerCard,
  ServerCard,
  ServerCardSkeleton,
  ServerTable,
} from "../components/console/servers/ServerViews";

type StatusFilter = "all" | "running" | "stopped";
type ViewMode = "cards" | "table";
const VIEW_KEY = "gp_console_server_view";

export default function ServersPage() {
  const { instances, instancesLoading, nodes, apps, live, loading, canManageTenant, canOperateInstances } = useConsole();
  const { busy, setState, remove } = useServerActions();

  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [view, setView] = useState<ViewMode>("cards");
  const [toDelete, setToDelete] = useState<InstanceItem | null>(null);

  useEffect(() => {
    const saved = localStorage.getItem(VIEW_KEY);
    if (saved === "cards" || saved === "table") setView(saved);
    if (typeof window !== "undefined") {
      const sp = new URLSearchParams(window.location.search);
      const urlStatus = sp.get("status");
      if (urlStatus === "running" || urlStatus === "stopped") setStatus(urlStatus);
      const urlView = sp.get("view");
      if (urlView === "cards" || urlView === "table") setView(urlView);
    }
  }, []);

  const changeStatus = (next: StatusFilter) => {
    setStatus(next);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (next === "all") sp.delete("status");
    else sp.set("status", next);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  };

  const changeView = (v: ViewMode) => {
    setView(v);
    localStorage.setItem(VIEW_KEY, v);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (v === "cards") sp.delete("view");
    else sp.set("view", v);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  };

  const running = instances.filter((i) => phaseOf(i) === "Running").length;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return instances
      .filter((inst) => {
        if (status === "running" && phaseOf(inst) !== "Running") return false;
        if (status === "stopped" && !(inst.spec.state === "Stopped" || phaseOf(inst) === "Stopped")) return false;
        if (!q) return true;
        const game = gameOf(inst, apps);
        return (
          inst.metadata.name.toLowerCase().includes(q) ||
          game.title.toLowerCase().includes(q) ||
          (inst.spec.image || "").toLowerCase().includes(q)
        );
      })
      .sort((a, b) => a.metadata.name.localeCompare(b.metadata.name));
  }, [apps, instances, query, status]);

  const onToggle = (inst: InstanceItem) =>
    void setState(inst, inst.spec.state === "Stopped" ? "Running" : "Stopped");

  const confirmDelete = async () => {
    if (!toDelete) return;
    const ok = await remove(toDelete);
    if (ok) setToDelete(null);
  };

  const initialLoading = (loading && apps.length === 0) || (instancesLoading && instances.length === 0);
  const empty = !loading && !instancesLoading && instances.length === 0;

  const description = initialLoading ? (
    "正在读取服务器列表…"
  ) : (
    <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
      <span>
        {instances.length === 0
          ? "创建并管理当前工作区的游戏服务器实例。"
          : `${running} 台运行中 · 共 ${instances.length} 台`}
      </span>
      {live && (
        <span className="inline-flex items-center gap-1.5 text-[13px] text-[#1F9D55]" title="状态实时更新中">
          <span className="h-1.5 w-1.5 rounded-full bg-[#1F9D55]" aria-hidden />
          实时
        </span>
      )}
    </span>
  );

  return (
    <>
      <PageHeader
        title="我的服务器"
        description={description}
        actions={
          canManageTenant ? (
            <LinkButton href="/console/new" variant="primary" icon={<Plus className="h-4 w-4" />}>
              新建服务器
            </LinkButton>
          ) : undefined
        }
      />

      {empty ? (
        <Card>
          <EmptyState
            image="/landing/features/connect.jpg"
            title="还没有服务器"
            body={
              canManageTenant
                ? "选一个游戏和配置，我们会帮你把服务器开好，并给你一个可以直接分享给朋友的连接地址。"
                : "这个工作区还没有创建服务器，请联系工作区管理员新建服务器。"
            }
            action={
              canManageTenant ? (
                <LinkButton href="/console/new" variant="primary" icon={<Plus className="h-4 w-4" />}>
                  新建服务器
                </LinkButton>
              ) : undefined
            }
          />
        </Card>
      ) : (
        <>
          <div className="mb-6 flex flex-wrap items-center gap-3">
            <div className="relative w-full sm:w-64">
              <Search
                className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]"
                aria-hidden
              />
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索服务器名称或游戏"
                aria-label="搜索服务器"
                className="pl-9"
              />
            </div>
            <Segmented<StatusFilter>
              label="按状态筛选"
              value={status}
              onChange={changeStatus}
              items={[
                { value: "all", label: "全部" },
                { value: "running", label: "运行中" },
                { value: "stopped", label: "已停止" },
              ]}
            />
            <div className="ml-auto">
              <Segmented<ViewMode>
                label="显示方式"
                value={view}
                onChange={changeView}
                items={[
                  {
                    value: "cards",
                    label: (
                      <>
                        <LayoutGrid className="h-4 w-4" aria-hidden />
                        <span className="sr-only">卡片视图</span>
                      </>
                    ),
                  },
                  {
                    value: "table",
                    label: (
                      <>
                        <List className="h-4 w-4" aria-hidden />
                        <span className="sr-only">表格视图</span>
                      </>
                    ),
                  },
                ]}
              />
            </div>
          </div>

          {initialLoading ? (
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {[0, 1, 2].map((i) => (
                <ServerCardSkeleton key={i} />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <Card>
              <EmptyState
                image={null}
                title="没有符合条件的服务器"
                body="换个关键词，或者切回「全部」看看。"
              />
            </Card>
          ) : view === "cards" ? (
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {filtered.map((inst) => (
                <ServerCard
                  key={inst.metadata.name}
                  inst={inst}
                  nodes={nodes}
                  busy={busy[inst.metadata.name]}
                  onToggle={canOperateInstances ? onToggle : undefined}
                  onDelete={canManageTenant ? setToDelete : undefined}
                />
              ))}
              {canManageTenant && <NewServerCard />}
            </div>
          ) : (
            <ServerTable
              items={filtered}
              nodes={nodes}
              busy={busy}
              onToggle={canOperateInstances ? onToggle : undefined}
              onDelete={canManageTenant ? setToDelete : undefined}
            />
          )}
        </>
      )}

      <DeleteServerDialog
        inst={toDelete}
        loading={!!toDelete && busy[toDelete.metadata.name] === "delete"}
        onConfirm={() => void confirmDelete()}
        onClose={() => setToDelete(null)}
      />
    </>
  );
}
