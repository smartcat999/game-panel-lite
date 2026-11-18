"use client";

import Link from "next/link";
import { Cpu, MemoryStick, Play, Plus, Square, SquareTerminal, Trash2 } from "lucide-react";
import { Card, CopyButton, IconButton, Meter, Mono, Skeleton, StatusPill, focusRing } from "../ui";
import { useConsole } from "../../../console/ConsoleProvider";
import { endpointOf, formatCpu, formatGB, parseNumericQuantity, phaseOf, resourceOf } from "../../../console/lib/format";
import { DEFAULT_GAME_COVER } from "../../../console/lib/presets";
import type { InstanceItem, NodeItem } from "../../../console/lib/types";
import type { ServerBusy } from "./api";
import { formatBytesCompact } from "./InfoTabs";
import { gameOf, IconLink, MoreMenu } from "./shared";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export interface ServerRowProps {
  inst: InstanceItem;
  nodes: NodeItem[];
  busy?: ServerBusy;
  onToggle?: (inst: InstanceItem) => void;
  onDelete?: (inst: InstanceItem) => void;
}

export const serverHref = (name: string) => `/console/servers/${encodeURIComponent(name)}`;

function Address({ inst, nodes }: { inst: InstanceItem; nodes: NodeItem[] }) {
  const { apps } = useConsole();
  const ep = endpointOf(inst, nodes, apps);
  const stopped = inst.spec.state === "Stopped" || phaseOf(inst) === "Stopped";
  if (!ep) {
    return <span className="text-sm text-[#8A90A6]">{stopped ? "启动后分配连接地址" : "地址分配中"}</span>;
  }
  return (
    <span className="inline-flex min-w-0 items-center gap-0.5">
      <Mono className="truncate text-[#14161F]">{ep}</Mono>
      <CopyButton text={ep} label="复制连接地址" />
    </span>
  );
}

