"use client";

import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { ImagePlus, Plus, RotateCcw, Trash2 } from "lucide-react";
import { Badge, Button, Card, CardHeader, CopyButton, Field, IconButton, Input, Meter, Modal, Mono, Segmented, Select, buttonClass } from "../ui";
import { useConsole } from "../../../console/ConsoleProvider";
import { endpointOf, formatCpu, formatGB, formatRateBytes, parseNumericQuantity, phaseOf, relativeTime, resourceOf } from "../../../console/lib/format";
import {
  DEFAULT_GAME_COVER,
  compileAppConfig,
  compressCoverImageFile,
  defaultAppConfigValues,
  normalizeConfigFieldValue,
  resolveRichApp,
  resolveServerArtifact,
} from "../../../console/lib/presets";
import type { AppItem, InstanceItem, InstancePort, NodeItem } from "../../../console/lib/types";
import { hostOf, portsOf, useServerActions } from "./api";
import { GameConfigEditor } from "./GameConfigEditor";
import {
  INSTANCE_APP_CONFIG_ANNOTATION,
  INSTANCE_APP_VERSION_ANNOTATION,
  INSTANCE_COVER_ANNOTATION,
  INSTANCE_RESTART_REQUESTED_AT_ANNOTATION,
  gameOf,
} from "./shared";

