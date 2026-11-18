"use client";

import { FormEvent, useMemo, useState } from "react";
import { Pencil, Play, Plus, Terminal, Trash2 } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import {
  Badge,
  Button,
  Card,
  CardHeader,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  Modal,
  Mono,
  MoreMenu,
  Segmented,
  Select,
  Switch,
  Textarea,
  type MenuItem,
} from "../ui";
import { TaskLogModal } from "../shared/TaskLogModal";
import { relativeTime } from "../../../console/lib/format";
import { resolveRichApp } from "../../../console/lib/presets";
import type { InstanceItem, InstanceSchedule } from "../../../console/lib/types";
import { useServerActions } from "./api";

const INTERVAL_OPTIONS: Array<{ value: number; label: string }> = [
  { value: 15, label: "每 15 分钟" },
  { value: 30, label: "每 30 分钟" },
  { value: 60, label: "每 1 小时" },
  { value: 180, label: "每 3 小时" },
  { value: 360, label: "每 6 小时" },
  { value: 720, label: "每 12 小时" },
  { value: 1440, label: "每 24 小时" },
];

function formatScheduleTrigger(s: InstanceSchedule): string {
  if (s.dailyAt?.trim()) {
    return `每日 ${s.dailyAt.trim()}`;
  }
  const mins = s.intervalMinutes || 60;
  const match = INTERVAL_OPTIONS.find((o) => o.value === mins);
  if (match) return match.label;
  if (mins >= 60 && mins % 60 === 0) return `每 ${mins / 60} 小时`;
  return `每 ${mins} 分钟`;
}

function formatActionLabel(s: InstanceSchedule): string {
  switch (s.action) {
    case "restart":
      return "例行重启实例";
    case "backup":
      return s.retainCount && s.retainCount > 0 ? `创建存档备份 · 保留最近 ${s.retainCount} 份` : "创建存档备份";
    default:
      return "游戏控制台指令";
  }
}

function formatScheduleStageLabel(stage?: string): string {
  switch (stage) {
    case "Queued":
      return "排队等待执行";
    case "ExecutingCommand":
      return "正在发送控制台指令";
    case "ArchivingBackup":
      return "正在归档并上传备份";
    case "RestartingInstance":
      return "正在重启服务器";
    case "Completed":
      return "";
    case "Failed":
      return "执行失败";
    default:
      if (!stage || /^[\x00-\x7F]+$/.test(stage)) return "";
      return stage;
  }
}

