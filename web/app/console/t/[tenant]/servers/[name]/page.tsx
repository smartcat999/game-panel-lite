"use client";

import Link from "next/link";
import { Suspense, useEffect, useMemo, useState } from "react";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Play, RotateCw, Square, Trash2, UserPlus } from "lucide-react";
import { useConsole } from "../../../../ConsoleProvider";
import {
  Button,
  Card,
  CopyButton,
  EmptyState,
  LinkButton,
  Mono,
  Segmented,
  Skeleton,
  StatusPill,
  focusRing,
} from "../../../../../components/console/ui";
import { endpointOf, phaseOf } from "../../../../lib/format";
import { tenantRoute } from "../../../../lib/routes";
import type { RegistryArtifact } from "../../../../lib/types";
import { subscribeResourceWatch } from "../../../../lib/ws";
import { DEFAULT_GAME_COVER, resolveRichApp } from "../../../../lib/presets";
import { ArtifactLogModal } from "../../../../../components/console/admin/ArtifactLogModal";
import { useServerActions } from "../../../../../components/console/servers/api";
import { resolveInstanceAlertBanner } from "../../../../../components/console/servers/error-copy";
import { DeleteServerDialog, gameOf, MoreMenu } from "../../../../../components/console/servers/shared";
import { NetworkTab, OverviewTab, SettingsTab } from "../../../../../components/console/servers/InfoTabs";
import { ConsoleTab } from "../../../../../components/console/servers/ConsoleTab";
import { FloatingTerminal, type WorkbenchMode } from "../../../../../components/console/servers/FloatingTerminal";
import { FilesTab } from "../../../../../components/console/servers/FilesTab";
import { AddonsTab } from "../../../../../components/console/servers/AddonsTab";
import { BackupTab } from "../../../../../components/console/servers/BackupTab";
import { ScheduleTab } from "../../../../../components/console/servers/ScheduleTab";
import { InviteModal } from "../../../../../components/console/servers/InviteModal";

const TABS = [
  { value: "overview", label: "概览" },
  { value: "console", label: "控制台" },
  { value: "files", label: "文件" },
  { value: "addons", label: "模组" },
  { value: "backup", label: "备份" },
  { value: "schedules", label: "定时任务" },
  { value: "network", label: "网络" },
  { value: "settings", label: "设置" },
] as const;
type Tab = (typeof TABS)[number]["value"];
const isTab = (v: string | null): v is Tab => TABS.some((t) => t.value === v);

function BackLink() {
  const { tenant } = useConsole();
  return (
    <Link
      href={tenantRoute(tenant)}
      className={`mb-5 inline-flex items-center gap-1.5 rounded-full text-sm font-medium text-[#5B6075] hover:text-[#2B3AE8] ${focusRing}`}
    >
      <ArrowLeft className="h-4 w-4" aria-hidden />
      我的服务器
    </Link>
  );
}

function DetailSkeleton() {
  return (
    <div>
      <Skeleton className="mb-5 h-5 w-24" />
      <Skeleton className="h-48 w-full rounded-[20px]" />
      <Skeleton className="mt-6 h-10 w-full max-w-[460px] rounded-full" />
      <div className="mt-6 grid grid-cols-3 gap-4">
        <Skeleton className="h-20" />
        <Skeleton className="h-20" />
        <Skeleton className="h-20" />
      </div>
    </div>
  );
}