function InfoList({ rows }: { rows: Array<[string, ReactNode]> }) {
  return (
    <dl className="divide-y divide-[#EEF0F7]">
      {rows.map(([k, v]) => (
        <div key={k} className="flex flex-col gap-1 px-6 py-4 sm:flex-row sm:items-center sm:gap-6">
          <dt className="w-32 shrink-0 text-[13px] text-[#8A90A6]">{k}</dt>
          <dd className="min-w-0 flex-1 break-words text-sm text-[#14161F]">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

function formatTime(iso?: string) {
  if (!iso) return "—";
  const d = new Date(iso);
  return `${d.toLocaleString("zh-CN", { hour12: false })}（${relativeTime(iso)}）`;
}

export function PortsTable({
  inst,
  nodes,
  onRemovePort,
  busy,
}: {
  inst: InstanceItem;
  nodes: NodeItem[];
  onRemovePort?: (targetPort: number, protocol: string) => void;
  busy?: boolean;
}) {
  const { apps } = useConsole();
  const host = hostOf(inst, nodes);
  const ports = portsOf(inst, apps);
  const stopped = phaseOf(inst) === "Stopped";
  if (!ports.length) return <p className="px-6 py-6 text-sm text-[#8A90A6]">未配置对外开放端口。</p>;
  return (
    <div tabIndex={0} role="region" aria-label="端口映射列表" className="overflow-x-auto">
      <table className="w-full min-w-[560px] text-left text-sm">
        <thead>
          <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
            <th scope="col" className="px-6 py-3.5 font-medium">名称</th>
            <th scope="col" className="px-4 py-3.5 font-medium">协议</th>
            <th scope="col" className="px-4 py-3.5 font-medium">端口映射</th>
            <th scope="col" className="px-6 py-3.5 font-medium">连接地址</th>
            {onRemovePort && ports.length > 1 && <th scope="col" className="w-16 px-6 py-3.5 text-right font-medium" />}
          </tr>
        </thead>
        <tbody>
          {ports.map((p, idx) => {
            const addr = host && p.publishedPort ? `${host}:${p.publishedPort}` : "";
            const proto = (p.protocol || "TCP").toUpperCase();
            return (
              <tr key={`${p.name}-${p.targetPort}-${proto}`} className="border-b border-[#EEF0F7] last:border-0">
                <td className="px-6 py-4 text-[#14161F]">
                  <span className="inline-flex items-center gap-2">
                    <span>{p.name || "—"}</span>
                    {idx === 0 && ports.length > 1 && <Badge tone="neutral">主连接端口</Badge>}
                  </span>
                </td>
                <td className="px-4 py-4 text-[#5B6075]">{proto}</td>
                <td className="px-4 py-4">
                  <span className="inline-flex items-center gap-1.5">
                    <Mono>{p.targetPort}</Mono>
                    <span className="text-[#8A90A6]">→</span>
                    {p.publishedPort ? (
                      <Mono>{p.publishedPort}</Mono>
                    ) : (
                      <span className="text-[#8A90A6]">{stopped ? "动态分配" : "分配中"}</span>
                    )}
                  </span>
                </td>
                <td className="px-6 py-4">
                  {addr ? (
                    <span className="inline-flex items-center gap-0.5">
                      <Mono>{addr}</Mono>
                      <CopyButton text={addr} label="复制连接地址" />
                    </span>
                  ) : (
                    <span className="text-[#8A90A6]">{stopped ? "启动后分配" : "地址分配中"}</span>
                  )}
                </td>
                {onRemovePort && ports.length > 1 && (
                  <td className="px-6 py-4 text-right">
                    {idx > 0 && (
                      <IconButton
                        label={`删除端口 ${p.targetPort}`}
                        disabled={busy}
                        onClick={() => onRemovePort(p.targetPort, proto)}
                        className="bg-transparent hover:bg-[#FEF3F2] hover:text-[#D92D20]"
                      >
                        <Trash2 className="h-4 w-4" />
                      </IconButton>
                    )}
                  </td>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function formatBytesCompact(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 MB";
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 0.1) return `${gb.toFixed(2)} GB`;
  const mb = bytes / (1024 * 1024);
  if (mb >= 1) return `${mb.toFixed(1)} MB`;
  const kb = bytes / 1024;
  return `${Math.max(1, Math.round(kb))} KB`;
}

function SpecTiles({ inst }: { inst: InstanceItem }) {
  const running = phaseOf(inst) === "Running";
  const usage = inst.status?.usage;

  const cpuLimitCores = Math.max(0.1, parseNumericQuantity(resourceOf(inst, "cpu")) || 2);
  const memLimitGB = Math.max(0.1, parseNumericQuantity(resourceOf(inst, "memory")) || 4);
  const diskLimitGB = Math.max(1, parseNumericQuantity(resourceOf(inst, "storage")) || 20);

  const memLimitBytes = memLimitGB * 1024 * 1024 * 1024;
  const diskLimitBytes = diskLimitGB * 1024 * 1024 * 1024;

  const cpuUsedCores =
    usage?.cpuMillicores !== undefined && usage.cpuMillicores > 0
      ? usage.cpuMillicores / 1000
      : 0;
  const memUsedBytes =
    usage?.memoryBytes !== undefined && usage.memoryBytes > 0
      ? usage.memoryBytes
      : 0;
  const diskUsedBytes =
    usage?.storageBytes !== undefined && usage.storageBytes > 0
      ? usage.storageBytes
      : 0;

  const cpuPct =
    running && cpuUsedCores > 0
      ? Math.min(100, Math.max(1, Math.round((cpuUsedCores / cpuLimitCores) * 100)))
      : 0;
  const memPct =
    running && memUsedBytes > 0
      ? Math.min(100, Math.max(1, Math.round((memUsedBytes / memLimitBytes) * 100)))
      : 0;
  const diskPct =
    diskUsedBytes > 0
      ? Math.min(100, Math.max(1, Math.round((diskUsedBytes / diskLimitBytes) * 100)))
      : 0;
  const rxRate = running ? (usage?.networkRxBytesPerSec ?? 0) : 0;
  const txRate = running ? (usage?.networkTxBytesPerSec ?? 0) : 0;

  const tiles = [
    {
      label: "CPU 负载",
      usedText: running ? `${cpuUsedCores.toFixed(2)} 核` : "0 核",
      limitText: formatCpu(resourceOf(inst, "cpu")),
      pct: cpuPct,
    },
    {
      label: "内存占用",
      usedText: running ? formatBytesCompact(memUsedBytes) : "0 MB",
      limitText: formatGB(resourceOf(inst, "memory")),
      pct: memPct,
    },
    {
      label: "存储空间",
      usedText: formatBytesCompact(diskUsedBytes),
      limitText: formatGB(resourceOf(inst, "storage")),
      pct: diskPct,
    },
  ];

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-4">
      {tiles.map((t) => (
        <Card key={t.label} className="px-5 py-4">
          <div className="flex items-center justify-between gap-2">
            <span className="text-[13px] font-medium text-[#5B6075]">{t.label}</span>
            <Badge tone={t.pct >= 90 ? "danger" : t.pct >= 75 ? "warning" : "neutral"}>
              <span className="font-mono tabular-nums">{t.pct}%</span>
            </Badge>
          </div>

          <div className="mt-2 flex items-baseline gap-1.5 tabular-nums">
            <span className="text-xl font-semibold text-[#14161F]">{t.usedText}</span>
            <span className="text-xs text-[#8A90A6]">/ {t.limitText}</span>
          </div>

          <Meter label={t.label} used={t.pct} max={100} hideHeader barClassName="mt-3" />
        </Card>
      ))}

      <Card className="px-5 py-4">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[13px] font-medium text-[#5B6075]">网络速率</span>
          <Badge tone={running ? "success" : "neutral"}>{running ? "实时" : "离线"}</Badge>
        </div>

        <div className="mt-2 flex items-baseline gap-1.5 tabular-nums">
          <span className="text-xl font-semibold text-[#14161F]">↓ {formatRateBytes(rxRate)}</span>
          <span className="text-xs text-[#8A90A6]">/ 下行</span>
        </div>

        <div className="mt-2.5 flex items-center justify-between gap-2 text-xs tabular-nums">
          <span className="text-[#8A90A6]">上行速率</span>
          <span className="font-mono text-[#5B6075]">↑ {formatRateBytes(txRate)}</span>
        </div>
      </Card>
    </div>
  );
}

export function OverviewTab({ inst, nodes }: { inst: InstanceItem; nodes: NodeItem[] }) {
  const { apps } = useConsole();
  const endpoint = endpointOf(inst, nodes, apps);
  const command = [...(inst.spec.command || []), ...(inst.spec.args || [])].join(" ");
  return (
    <div className="space-y-6">
      <SpecTiles inst={inst} />
      <Card>
        <CardHeader title="基本信息" />
        <InfoList
          rows={[
            ["游戏应用", gameOf(inst, apps).title],
            [
              "连接地址",
              endpoint ? (
                <span className="inline-flex items-center gap-0.5">
                  <Mono>{endpoint}</Mono>
                  <CopyButton text={endpoint} label="复制连接地址" />
                </span>
              ) : (
                <span className="text-[#8A90A6]">待分配</span>
              ),
            ],
            ["计算节点", inst.spec.nodeName || "等待调度"],
            ...(inst.spec.serverArtifact
              ? ([
                  [
                    "服务端版本",
                    <Mono key="srv-art">
                      {inst.spec.serverArtifact.version}（{inst.spec.serverArtifact.repository}/{inst.spec.serverArtifact.package}）
                    </Mono>,
                  ],
                ] as Array<[string, ReactNode]>)
              : []),
            ["镜像地址", inst.spec.image ? <Mono>{inst.spec.image}</Mono> : "—"],
            ...(command ? ([["启动命令", <Mono key="cmd">{command}</Mono>]] as Array<[string, ReactNode]>) : []),
            ...(inst.spec.workingDir
              ? ([["工作目录", <Mono key="wd">{inst.spec.workingDir}</Mono>]] as Array<[string, ReactNode]>)
              : []),
            ["创建时间", formatTime(inst.metadata.creationTimestamp)],
          ]}
        />
      </Card>
    </div>
  );
}

export function NetworkTab({
  inst,
  nodes,
  readOnly,
}: {
  inst: InstanceItem;
  nodes: NodeItem[];
  readOnly?: boolean;
}) {
  const host = hostOf(inst, nodes);
  const { busy, updateSpec } = useServerActions();
  const isUpdating = busy[inst.metadata.name] === "update";

  const [addOpen, setAddOpen] = useState(false);
  const [portName, setPortName] = useState("");
  const [protocol, setProtocol] = useState<"TCP" | "UDP">("TCP");
  const [targetPort, setTargetPort] = useState("");
  const [publishedPort, setPublishedPort] = useState("");

  async function handleAddPort(e: FormEvent) {
    e.preventDefault();
    const tp = Number(targetPort);
    if (!Number.isInteger(tp) || tp < 1 || tp > 65535) return;
    const pub = publishedPort.trim() ? Number(publishedPort) : 0;
    const current = inst.spec.ports || [];
    const cleanName =
      portName.trim().toLowerCase().replace(/[^a-z0-9-]/g, "-") || `port-${tp}`;

    const nextPorts: InstancePort[] = [
      ...current.filter((p) => !(p.targetPort === tp && (p.protocol || "TCP").toUpperCase() === protocol)),
      {
        name: cleanName,
        protocol,
        targetPort: tp,
        ...(pub > 0 ? { publishedPort: pub } : {}),
      },
    ];

    const ok = await updateSpec(inst, { ports: nextPorts }, `已添加端口 ${tp}/${protocol}`);
    if (ok) {
      setAddOpen(false);
      setPortName("");
      setTargetPort("");
      setPublishedPort("");
    }
  }

  async function handleRemovePort(tp: number, proto: string) {
    const current = inst.spec.ports || [];
    if (current.length <= 1) return;
    const nextPorts = current.filter(
      (p) => !(p.targetPort === tp && (p.protocol || "TCP").toUpperCase() === proto.toUpperCase()),
    );
    await updateSpec(inst, { ports: nextPorts }, `已移除端口 ${tp}/${proto}`);
  }

  return (
    <>
      <Card>
        <CardHeader
          title="端口映射"
          description={
            <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px]">
              <span>计算节点 {inst.spec.nodeName || "等待调度"}</span>
              <span>·</span>
              {host ? (
                <span className="inline-flex items-center gap-0.5">
                  <span>主机地址 </span>
                  <Mono>{host}</Mono>
                  <CopyButton text={host} label="复制 IP" />
                </span>
              ) : (
                <span>主机地址待分配</span>
              )}
            </span>
          }
          actions={
            !readOnly ? (
              <Button size="sm" icon={<Plus className="h-3.5 w-3.5" />} onClick={() => setAddOpen(true)}>
                添加端口
              </Button>
            ) : undefined
          }
        />
        <PortsTable inst={inst} nodes={nodes} onRemovePort={!readOnly ? handleRemovePort : undefined} busy={isUpdating} />
      </Card>

      <Modal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        title="添加端口"
        footer={
          <>
            <Button variant="ghost" onClick={() => setAddOpen(false)}>
              取消
            </Button>
            <Button
              variant="primary"
              loading={isUpdating}
              disabled={!targetPort.trim() || isUpdating}
              onClick={(e) => void handleAddPort(e as unknown as FormEvent)}
            >
              添加
            </Button>
          </>
        }
      >
        <form onSubmit={handleAddPort} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <Field label="名称" htmlFor="port-name">
              <Input
                id="port-name"
                placeholder="例如 rcon"
                value={portName}
                onChange={(e) => setPortName(e.target.value)}
              />
            </Field>
            <Field label="协议" htmlFor="port-proto">
              <Select
                id="port-proto"
                value={protocol}
                onChange={(e) => setProtocol(e.target.value as "TCP" | "UDP")}
              >
                <option value="TCP">TCP</option>
                <option value="UDP">UDP</option>
              </Select>
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="游戏端口" htmlFor="port-target">
              <Input
                id="port-target"
                type="number"
                min={1}
                max={65535}
                placeholder="25575"
                value={targetPort}
                onChange={(e) => setTargetPort(e.target.value)}
                className="font-mono"
              />
            </Field>
            <Field label="对外端口" htmlFor="port-pub">
              <Input
                id="port-pub"
                type="number"
                min={1024}
                max={65535}
                placeholder="自动分配"
                value={publishedPort}
                onChange={(e) => setPublishedPort(e.target.value)}
                className="font-mono"
              />
            </Field>
          </div>
        </form>
      </Modal>
    </>
  );
}

export function SettingsTab({
  inst,
  onDelete,
  readOnly,
}: {
  inst: InstanceItem;
  onDelete?: () => void;
  readOnly?: boolean;
}) {
  const { quota, apps, toast } = useConsole();
  const { busy, updateSpec } = useServerActions();
  const isUpdating = busy[inst.metadata.name] === "update";

  const game = useMemo(() => gameOf(inst, apps), [apps, inst]);
  const matchedApp: AppItem | undefined = useMemo(() => {
    const presetId = inst.metadata.labels?.["gamepanel.io/preset"] || game.id;
    return resolveRichApp(presetId, apps);
  }, [apps, game.id, inst]);

  const searchParams = useSearchParams();
  const hasGameSchema = (matchedApp?.configSchema?.length || 0) > 0;
  const canDelete = !readOnly && Boolean(onDelete);
  const sectionParam = searchParams.get("section");
  const [subTab, setSubTab] = useState<"game" | "container" | "cover" | "danger">(
    sectionParam === "container" || sectionParam === "cover"
      ? sectionParam
      : sectionParam === "danger" && canDelete
        ? "danger"
        : hasGameSchema
          ? "game"
          : "container",
  );

  useEffect(() => {
    if (sectionParam === "container") setSubTab("container");
    else if (sectionParam === "cover") setSubTab("cover");
    else if (sectionParam === "danger" && canDelete) setSubTab("danger");
  }, [sectionParam, canDelete]);

  useEffect(() => {
    if (!hasGameSchema && subTab === "game") {
      setSubTab("container");
    } else if (!canDelete && subTab === "danger") {
      setSubTab(hasGameSchema ? "game" : "container");
    }
  }, [hasGameSchema, canDelete, subTab]);

  const parseInitialGameValues = (item: InstanceItem, app?: AppItem): Record<string, string> => {
    if (!app) return {};
    const defaults = defaultAppConfigValues(app);
    const rawEnv = item.spec.env as unknown;
    const envRecord: Record<string, string> = Array.isArray(rawEnv)
      ? Object.fromEntries((rawEnv as Array<{ name: string; value: string }>).map((e) => [e.name, e.value]))
      : ((rawEnv as Record<string, string>) || {});

    let saved: Record<string, string> = {};
    const rawSaved = item.metadata.annotations?.[INSTANCE_APP_CONFIG_ANNOTATION];
    if (rawSaved) {
      try {
        saved = JSON.parse(rawSaved);
      } catch {
        /* ignore */
      }
    }
    const merged: Record<string, string> = { ...defaults, ...saved };
    for (const f of app.configSchema || []) {
      if (f.envKey && envRecord[f.envKey] !== undefined && saved[f.name] === undefined) {
        merged[f.name] = normalizeConfigFieldValue(f, envRecord[f.envKey]);
      } else if (merged[f.name] !== undefined) {
        merged[f.name] = normalizeConfigFieldValue(f, merged[f.name]);
      }
    }
    return merged;
  };

  const initialGameValues = useMemo(() => parseInitialGameValues(inst, matchedApp), [inst, matchedApp]);
  const initialAppVersion =
    inst.metadata.annotations?.[INSTANCE_APP_VERSION_ANNOTATION] ||
    matchedApp?.recommendedVersion ||
    matchedApp?.versions?.[0] ||
    "";

  const [gameValues, setGameValues] = useState<Record<string, string>>(initialGameValues);
  const [appVersion, setAppVersion] = useState<string>(initialAppVersion);

  const parseEnvEntries = (raw: unknown): Array<{ key: string; value: string }> => {
    if (Array.isArray(raw)) {
      return (raw as Array<{ name: string; value: string }>).map((x) => ({ key: x.name, value: x.value }));
    }
    return Object.entries((raw as Record<string, string>) || {}).map(([key, value]) => ({ key, value }));
  };

  const extractCommandStr = (item: InstanceItem): string => {
    const cmd = item.spec.command || [];
    const args = item.spec.args || [];
    if (cmd.length === 3 && cmd[0] === "sh" && cmd[1] === "-c" && args.length === 0) {
      return cmd[2];
    }
    if (cmd.length === 2 && cmd[0] === "sh" && cmd[1] === "-c" && args.length === 1) {
      return args[0];
    }
    return [...cmd, ...args].join(" ");
  };

  const initialCpu = String(Math.max(1, parseNumericQuantity(resourceOf(inst, "cpu")) || 2));
  const initialMem = String(Math.max(1, parseNumericQuantity(resourceOf(inst, "memory")) || 4));
  const initialDisk = String(Math.max(5, parseNumericQuantity(resourceOf(inst, "storage")) || 20));
  const initialCmd = extractCommandStr(inst);
  const initialEnv = parseEnvEntries(inst.spec.env);
  const initialCover = inst.metadata.annotations?.[INSTANCE_COVER_ANNOTATION]?.trim() || "";

  const [cpuCores, setCpuCores] = useState(initialCpu);
  const [memGB, setMemGB] = useState(initialMem);
  const [diskGB, setDiskGB] = useState(initialDisk);
  const [commandStr, setCommandStr] = useState(initialCmd);
  const [envRows, setEnvRows] = useState<Array<{ key: string; value: string }>>(initialEnv);
  const [customCover, setCustomCover] = useState<string>(initialCover);

  useEffect(() => {
    setCpuCores(String(Math.max(1, parseNumericQuantity(resourceOf(inst, "cpu")) || 2)));
    setMemGB(String(Math.max(1, parseNumericQuantity(resourceOf(inst, "memory")) || 4)));
    setDiskGB(String(Math.max(5, parseNumericQuantity(resourceOf(inst, "storage")) || 20)));
    setCommandStr(extractCommandStr(inst));
    setEnvRows(parseEnvEntries(inst.spec.env));
    setCustomCover(inst.metadata.annotations?.[INSTANCE_COVER_ANNOTATION]?.trim() || "");
    setGameValues(parseInitialGameValues(inst, matchedApp));
    setAppVersion(
      inst.metadata.annotations?.[INSTANCE_APP_VERSION_ANNOTATION] ||
        matchedApp?.recommendedVersion ||
        matchedApp?.versions?.[0] ||
        "",
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inst.metadata.name, inst.metadata.generation, inst.metadata.resourceVersion, matchedApp?.metadata.name]);

  const curCpuNum = Math.max(1, parseNumericQuantity(resourceOf(inst, "cpu")) || 2);
  const curMemNum = Math.max(1, parseNumericQuantity(resourceOf(inst, "memory")) || 4);
  const curDiskNum = Math.max(5, parseNumericQuantity(resourceOf(inst, "storage")) || 20);
  const maxCpuAllowed = Math.max(curCpuNum, quota.cpu.max - quota.cpu.used + curCpuNum);
  const maxMemAllowed = Math.max(curMemNum, quota.memory.max - quota.memory.used + curMemNum);
  const maxDiskAllowed = Math.max(curDiskNum, quota.storage.max - quota.storage.used + curDiskNum);

  const cpuError = Number(cpuCores) > maxCpuAllowed ? `超出剩余 CPU 配额（最多可用 ${maxCpuAllowed} 核）` : undefined;
  const memError = Number(memGB) > maxMemAllowed ? `超出剩余内存配额（最多可用 ${maxMemAllowed} GB）` : undefined;
  const diskError = Number(diskGB) > maxDiskAllowed ? `超出剩余存储配额（最多可用 ${maxDiskAllowed} GB）` : undefined;
  const quotaExceeded = !!(cpuError || memError || diskError);

  const gameDirty =
    hasGameSchema &&
    (JSON.stringify(gameValues) !== JSON.stringify(initialGameValues) || appVersion !== initialAppVersion);

  const coverDirty = customCover.trim() !== initialCover;
  const dirty =
    gameDirty ||
    coverDirty ||
    cpuCores !== initialCpu ||
    memGB !== initialMem ||
    diskGB !== initialDisk ||
    commandStr !== initialCmd ||
    JSON.stringify(envRows) !== JSON.stringify(initialEnv);

  async function handleSaveSettings(e?: FormEvent) {
    e?.preventDefault();
    if (!dirty || isUpdating || quotaExceeded) return;
    const envMap: Record<string, string> = {};
    for (const r of envRows) {
      const k = r.key.trim();
      if (k) envMap[k] = r.value;
    }

    const compiled =
      matchedApp && hasGameSchema ? compileAppConfig(matchedApp, appVersion, gameValues, envMap) : null;
    const finalEnv = compiled ? compiled.env : envMap;
    let finalFiles = inst.spec.files;
    if (compiled && compiled.files.length > 0) {
      const renderedPaths = new Set(compiled.files.map((f) => f.path));
      const preservedCustom = (inst.spec.files || []).filter((f) => !renderedPaths.has(f.path));
      finalFiles = [...compiled.files, ...preservedCustom];
    }
    const finalImage =
      compiled && appVersion !== initialAppVersion ? compiled.image : inst.spec.image;
    const nextServerArtifact =
      matchedApp?.serverBinding && appVersion
        ? resolveServerArtifact(matchedApp, appVersion)
        : inst.spec.serverArtifact;

    const trimmedCmd = commandStr.trim();
    const cmdChanged = commandStr !== initialCmd;
    const nextCommand = cmdChanged ? (trimmedCmd ? ["sh", "-c", trimmedCmd] : []) : inst.spec.command;
    const nextArgs = cmdChanged ? [] : inst.spec.args;
    const cpuVal = `${Math.max(1, Number(cpuCores) || 2)}`;
    const memVal = `${Math.max(1, Number(memGB) || 4)}Gi`;
    const diskVal = `${Math.max(5, Number(diskGB) || 20)}Gi`;

    const onlyCoverChanged =
      coverDirty &&
      !gameDirty &&
      cpuCores === initialCpu &&
      memGB === initialMem &&
      diskGB === initialDisk &&
      !cmdChanged &&
      JSON.stringify(envRows) === JSON.stringify(initialEnv);

    const annotationsPatch: Record<string, string> = {};
    if (hasGameSchema) {
      annotationsPatch[INSTANCE_APP_CONFIG_ANNOTATION] = JSON.stringify(gameValues);
      if (appVersion) annotationsPatch[INSTANCE_APP_VERSION_ANNOTATION] = appVersion;
    }
    if (coverDirty) {
      annotationsPatch[INSTANCE_COVER_ANNOTATION] = customCover.trim();
    }
    if (inst.spec.state === "Running" && !onlyCoverChanged) {
      annotationsPatch[INSTANCE_RESTART_REQUESTED_AT_ANNOTATION] = new Date().toISOString();
    }

    await updateSpec(
      inst,
      {
        image: finalImage,
        command: nextCommand,
        args: nextArgs,
        ...(nextServerArtifact ? { serverArtifact: nextServerArtifact } : {}),
        env: finalEnv,
        ...(finalFiles ? { files: finalFiles } : {}),
        resources: {
          requests: { cpu: cpuVal, memory: memVal, storage: diskVal },
          limits: { cpu: cpuVal, memory: memVal, storage: diskVal },
        },
      },
      appVersion !== initialAppVersion
        ? "已更新游戏版本与参数，服务器正在平滑升级重启"
        : onlyCoverChanged
          ? "已更新服务器封面"
          : "已保存配置，服务器正在自动应用新配置",
      Object.keys(annotationsPatch).length > 0 ? annotationsPatch : undefined,
    );
  }

  return (
    <Card>
      <div className="flex flex-col gap-3 border-b border-[#EEF0F7] px-4 py-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:px-6">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-base font-semibold text-[#14161F]">服务器配置</h2>
          {!readOnly && subTab !== "danger" && (
            <Button
              size="sm"
              variant="primary"
              className="sm:hidden"
              loading={isUpdating}
              disabled={!dirty || isUpdating || quotaExceeded}
              onClick={() => void handleSaveSettings()}
            >
              保存
            </Button>
          )}
        </div>
        <div className="flex min-w-0 max-w-full items-center gap-2.5">
          <Segmented
            label="配置视图"
            value={subTab}
            onChange={setSubTab}
            className="w-full justify-between sm:w-auto sm:justify-start"
            items={[
              ...(hasGameSchema ? [{ value: "game" as const, label: "游戏参数" }] : []),
              { value: "container" as const, label: "资源规格" },
              { value: "cover" as const, label: "服务器封面" },
              ...(canDelete ? [{ value: "danger" as const, label: "危险操作" }] : []),
            ]}
          />
          {!readOnly && subTab !== "danger" && (
            <Button
              size="sm"
              variant="primary"
              className="hidden sm:inline-flex"
              loading={isUpdating}
              disabled={!dirty || isUpdating || quotaExceeded}
              onClick={() => void handleSaveSettings()}
            >
              保存
            </Button>
          )}
        </div>
      </div>
      {hasGameSchema && subTab === "game" && matchedApp ? (
        <div className="px-6 py-5">
          <GameConfigEditor
            app={matchedApp}
            values={gameValues}
            onChange={setGameValues}
            version={appVersion}
            onVersionChange={setAppVersion}
            addons={inst.spec.addons}
            disabled={readOnly}
          />
        </div>
      ) : subTab === "cover" ? (
        <div className="flex flex-col gap-6 px-6 py-6 sm:flex-row sm:items-center">
          <div className="relative aspect-[16/9] w-full shrink-0 overflow-hidden rounded-xl bg-[#14161F] ring-1 ring-[#14161F]/10 sm:w-56">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={customCover.trim() || game.defaultCover}
              onError={(e) => {
                if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                  e.currentTarget.src = DEFAULT_GAME_COVER;
                }
              }}
              alt=""
              className="h-full w-full object-cover"
            />
          </div>
          <div className="min-w-0 flex-1 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-sm font-semibold text-[#14161F]">自定义服务器封面</div>
              {!readOnly && (
                <div className="flex items-center gap-2">
                  <label className={buttonClass("secondary", "sm", "cursor-pointer")}>
                    <ImagePlus className="h-3.5 w-3.5" aria-hidden />
                    <span>上传图片</span>
                    <input
                      type="file"
                      accept="image/*"
                      className="sr-only"
                      onChange={async (e) => {
                        const file = e.target.files?.[0];
                        e.target.value = "";
                        if (!file) return;
                        try {
                          const dataUrl = await compressCoverImageFile(file);
                          setCustomCover(dataUrl);
                          toast("success", "已载入新封面，点击右上角「保存」生效");
                        } catch (err) {
                          toast("error", err instanceof Error ? err.message : "图片处理失败");
                        }
                      }}
                    />
                  </label>
                  {customCover && (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      icon={<RotateCcw className="h-3.5 w-3.5" />}
                      onClick={() => setCustomCover("")}
                    >
                      恢复默认封面
                    </Button>
                  )}
                </div>
              )}
            </div>
            <Input
              disabled={readOnly}
              placeholder={game.defaultCover}
              value={customCover.startsWith("data:image/") ? "" : customCover}
              onChange={(e) => setCustomCover(e.target.value)}
              className="font-mono text-xs"
            />
            {customCover.startsWith("data:image/") && (
              <div className="text-xs text-[#2B3AE8]">已选择本地上传封面，保存后生效</div>
            )}
          </div>
        </div>
      ) : subTab === "danger" && canDelete ? (
        <div className="flex flex-wrap items-center justify-between gap-4 px-6 py-6">
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-[#B42318]">删除服务器</h3>
            <p className="mt-1 text-sm text-[#5B6075]">删除后世界存档和所有文件都会被清除，且无法恢复。</p>
          </div>
          <Button variant="danger" icon={<Trash2 className="h-4 w-4" />} onClick={onDelete}>
            删除服务器
          </Button>
        </div>
      ) : (
        <form onSubmit={(e) => void handleSaveSettings(e)} className="space-y-5 px-6 py-5">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="CPU" htmlFor="cfg-cpu" error={cpuError}>
              <Select id="cfg-cpu" disabled={readOnly} value={cpuCores} onChange={(e) => setCpuCores(e.target.value)}>
                {[1, 2, 4, 8].map((v) => {
                  const over = v > maxCpuAllowed;
                  return (
                    <option key={v} value={String(v)} disabled={over}>
                      {v} 核{over ? "（超出剩余配额）" : ""}
                    </option>
                  );
                })}
              </Select>
            </Field>
            <Field label="内存" htmlFor="cfg-mem" error={memError}>
              <Select
                id="cfg-mem"
                disabled={readOnly}
                value={memGB}
                onChange={(e) => {
                  const nextMem = e.target.value;
                  setMemGB(nextMem);
                  const heapMB = Math.max(512, Math.round((Number(nextMem) || 2) * 1024 * 0.75));
                  setEnvRows((prev) =>
                    prev.map((r) => (r.key.trim().toUpperCase() === "MEMORY" ? { ...r, value: `${heapMB}M` } : r)),
                  );
                }}
              >
                {[2, 4, 8, 16].map((v) => {
                  const over = v > maxMemAllowed;
                  return (
                    <option key={v} value={String(v)} disabled={over}>
                      {v} GB{over ? "（超出剩余配额）" : ""}
                    </option>
                  );
                })}
              </Select>
            </Field>
            <Field label="存储" htmlFor="cfg-disk" error={diskError}>
              <Select id="cfg-disk" disabled={readOnly} value={diskGB} onChange={(e) => setDiskGB(e.target.value)}>
                {[10, 20, 40, 80].map((v) => {
                  const over = v > maxDiskAllowed;
                  return (
                    <option key={v} value={String(v)} disabled={over}>
                      {v} GB{over ? "（超出剩余配额）" : ""}
                    </option>
                  );
                })}
              </Select>
            </Field>
          </div>

          <Field label="启动命令" htmlFor="cfg-cmd">
            <Input
              id="cfg-cmd"
              disabled={readOnly}
              placeholder="默认"
              value={commandStr}
              onChange={(e) => setCommandStr(e.target.value)}
              className="font-mono text-[13px]"
              spellCheck={false}
            />
          </Field>

          <div className="border-t border-[#EEF0F7] pt-4">
            <div className="flex items-center justify-between gap-3">
              <span className="text-[13px] font-medium text-[#3D4258]">环境变量</span>
              {!readOnly && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  icon={<Plus className="h-3.5 w-3.5" />}
                  onClick={() => setEnvRows((prev) => [...prev, { key: "", value: "" }])}
                >
                  添加变量
                </Button>
              )}
            </div>
            {envRows.length === 0 ? (
              <p className="mt-2 text-[13px] text-[#8A90A6]">没有环境变量。</p>
            ) : (
              <div className="mt-2 space-y-2">
                {envRows.map((row, idx) => (
                  <div key={idx} className="flex items-center gap-2">
                    <div className="w-[40%] min-w-0">
                      <Input
                        placeholder="名称"
                        disabled={readOnly}
                        value={row.key}
                        spellCheck={false}
                        onChange={(e) =>
                          setEnvRows((prev) => prev.map((r, i) => (i === idx ? { ...r, key: e.target.value } : r)))
                        }
                        className="font-mono text-[13px]"
                      />
                    </div>
                    <div className="min-w-0 flex-1">
                      <Input
                        placeholder="值"
                        disabled={readOnly}
                        value={row.value}
                        spellCheck={false}
                        onChange={(e) =>
                          setEnvRows((prev) => prev.map((r, i) => (i === idx ? { ...r, value: e.target.value } : r)))
                        }
                        className="font-mono text-[13px]"
                      />
                    </div>
                    {!readOnly && (
                      <IconButton
                        label={`删除变量 ${row.key || idx + 1}`}
                        onClick={() => setEnvRows((prev) => prev.filter((_, i) => i !== idx))}
                      >
                        <Trash2 className="h-4 w-4" />
                      </IconButton>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </form>
      )}
    </Card>
  );
}