function LiveSpecMeters({ inst }: { inst: InstanceItem }) {
  const running = phaseOf(inst) === "Running";
  const usage = inst.status?.usage;

  const cpuLimitCores = Math.max(0.1, parseNumericQuantity(resourceOf(inst, "cpu")) || 2);
  const memLimitGB = Math.max(0.1, parseNumericQuantity(resourceOf(inst, "memory")) || 4);
  const memLimitBytes = memLimitGB * 1024 * 1024 * 1024;

  const cpuUsedCores =
    usage?.cpuMillicores !== undefined && usage.cpuMillicores > 0
      ? usage.cpuMillicores / 1000
      : 0;
  const memUsedBytes =
    usage?.memoryBytes !== undefined && usage.memoryBytes > 0
      ? usage.memoryBytes
      : 0;

  const cpuPct =
    running && cpuUsedCores > 0
      ? Math.min(100, Math.max(1, Math.round((cpuUsedCores / cpuLimitCores) * 100)))
      : 0;
  const memPct =
    running && memUsedBytes > 0
      ? Math.min(100, Math.max(1, Math.round((memUsedBytes / memLimitBytes) * 100)))
      : 0;

  const items = [
    {
      key: "cpu",
      icon: <Cpu className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />,
      used: running ? `${cpuUsedCores.toFixed(2)}` : "0",
      limit: formatCpu(resourceOf(inst, "cpu")),
      pct: cpuPct,
    },
    {
      key: "mem",
      icon: <MemoryStick className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />,
      used: running ? formatBytesCompact(memUsedBytes) : "0 MB",
      limit: formatGB(resourceOf(inst, "memory")),
      pct: memPct,
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-3.5 pt-0.5">
      {items.map((it) => (
        <Meter
          key={it.key}
          label={it.key === "cpu" ? "CPU 使用率" : "内存使用率"}
          used={it.pct}
          max={100}
          className="min-w-0"
          barClassName="mt-1.5"
          headerLeft={
            <span className="inline-flex min-w-0 items-center gap-1 whitespace-nowrap truncate text-[12px]">
              {it.icon}
              <span className="truncate font-medium tabular-nums text-[#14161F]">{it.used}</span>
            </span>
          }
          headerRight={<span className="shrink-0 whitespace-nowrap text-[11px] tabular-nums text-[#8A90A6]">/ {it.limit}</span>}
        />
      ))}
    </div>
  );
}

function ToggleButton({
  inst,
  busy,
  onToggle,
}: {
  inst: InstanceItem;
  busy?: ServerBusy;
  onToggle: (inst: InstanceItem) => void;
}) {
  const stopped = inst.spec.state === "Stopped";
  const working = !!busy;
  return (
    <IconButton
      label={stopped ? "启动" : "停止"}
      disabled={working}
      onClick={() => onToggle(inst)}
      className={stopped ? "bg-[#EEF1FF] text-[#2B3AE8]" : undefined}
    >
      {working ? (
        <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" />
      ) : stopped ? (
        <Play className="h-4 w-4" />
      ) : (
        <Square className="h-3.5 w-3.5" />
      )}
    </IconButton>
  );
}

export function ServerCard({ inst, nodes, busy, onToggle, onDelete }: ServerRowProps) {
  const { apps } = useConsole();
  const game = gameOf(inst, apps);
  const name = inst.metadata.name;
  return (
    <Card className="flex flex-col transition-shadow hover:shadow-[0_20px_40px_-24px_rgba(43,58,232,0.45)]">
      <Link href={serverHref(name)} className={cx("relative block rounded-t-[19px]", focusRing)} tabIndex={-1}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={game.cover}
          onError={(e) => {
            if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
              e.currentTarget.src = DEFAULT_GAME_COVER;
            }
          }}
          alt=""
          className="aspect-[16/9] w-full rounded-t-[19px] object-cover"
          loading="lazy"
        />
        <span className="absolute bottom-3 left-3 rounded-full bg-white/90 px-2.5 py-1 text-xs font-medium text-[#14161F] backdrop-blur">
          {game.title}
        </span>
      </Link>
      <div className="flex flex-1 flex-col gap-3 p-5">
        <div className="flex items-start justify-between gap-3">
          <Link
            href={serverHref(name)}
            className={cx("min-w-0 truncate text-base font-semibold text-[#14161F] hover:text-[#2B3AE8]", focusRing, "rounded")}
          >
            {name}
          </Link>
          <StatusPill phase={phaseOf(inst)} />
        </div>
        <div className="flex min-h-7 items-center">
          <Address inst={inst} nodes={nodes} />
        </div>
        <LiveSpecMeters inst={inst} />
        <div className="mt-auto flex items-center gap-2 pt-2">
          {onToggle && <ToggleButton inst={inst} busy={busy} onToggle={onToggle} />}
          <IconLink href={`${serverHref(name)}?tab=console`} label="打开控制台">
            <SquareTerminal className="h-4 w-4" />
          </IconLink>
          {onDelete && (
            <div className="ml-auto">
              <MoreMenu
                items={[
                  {
                    label: "删除服务器",
                    danger: true,
                    icon: <Trash2 className="h-4 w-4" />,
                    onSelect: () => onDelete(inst),
                  },
                ]}
              />
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

export function NewServerCard() {
  return (
    <Link
      href="/console/new"
      className={cx(
        "flex min-h-[280px] flex-col items-center justify-center gap-3 rounded-[20px] border-2 border-dashed border-[#D5DAEE] bg-white/50 text-[#5B6075] transition-colors hover:border-[#2B3AE8] hover:bg-white hover:text-[#2B3AE8]",
        focusRing,
      )}
    >
      <span className="flex h-11 w-11 items-center justify-center rounded-full bg-[#EEF1FF] text-[#2B3AE8]">
        <Plus className="h-5 w-5" />
      </span>
      <span className="text-[15px] font-medium">新建服务器</span>
    </Link>
  );
}

export function ServerCardSkeleton() {
  return (
    <Card>
      <Skeleton className="aspect-[16/9] w-full rounded-b-none rounded-t-[19px]" />
      <div className="space-y-3 p-5">
        <Skeleton className="h-5 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-4 w-1/3" />
        <div className="flex gap-2 pt-2">
          <Skeleton className="h-9 w-9 rounded-full" />
          <Skeleton className="h-9 w-9 rounded-full" />
        </div>
      </div>
    </Card>
  );
}

export function ServerTable({
  items,
  nodes,
  busy,
  onToggle,
  onDelete,
}: {
  items: InstanceItem[];
  nodes: NodeItem[];
  busy: Record<string, ServerBusy | undefined>;
  onToggle?: (inst: InstanceItem) => void;
  onDelete?: (inst: InstanceItem) => void;
}) {
  const { apps } = useConsole();
  return (
    <Card className="overflow-visible">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[680px] text-left text-sm">
          <thead>
            <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
              <th className="px-6 py-3.5 font-medium">服务器</th>
              <th className="whitespace-nowrap px-4 py-3.5 font-medium">状态</th>
              <th className="px-4 py-3.5 font-medium">连接地址</th>
              <th className="w-72 min-w-[260px] px-4 py-3.5 font-medium">配置</th>
              <th className="px-6 py-3.5 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((inst) => {
              const name = inst.metadata.name;
              const game = gameOf(inst, apps);
              return (
                <tr
                  key={name}
                  className="border-b border-[#EEF0F7] transition-colors last:border-0 hover:bg-[#F6F8FF]"
                >
                  <td className="px-6 py-4">
                    <Link
                      href={serverHref(name)}
                      className={cx("font-semibold text-[#14161F] hover:text-[#2B3AE8]", focusRing, "rounded")}
                    >
                      {name}
                    </Link>
                    <div className="mt-0.5 text-xs text-[#8A90A6]">{game.title}</div>
                  </td>
                  <td className="whitespace-nowrap px-4 py-4">
                    <StatusPill phase={phaseOf(inst)} />
                  </td>
                  <td className="px-4 py-4">
                    <Address inst={inst} nodes={nodes} />
                  </td>
                  <td className="px-4 py-3">
                    <LiveSpecMeters inst={inst} />
                  </td>
                  <td className="px-6 py-4">
                    <div className="flex items-center justify-end gap-2">
                      {onToggle && <ToggleButton inst={inst} busy={busy[name]} onToggle={onToggle} />}
                      <IconLink href={`${serverHref(name)}?tab=console`} label="打开控制台">
                        <SquareTerminal className="h-4 w-4" />
                      </IconLink>
                      {onDelete && (
                        <MoreMenu
                          items={[
                            {
                              label: "删除服务器",
                              danger: true,
                              icon: <Trash2 className="h-4 w-4" />,
                              onSelect: () => onDelete(inst),
                            },
                          ]}
                        />
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