export function ScheduleTab({ inst, running }: { inst: InstanceItem; running?: boolean }) {
  const name = inst.metadata.name;
  const { apps, toast, token, tenant } = useConsole();
  const { updateSpec } = useServerActions();

  const presetId =
    inst.metadata.labels?.["gamepanel.io/preset"] ||
    inst.metadata.labels?.["gamepanel.io/app"] ||
    "custom-container";
  const richApp = useMemo(() => resolveRichApp(presetId, apps), [presetId, apps]);
  const saveCmd = richApp?.storagePolicy?.preBackupCommand || "save-all";
  const consoleActions = useMemo(() => richApp?.consoleActions || [], [richApp]);

  const schedules = useMemo(() => inst.spec.schedules || [], [inst.spec.schedules]);
  const statuses = useMemo(() => inst.status?.scheduleStatuses || [], [inst.status?.scheduleStatuses]);

  const [busyId, setBusyId] = useState<string | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<InstanceSchedule | null>(null);
  const [logScheduleId, setLogScheduleId] = useState<string | null>(null);

  // Form state
  const [formName, setFormName] = useState("");
  const [formAction, setFormAction] = useState<InstanceSchedule["action"]>("command");
  const [triggerMode, setTriggerMode] = useState<"interval" | "daily">("interval");
  const [formInterval, setFormInterval] = useState("30");
  const [formDailyAt, setFormDailyAt] = useState("04:00");
  const [formCommand, setFormCommand] = useState("");
  const [formRetainCount, setFormRetainCount] = useState("5");
  const [formEnabled, setFormEnabled] = useState(true);

  const presetTemplates = useMemo(() => {
    const broadcastAct = consoleActions.find(
      (a) =>
        a.id.includes("say") ||
        a.id.includes("broadcast") ||
        a.id.includes("announce") ||
        a.title.includes("广播") ||
        a.title.includes("公告"),
    );
    const defaultBroadcast = broadcastAct
      ? broadcastAct.command.replace(/\{\{[^}]+\}\}/g, "服务器例行维护与存档保存中")
      : `say 服务器例行维护与存档保存中`;

    return [
      {
        key: "auto-save",
        label: "自动保存世界 (每30分)",
        name: "定时保存世界存档",
        action: "command" as const,
        triggerMode: "interval" as const,
        interval: "30",
        dailyAt: "04:00",
        command: saveCmd,
      },
      {
        key: "hourly-broadcast",
        label: "整点全服公告 (每1小时)",
        name: "整点全服公告提醒",
        action: "command" as const,
        triggerMode: "interval" as const,
        interval: "60",
        dailyAt: "04:00",
        command: defaultBroadcast,
      },
      {
        key: "daily-restart",
        label: "凌晨例行重启 (04:00)",
        name: "每日凌晨例行维护重启",
        action: "restart" as const,
        triggerMode: "daily" as const,
        interval: "1440",
        dailyAt: "04:00",
        command: `${defaultBroadcast}\n${saveCmd}`,
      },
      {
        key: "auto-backup",
        label: "定时云端备份 (每6小时)",
        name: "每 6 小时自动创建备份",
        action: "backup" as const,
        triggerMode: "interval" as const,
        interval: "360",
        dailyAt: "04:00",
        command: saveCmd,
      },
    ];
  }, [consoleActions, saveCmd]);

  function openCreateModal() {
    setEditingId(null);
    setFormName("定时保存世界存档");
    setFormAction("command");
    setTriggerMode("interval");
    setFormInterval("30");
    setFormDailyAt("04:00");
    setFormCommand(saveCmd);
    setFormRetainCount("5");
    setFormEnabled(true);
    setModalOpen(true);
  }

  function openEditModal(sched: InstanceSchedule) {
    setEditingId(sched.id);
    setFormName(sched.name);
    setFormAction(sched.action);
    if (sched.dailyAt?.trim()) {
      setTriggerMode("daily");
      setFormDailyAt(sched.dailyAt.trim());
      setFormInterval("60");
    } else {
      setTriggerMode("interval");
      setFormInterval(String(sched.intervalMinutes || 60));
      setFormDailyAt("04:00");
    }
    setFormCommand(sched.command || "");
    setFormRetainCount(String(sched.retainCount ?? (sched.action === "backup" ? 5 : 0)));
    setFormEnabled(sched.enabled !== false);
    setModalOpen(true);
  }

  const editingTarget = useMemo(
    () => (editingId ? schedules.find((s) => s.id === editingId) || null : null),
    [editingId, schedules],
  );

  const isFormDirty = useMemo(() => {
    if (!editingTarget) return formName.trim().length > 0;
    const origMode = editingTarget.dailyAt?.trim() ? "daily" : "interval";
    return (
      formName.trim() !== editingTarget.name.trim() ||
      formAction !== editingTarget.action ||
      triggerMode !== origMode ||
      (triggerMode === "daily" && formDailyAt.trim() !== (editingTarget.dailyAt || "").trim()) ||
      (triggerMode === "interval" && Number(formInterval) !== (editingTarget.intervalMinutes || 60)) ||
      (formAction === "backup" && Number(formRetainCount) !== (editingTarget.retainCount || 0)) ||
      formCommand.trim() !== (editingTarget.command || "").trim() ||
      formEnabled !== (editingTarget.enabled !== false)
    );
  }, [editingTarget, formName, formAction, triggerMode, formDailyAt, formInterval, formRetainCount, formCommand, formEnabled]);

  async function handleSaveSchedule(e: FormEvent) {
    e.preventDefault();
    if (!formName.trim()) return;
    if (formAction === "command" && !formCommand.trim()) {
      toast("error", "请填写要执行的游戏控制台指令");
      return;
    }
    const id = editingId || `sched-${Date.now().toString(36)}`;
    const retainNum = formAction === "backup" ? Math.max(0, Number(formRetainCount) || 0) : 0;
    const nextItem: InstanceSchedule = {
      id,
      name: formName.trim(),
      action: formAction,
      command: formCommand.trim() || undefined,
      intervalMinutes: triggerMode === "interval" ? Math.max(1, Number(formInterval) || 30) : undefined,
      dailyAt: triggerMode === "daily" ? formDailyAt.trim() || "04:00" : undefined,
      retainCount: retainNum > 0 ? retainNum : undefined,
      enabled: formEnabled,
    };
    const nextList = editingId
      ? schedules.map((s) => (s.id === editingId ? nextItem : s))
      : [...schedules, nextItem];

    setBusyId("save");
    try {
      const ok = await updateSpec(inst, { schedules: nextList }, editingId ? "定时任务已更新" : "定时任务已创建");
      if (ok) setModalOpen(false);
    } finally {
      setBusyId(null);
    }
  }

  async function handleToggleSchedule(sched: InstanceSchedule) {
    setBusyId(sched.id);
    try {
      const nextList = schedules.map((s) => (s.id === sched.id ? { ...s, enabled: !s.enabled } : s));
      await updateSpec(
        inst,
        { schedules: nextList },
        !sched.enabled ? `已启用「${sched.name}」` : `已暂停「${sched.name}」`,
      );
    } finally {
      setBusyId(null);
    }
  }

  async function handleDeleteSchedule() {
    if (!confirmDelete) return;
    const target = confirmDelete;
    setBusyId(target.id);
    try {
      const nextList = schedules.filter((s) => s.id !== target.id);
      const ok = await updateSpec(inst, { schedules: nextList }, `已删除定时任务「${target.name}」`);
      if (ok) setConfirmDelete(null);
    } finally {
      setBusyId(null);
    }
  }

  async function handleRunNow(sched: InstanceSchedule) {
    if (!running) {
      toast("error", "服务器当前未运行，请先启动服务器后再执行");
      return;
    }
    setBusyId(`run-${sched.id}`);
    try {
      const nextList = schedules.map((s) =>
        s.id === sched.id ? { ...s, manualTriggerAt: new Date().toISOString() } : s,
      );
      await updateSpec(
        inst,
        { schedules: nextList },
        "已提交立即执行请求，正在后台运行",
      );
    } finally {
      setBusyId(null);
    }
  }

  const logSchedule = logScheduleId ? schedules.find((s) => s.id === logScheduleId) ?? null : null;
  const logStatus = logScheduleId ? statuses.find((x) => x.id === logScheduleId) ?? null : null;
  const logTaskId = logStatus?.runID ? `sched-${logStatus.runID}` : null;
  const logIsRunning = logStatus?.lastResult === "Running";
  const logIsFailed = logStatus?.lastResult === "Failed";
  const logIsUnknown = logStatus?.lastResult === "Unknown";
  const logProgress = logStatus?.progress;

  return (
    <>
      <Card>
        <CardHeader
          title={
            <span className="inline-flex items-baseline gap-2">
              <span>定时任务</span>
              {schedules.length > 0 && (
                <span className="text-xs font-medium tabular-nums text-[#8A90A6]">
                  {schedules.length}
                </span>
              )}
            </span>
          }
          actions={
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="h-3.5 w-3.5" />}
              onClick={openCreateModal}
            >
              新建任务
            </Button>
          }
        />

        {schedules.length === 0 ? (
          <EmptyState
            title="暂无定时任务"
            body="点击右上角「新建任务」配置自动保存、游戏内广播或定时重启。"
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] text-left text-sm">
              <thead>
                <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
                  <th className="px-6 py-3.5 font-medium">任务名称</th>
                  <th className="px-4 py-3.5 font-medium">触发周期</th>
                  <th className="px-4 py-3.5 font-medium">执行动作与指令</th>
                  <th className="px-4 py-3.5 font-medium">运行状态</th>
                  <th className="px-6 py-3.5 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#EEF0F7]">
                {schedules.map((sched) => {
                  const st = statuses.find((x) => x.id === sched.id);
                  const isBusy = busyId === `run-${sched.id}` || busyId === sched.id;
                  const isRunningTask = st?.lastResult === "Running";
                  const badgeTone =
                    st?.lastResult === "Running"
                      ? "accent"
                      : st?.lastResult === "Failed"
                        ? "danger"
                        : st?.lastResult === "Unknown"
                          ? "warning"
                          : st?.lastResult === "Succeeded"
                            ? "success"
                            : "neutral";
                  const badgeText =
                    st?.lastResult === "Running"
                      ? "执行中"
                      : st?.lastResult === "Failed"
                        ? "执行失败"
                        : st?.lastResult === "Unknown"
                          ? "结果未知"
                          : st?.lastResult === "Succeeded"
                            ? "上次成功"
                            : sched.enabled
                              ? "等待执行"
                              : "已停用";
                  const pct = typeof st?.progress === "number" && st.progress > 0 ? Math.min(100, st.progress) : null;
                  const overflowItems: MenuItem[] = [
                    {
                      label: "立即执行一次",
                      icon: <Play className="h-4 w-4" />,
                      disabled: !running || isBusy || isRunningTask,
                      onSelect: () => void handleRunNow(sched),
                    },
                  ];
                  if (st?.runID) {
                    overflowItems.push({
                      label: "查看日志",
                      icon: <Terminal className="h-4 w-4" />,
                      onSelect: () => setLogScheduleId(sched.id),
                    });
                  }
                  overflowItems.push(
                    {
                      label: "编辑任务",
                      icon: <Pencil className="h-4 w-4" />,
                      disabled: isBusy,
                      onSelect: () => openEditModal(sched),
                    },
                    {
                      label: "删除任务",
                      icon: <Trash2 className="h-4 w-4" />,
                      danger: true,
                      disabled: isBusy,
                      onSelect: () => setConfirmDelete(sched),
                    },
                  );
                  return (
                    <tr key={sched.id} className="transition-colors hover:bg-[#F9FAFE]">
                      <td className="px-6 py-4">
                        <div className="font-semibold text-[#14161F]">{sched.name}</div>
                        <div className="mt-0.5 text-xs text-[#8A90A6]">{formatActionLabel(sched)}</div>
                      </td>
                      <td className="px-4 py-4">
                        <Mono className="text-[13px] font-medium text-[#14161F]">
                          {formatScheduleTrigger(sched)}
                        </Mono>
                      </td>
                      <td className="max-w-[280px] px-4 py-4">
                        {sched.command ? (
                          <Mono className="block truncate rounded-lg bg-[#F4F6FA] px-2.5 py-1 text-xs text-[#3D4258]">
                            {sched.command.replace(/\n/g, " ; ")}
                          </Mono>
                        ) : (
                          <span className="text-xs text-[#8A90A6]">
                            {sched.action === "restart" ? "直接重启服务器" : "直接执行归档备份"}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-4">
                        <div className="space-y-1.5">
                          <div className="flex items-center gap-2">
                            <Badge tone={badgeTone}>{badgeText}</Badge>
                            <span className="text-xs text-[#8A90A6]">
                              {isRunningTask
                                ? formatScheduleStageLabel(st?.stage) || st?.lastOutput || "正在执行"
                                : st?.lastRunAt
                                  ? relativeTime(st.lastRunAt)
                                  : "等待首次触发"}
                            </span>
                          </div>
                          {isRunningTask && (
                            <div className="flex items-center gap-2">
                              <progress
                                className="gp-progress gp-progress-accent h-1.5 w-28"
                                max={100}
                                {...(pct !== null ? { value: pct } : {})}
                              />
                              {pct !== null && (
                                <span className="text-[11px] tabular-nums text-[#5B6075]">{pct}%</span>
                              )}
                            </div>
                          )}
                          {!isRunningTask && st?.lastOutput && st?.lastResult === "Failed" && (
                            <div className="max-w-[220px] truncate text-xs text-[#B42318]" title={st.lastOutput}>
                              {st.lastOutput}
                            </div>
                          )}
                        </div>
                      </td>
                      <td className="px-6 py-4 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <Switch
                            checked={sched.enabled}
                            onChange={() => void handleToggleSchedule(sched)}
                            label={sched.enabled ? `停用 ${sched.name}` : `启用 ${sched.name}`}
                            disabled={isBusy}
                          />
                          <MoreMenu
                            label={`更多操作 (${sched.name})`}
                            className="h-8 w-8 bg-transparent"
                            items={overflowItems}
                          />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editingId ? "编辑定时任务" : "新建定时任务"}
        width="max-w-lg"
        footer={
          <>
            <Button variant="secondary" onClick={() => setModalOpen(false)}>
              取消
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="schedule-form"
              loading={busyId === "save"}
              disabled={busyId !== null || !isFormDirty || !formName.trim()}
            >
              {editingId ? "保存修改" : "创建任务"}
            </Button>
          </>
        }
      >
        <form id="schedule-form" onSubmit={(e) => void handleSaveSchedule(e)} className="space-y-4">
          {!editingId && (
            <div>
              <div className="mb-1.5 text-xs font-medium text-[#5B6075]">一键套用常用运维模板</div>
              <div className="flex flex-wrap gap-1.5">
                {presetTemplates.map((tpl) => (
                  <button
                    key={tpl.key}
                    type="button"
                    onClick={() => {
                      setFormName(tpl.name);
                      setFormAction(tpl.action);
                      setTriggerMode(tpl.triggerMode);
                      setFormInterval(tpl.interval);
                      setFormDailyAt(tpl.dailyAt);
                      setFormCommand(tpl.command);
                    }}
                    className="rounded-lg border border-[#E3E6F2] bg-[#F7F8FC] px-2.5 py-1 text-xs font-medium text-[#3D4258] transition-colors hover:border-[#2B3AE8] hover:bg-[#EEF1FF] hover:text-[#2B3AE8]"
                  >
                    {tpl.label}
                  </button>
                ))}
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="任务名称">
              <Input
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
                placeholder="例如：每 30 分钟保存世界"
                required
              />
            </Field>
            <Field label="执行动作类型">
              <Select
                value={formAction}
                onChange={(e) => setFormAction(e.target.value as InstanceSchedule["action"])}
              >
                <option value="command">发送游戏控制台指令</option>
                <option value="restart">例行重启服务器实例</option>
                <option value="backup">创建云端存档备份</option>
              </Select>
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="触发方式">
              <Segmented
                label="触发方式"
                value={triggerMode}
                onChange={setTriggerMode}
                items={[
                  { value: "interval", label: "固定间隔" },
                  { value: "daily", label: "每日定点" },
                ]}
              />
            </Field>
            {triggerMode === "interval" ? (
              <Field label="执行间隔">
                <Select value={formInterval} onChange={(e) => setFormInterval(e.target.value)}>
                  {INTERVAL_OPTIONS.map((opt) => (
                    <option key={opt.value} value={String(opt.value)}>
                      {opt.label}
                    </option>
                  ))}
                </Select>
              </Field>
            ) : (
              <Field label="每日执行时刻 (北京时间)">
                <Input
                  type="time"
                  value={formDailyAt}
                  onChange={(e) => setFormDailyAt(e.target.value)}
                  className="font-mono"
                  required
                />
              </Field>
            )}
          </div>

          <div className="grid min-h-[212px] content-start space-y-4">
            <Field
              label={
                formAction === "command"
                  ? "游戏控制台指令（支持多行顺序下发）"
                  : formAction === "restart"
                    ? "重启前预备指令（选填，如全服公告与保存世界）"
                    : "备份前预备指令（选填，如刷盘保存存档）"
              }
              hint={
                consoleActions.length > 0 ? (
                  <span className="inline-flex flex-wrap items-center gap-1 pt-1">
                    <span className="text-[11px] text-[#8A90A6]">点击插入当前游戏快捷指令：</span>
                    {consoleActions.slice(0, 5).map((act) => (
                      <button
                        key={act.id}
                        type="button"
                        onClick={() => {
                          const resolved = act.command.replace(/\{\{[^}]+\}\}/g, (m) => {
                            const pName = m.replace(/\{|\}/g, "").trim();
                            const param = act.params?.find((p) => p.name === pName);
                            return param?.defaultValue || param?.options?.[0]?.value || "";
                          });
                          setFormCommand((prev) => (prev.trim() ? `${prev.trim()}\n${resolved}` : resolved));
                        }}
                        className="rounded border border-[#E3E6F2] bg-[#F7F8FC] px-1.5 py-0.5 font-mono text-[11px] text-[#2B3AE8] hover:bg-[#EEF1FF]"
                      >
                        + {act.title}
                      </button>
                    ))}
                  </span>
                ) : undefined
              }
            >
              <Textarea
                rows={3}
                value={formCommand}
                onChange={(e) => setFormCommand(e.target.value)}
                className="font-mono text-xs"
                placeholder={
                  formAction === "command"
                    ? `例如：\n${saveCmd}`
                    : `例如：\nsay 服务器即将维护重启\n${saveCmd}`
                }
              />
            </Field>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <Field label="任务状态">
                <Select
                  value={formEnabled ? "enabled" : "disabled"}
                  onChange={(e) => setFormEnabled(e.target.value === "enabled")}
                >
                  <option value="enabled">启用（服务器运行期间自动按计划触发）</option>
                  <option value="disabled">停用（保留配置但不自动触发）</option>
                </Select>
              </Field>
              <div
                className={formAction === "backup" ? "" : "invisible pointer-events-none"}
                aria-hidden={formAction !== "backup"}
              >
                <Field label="自动清理旧备份（保留份数）">
                  <Select value={formRetainCount} onChange={(e) => setFormRetainCount(e.target.value)}>
                    <option value="3">保留最近 3 份</option>
                    <option value="5">保留最近 5 份（推荐）</option>
                    <option value="10">保留最近 10 份</option>
                    <option value="15">保留最近 15 份</option>
                    <option value="0">不单独限制（遵循备份页策略）</option>
                  </Select>
                </Field>
              </div>
            </div>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!confirmDelete}
        title={`删除定时任务「${confirmDelete?.name ?? ""}」？`}
        body="删除后该计划将不再自动执行，已创建的存档与备份不受影响。"
        confirmLabel="删除任务"
        danger
        loading={busyId === confirmDelete?.id}
        onConfirm={() => void handleDeleteSchedule()}
        onClose={() => setConfirmDelete(null)}
      />

      <TaskLogModal
        open={!!logSchedule && !!logTaskId}
        onClose={() => setLogScheduleId(null)}
        wsPath={
          logTaskId && tenant
            ? `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/instances/${encodeURIComponent(name)}/log?taskID=${encodeURIComponent(logTaskId)}`
            : ""
        }
        token={token}
        title={logSchedule?.name || "定时任务"}
        description={
          logSchedule ? `${formatActionLabel(logSchedule)} · ${formatScheduleTrigger(logSchedule)}` : undefined
        }
        badgeTone={
          logIsRunning
            ? "accent"
            : logIsFailed
              ? "danger"
              : logIsUnknown
                ? "warning"
                : "success"
        }
        badgeLabel={
          logIsRunning
            ? "执行中"
            : logIsFailed
              ? "执行失败"
              : logIsUnknown
                ? "结果未知"
                : "执行成功"
        }
        message={
          logIsRunning
            ? logStatus?.lastOutput || formatScheduleStageLabel(logStatus?.stage)
            : logIsFailed
              ? logStatus?.lastOutput
              : logStatus?.lastRunAt
                ? `完成于 ${relativeTime(logStatus.lastRunAt)}`
                : undefined
        }
        progress={logProgress}
        isIndeterminate={logIsRunning && !(typeof logProgress === "number" && logProgress > 0)}
        isActive={logIsRunning}
      />
    </>
  );
}