function ServerDetail() {
  const params = useParams<{ name: string }>();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const {
    apiFetch,
    token,
    tenant,
    tenantObj,
    instances,
    instancesLoading,
    nodes,
    apps,
    loading,
    canManageTenant,
    canOperateInstances,
  } = useConsole();
  const { busy, setState, restart, remove } = useServerActions();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [termOpen, setTermOpen] = useState(false);
  const [termMode, setTermMode] = useState<WorkbenchMode>("terminal");
  const [startingArtifacts, setStartingArtifacts] = useState<RegistryArtifact[]>([]);
  const [pullLogTarget, setPullLogTarget] = useState<{ artifact: RegistryArtifact; title: string } | null>(null);

  let name = params?.name ?? "";
  try {
    name = decodeURIComponent(name);
  } catch {
    /* keep raw */
  }

  const tabParam =
    typeof window !== "undefined" ? new URLSearchParams(window.location.search).get("tab") : searchParams.get("tab");
  const urlTab: Tab = isTab(tabParam) ? tabParam : "overview";
  const [tab, setTab] = useState<Tab>(urlTab);

  useEffect(() => {
    const syncTabFromLocation = () => {
      const liveTab =
        typeof window !== "undefined"
          ? new URLSearchParams(window.location.search).get("tab")
          : searchParams.get("tab");
      setTab(isTab(liveTab) ? liveTab : "overview");
    };
    syncTabFromLocation();
    window.addEventListener("popstate", syncTabFromLocation);
    return () => window.removeEventListener("popstate", syncTabFromLocation);
  }, [searchParams]);

  const changeTab = (t: Tab, extra?: Record<string, string | null>) => {
    const savedScrollY = typeof window !== "undefined" ? window.scrollY : 0;
    setTab(t);
    if (typeof window !== "undefined") {
      const sp = new URLSearchParams(window.location.search);
      if (t === "overview") sp.delete("tab");
      else sp.set("tab", t);
      if (!extra || !("section" in extra)) {
        sp.delete("section");
      }
      if (extra) {
        for (const [k, v] of Object.entries(extra)) {
          if (!v) sp.delete(k);
          else sp.set(k, v);
        }
      }
      const qs = sp.toString();
      const nextUrl = qs ? `${pathname}?${qs}` : pathname;
      window.history.replaceState(window.history.state, "", nextUrl);
      requestAnimationFrame(() => {
        if (Math.abs(window.scrollY - savedScrollY) > 2) {
          window.scrollTo({ top: savedScrollY, behavior: "instant" as ScrollBehavior });
        }
      });
    }
  };

  const inst = instances.find((i) => i.metadata.name === name);
  const instStopped = inst?.spec.state === "Stopped";
  const instPhase = inst ? phaseOf(inst) : "";
  const instRunning = instPhase === "Running";

  const requiredArtifactKeys = useMemo(() => {
    const keys = new Set<string>();
    if (inst?.spec.serverArtifact?.repository && inst.spec.serverArtifact.package && inst.spec.serverArtifact.version) {
      keys.add(`${inst.spec.serverArtifact.repository}/${inst.spec.serverArtifact.package}@${inst.spec.serverArtifact.version}`);
    }
    for (const a of inst?.spec.addons || []) {
      if (!a.disabled && a.repository && a.package && a.version) {
        keys.add(`${a.repository}/${a.package}@${a.version}`);
      }
    }
    return Array.from(keys);
  }, [inst?.spec.addons, inst?.spec.serverArtifact]);

  const requiredArtifactKeySig = requiredArtifactKeys.join("|");

  useEffect(() => {
    if (!inst || instStopped || instRunning || !requiredArtifactKeySig) {
      setStartingArtifacts([]);
      return;
    }
    const keySet = new Set(requiredArtifactKeySig.split("|").filter(Boolean));
    const matchesInst = (art: RegistryArtifact) =>
      keySet.has(`${art.spec?.repository}/${art.spec?.package}@${art.spec?.version}`);
    let cancelled = false;
    const loadArtifacts = () => {
      apiFetch("/apis/registry.gamepanel.io/v1/artifacts?limit=200")
        .then((res) => (res.ok ? res.json() : null))
        .then((data: { items?: RegistryArtifact[] } | null) => {
          if (cancelled || !data?.items) return;
          setStartingArtifacts(data.items.filter(matchesInst));
        })
        .catch(() => undefined);
    };
    loadArtifacts();
    const unsub = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: loadArtifacts,
      onEvent: (ev) => {
        const obj = ev.object;
        if (!obj?.metadata?.name || !matchesInst(obj)) return;
        setStartingArtifacts((prev) => {
          if (ev.type === "DELETED") {
            return prev.filter((item) => item.metadata.name !== obj.metadata.name);
          }
          const idx = prev.findIndex((item) => item.metadata.name === obj.metadata.name);
          if (idx >= 0) {
            const next = prev.slice();
            next[idx] = obj;
            return next;
          }
          return [...prev, obj];
        });
      },
    });
    return () => {
      cancelled = true;
      unsub();
    };
  }, [apiFetch, inst, instRunning, instStopped, requiredArtifactKeySig, token]);

  if (!inst) {
    if (loading || instancesLoading) return <DetailSkeleton />;
    return (
      <>
        <BackLink />
        <Card>
          <EmptyState
            title="找不到这台服务器"
            body={
              <>
                <Mono>{name}</Mono> 可能已经被删除，或者属于另一个工作区。可以在顶部切换工作区后再看看。
              </>
            }
            action={
              <LinkButton href={tenantRoute(tenant)} variant="primary">
                返回我的服务器
              </LinkButton>
            }
          />
        </Card>
      </>
    );
  }

  if (loading && apps.length === 0) return <DetailSkeleton />;

  const game = gameOf(inst, apps);
  const matchedApp = resolveRichApp(
    inst.metadata.labels?.["gamepanel.io/preset"] ||
      inst.metadata.labels?.["core.gamepanel.io/app"] ||
      game.id,
    apps,
  );
  const hasAddonBinding = Boolean(matchedApp?.addonBinding);
  const addonTabLabel =
    matchedApp?.addonBinding?.installMode === "flat_dir" &&
    !matchedApp?.addonBinding?.repository?.includes("workshop")
      ? "插件"
      : "模组";
  const phase = phaseOf(inst);
  const stopped = inst.spec.state === "Stopped";
  const running = phase === "Running";
  const endpoint = endpointOf(inst, nodes, apps);
  const state = busy[name];
  const readyCond = inst.status?.conditions?.find((c) => c.type === "Ready");
  const schedCond = inst.status?.conditions?.find((c) => c.type === "Scheduled");
  const unschedulable = schedCond?.status === "False";
  const tenantSuspended = Boolean(
    tenantObj?.status?.phase === "Suspended" ||
      inst.status?.resourceControl?.suspended ||
      readyCond?.reason === "SuspendedByTenant",
  );
  const rawBannerDesc =
    (unschedulable ? schedCond?.message : undefined) ||
    readyCond?.message ||
    inst.status?.message ||
    "";
  const alertBanner = resolveInstanceAlertBanner({
    inst,
    matchedApp,
    addonTabLabel,
    phase,
    unschedulable,
    readyReason: readyCond?.reason,
    rawBannerDesc,
    tenantSuspended,
  });
  const isArtifactSyncFailure = alertBanner?.kind === "artifact";
  const isQuotaOrOOMIssue = alertBanner?.kind === "quota" || alertBanner?.kind === "oom";

  const onDelete = async () => {
    const ok = await remove(inst);
    if (ok) {
      setConfirmDelete(false);
      router.push(tenantRoute(tenant));
    }
  };

  return (
    <>
      <BackLink />

      {/* Banner: image is clipped in its own layer so the menu popover is not. */}
      <section className="relative rounded-[20px] shadow-[0_16px_40px_-28px_rgba(43,58,232,0.6)]">
        <div className="absolute inset-0 overflow-hidden rounded-[20px]" aria-hidden>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={game.cover}
            onError={(e) => {
              if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                e.currentTarget.src = DEFAULT_GAME_COVER;
              }
            }}
            alt=""
            className="h-full w-full object-cover"
          />
          <div className="absolute inset-0 bg-gradient-to-r from-[#0F1222]/90 via-[#0F1222]/70 to-[#0F1222]/30" />
        </div>
        <div className="relative flex min-h-[200px] flex-col justify-end gap-5 p-6 sm:p-8 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0 text-white">
            <div className="text-sm text-white/75">{game.title}</div>
            <div className="mt-1 flex flex-wrap items-center gap-3">
              <h1 className="min-w-0 break-words text-[26px] font-bold leading-tight tracking-tight sm:text-[30px]">
                {name}
              </h1>
              <StatusPill phase={phase} />
            </div>
            <div className="mt-3 inline-flex items-center gap-1 rounded-full bg-white/15 py-1 pl-3.5 pr-1 text-sm backdrop-blur">
              {endpoint ? (
                <>
                  <Mono>{endpoint}</Mono>
                  <span className="rounded-full bg-white">
                    <CopyButton text={endpoint} label="复制连接地址" />
                  </span>
                </>
              ) : (
                <span className="py-1 pr-2.5 text-white/80">
                  {stopped ? "启动后分配连接地址" : phase === "Failed" ? "启动异常，等待重试" : "地址分配中"}
                </span>
              )}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              icon={<UserPlus className="h-4 w-4" />}
              onClick={() => setInviteOpen(true)}
            >
              邀请好友
            </Button>
            {canOperateInstances && (
              stopped ? (
                <Button
                  variant="primary"
                  icon={<Play className="h-4 w-4" />}
                  loading={state === "start"}
                  disabled={!!state}
                  onClick={() => void setState(inst, "Running")}
                >
                  启动
                </Button>
              ) : (
                <>
                  <Button
                    icon={<Square className="h-3.5 w-3.5" />}
                    loading={state === "stop" || phase === "Stopping"}
                    disabled={!!state || phase === "Stopping" || phase === "Restarting"}
                    onClick={() => void setState(inst, "Stopped")}
                  >
                    停止
                  </Button>
                  <Button
                    variant={phase === "Failed" ? "primary" : "secondary"}
                    icon={<RotateCw className="h-4 w-4" />}
                    loading={state === "restart" || phase === "Restarting"}
                    disabled={!!state || phase === "Stopping" || phase === "Restarting"}
                    onClick={() => void restart(inst)}
                  >
                    {phase === "Failed" ? "重试启动" : "重启"}
                  </Button>
                </>
              )
            )}
            {canManageTenant && (
              <MoreMenu
                className="h-10 w-10 bg-white"
                items={[
                  {
                    label: "删除服务器",
                    danger: true,
                    icon: <Trash2 className="h-4 w-4" />,
                    onSelect: () => setConfirmDelete(true),
                  },
                ]}
              />
            )}
          </div>
        </div>
      </section>

      {alertBanner && (
        <div
          role="alert"
          className={`mt-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border px-5 py-3.5 text-sm ${
            alertBanner.isError
              ? "border-[#FECDCA] bg-[#FEF3F2] text-[#B42318]"
              : "border-[#FEDF89] bg-[#FFFAEB] text-[#B54708]"
          }`}
        >
          <div className="min-w-0 flex-1">
            <div className="font-semibold">{alertBanner.title}</div>
            <div className="mt-0.5 text-xs opacity-90">{alertBanner.desc}</div>
            {alertBanner.detail && (
              <div className="mt-1 break-all font-mono text-[11px] opacity-75">
                {alertBanner.detail}
              </div>
            )}
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {canManageTenant && isQuotaOrOOMIssue && tab !== "settings" && (
              <Button size="sm" variant="secondary" onClick={() => changeTab("settings", { section: "container" })}>
                前往调整规格
              </Button>
            )}
            {canOperateInstances && alertBanner.kind === "disk" && tab !== "files" && (
              <Button size="sm" variant="secondary" onClick={() => changeTab("files")}>
                前往文件清理
              </Button>
            )}
            {canOperateInstances && isArtifactSyncFailure && hasAddonBinding && tab !== "addons" && (
              <Button size="sm" variant="secondary" onClick={() => changeTab("addons")}>
                前往{addonTabLabel}管理
              </Button>
            )}
            {alertBanner.kind === "crash" && tab !== "console" && (
              <Button size="sm" variant="secondary" onClick={() => changeTab("console")}>
                查看控制台日志
              </Button>
            )}
            {canOperateInstances &&
              !stopped &&
              phase === "Failed" &&
              !isQuotaOrOOMIssue &&
              !isArtifactSyncFailure && (
                <Button
                  size="sm"
                  variant="secondary"
                  loading={state === "restart"}
                  disabled={!!state}
                  onClick={() => void restart(inst)}
                >
                  重试启动
                </Button>
              )}
          </div>
        </div>
      )}

      {!alertBanner &&
        !stopped &&
        !running &&
        (() => {
          const pullingList = startingArtifacts.filter(
            (a) => a.status?.phase === "Pending" || a.status?.phase === "Syncing",
          );
          if (pullingList.length === 0) return null;
          const srvArt = inst.spec.serverArtifact;
          const primary =
            pullingList.find(
              (a) =>
                srvArt &&
                a.spec.repository === srvArt.repository &&
                a.spec.package === srvArt.package &&
                a.spec.version === srvArt.version,
            ) || pullingList[0];
          const isServerPkg = Boolean(
            srvArt &&
              primary.spec.repository === srvArt.repository &&
              primary.spec.package === srvArt.package,
          );
          const matchedAddonTitle = !isServerPkg
            ? inst.spec.addons?.find(
                (a) => a.repository === primary.spec.repository && a.package === primary.spec.package,
              )?.title
            : undefined;
          const targetLabel = isServerPkg
            ? `${game.title} 服务端 (${primary.spec.version})`
            : `${addonTabLabel}《${matchedAddonTitle || primary.spec.package}》`;
          const pct = typeof primary.status?.progress === "number" ? primary.status.progress : 0;
          const livePullModalArt = pullLogTarget
            ? (startingArtifacts.find((a) => a.metadata.name === pullLogTarget.artifact.metadata.name) ??
                pullLogTarget.artifact)
            : null;
          return (
            <>
              <div className="mt-4 flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-[#DCE3FA] bg-[#F5F7FF] px-5 py-3.5 text-sm text-[#14161F]">
                <div className="min-w-0 flex-1 space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold">正在准备启动文件：{targetLabel}</span>
                    {pullingList.length > 1 && (
                      <span className="text-xs text-[#5B6075]">
                        （共 {pullingList.length} 项待同步）
                      </span>
                    )}
                  </div>
                  <div className="flex flex-wrap items-center gap-3">
                    <progress
                      className="gp-progress h-1.5 w-full max-w-xs"
                      max={100}
                      {...(pct > 0 ? { value: Math.min(100, pct) } : {})}
                    />
                    <span className="text-xs text-[#5B6075]">
                      {primary.status?.stage || "正在拉取资源"}
                      {pct > 0 ? ` · ${pct}%` : ""}
                    </span>
                  </div>
                </div>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setPullLogTarget({ artifact: primary, title: targetLabel })}
                >
                  查看拉取日志
                </Button>
              </div>
              <ArtifactLogModal
                open={!!pullLogTarget}
                onClose={() => setPullLogTarget(null)}
                artifact={livePullModalArt}
                title={pullLogTarget?.title}
                token={token}
              />
            </>
          );
        })()}

      <div className="mt-6 mb-6 overflow-x-auto">
        <Segmented<Tab>
          label="服务器详情"
          value={tab}
          onChange={changeTab}
          items={TABS.filter((t) => {
            if (t.value === "addons" && !hasAddonBinding) return false;
            if (
              !canOperateInstances &&
              (t.value === "files" || t.value === "addons" || t.value === "backup" || t.value === "schedules")
            ) {
              return false;
            }
            return true;
          }).map((t) => (t.value === "addons" ? { ...t, label: addonTabLabel } : t))}
        />
      </div>

      <div role="tabpanel" className="min-h-[560px]" aria-label={tab === "addons" ? addonTabLabel : TABS.find((t) => t.value === tab)?.label}>
        {tab === "overview" && <OverviewTab inst={inst} nodes={nodes} />}
        {tab === "console" && (
          <ConsoleTab
            key={name}
            name={name}
            running={running}
            onOpenTerminal={
              canOperateInstances
                ? () => {
                    setTermMode("terminal");
                    setTermOpen(true);
                  }
                : undefined
            }
          />
        )}
        {tab === "files" && canOperateInstances && <FilesTab key={name} name={name} />}
        {tab === "addons" && canOperateInstances && matchedApp?.addonBinding && (
          <AddonsTab key={name} inst={inst} app={matchedApp} running={!stopped} />
        )}
        {tab === "backup" && canOperateInstances && (
          <BackupTab
            key={name}
            inst={inst}
            app={matchedApp ?? undefined}
            running={!stopped}
            onOpenConsole={() => changeTab("console")}
          />
        )}
        {tab === "schedules" && canOperateInstances && <ScheduleTab key={name} inst={inst} running={!stopped} />}
        {tab === "network" && <NetworkTab inst={inst} nodes={nodes} readOnly={!canOperateInstances} />}
        {tab === "settings" && (
          <SettingsTab inst={inst} onDelete={canManageTenant ? () => setConfirmDelete(true) : undefined} readOnly={!canOperateInstances} />
        )}
      </div>

      <FloatingTerminal
        key={`term-${name}`}
        inst={inst}
        app={matchedApp ?? undefined}
        name={name}
        running={running}
        canExec={canOperateInstances}
        open={termOpen}
        onOpenChange={setTermOpen}
        mode={termMode}
        onModeChange={setTermMode}
      />

      <InviteModal open={inviteOpen} onClose={() => setInviteOpen(false)} inst={inst} nodes={nodes} />

      <DeleteServerDialog
        inst={confirmDelete ? inst : null}
        loading={state === "delete"}
        onConfirm={() => void onDelete()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  );
}

export default function ServerDetailPage() {
  return (
    <Suspense fallback={<DetailSkeleton />}>
      <ServerDetail />
    </Suspense>
  );
}
