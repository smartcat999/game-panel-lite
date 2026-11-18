"use client";

import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Check, Pencil, Plus, Trash2 } from "lucide-react";
import type {
  AppConfigField,
  AppConsoleAction,
  AppMetricItemAction,
  AppMetricSpec,
} from "../../../console/lib/types";
import { Badge, Button, IconButton, Input, Modal, Mono, Select, Textarea } from "../ui";

const SOURCE_META: Record<string, { badge: string; tone: "accent" | "success" | "neutral" }> = {
  stdin_log: { badge: "日志流", tone: "accent" },
  exec: { badge: "后台命令", tone: "success" },
  file: { badge: "文件读取", tone: "neutral" },
};

const MODE_LABELS: Record<string, string> = {
  regex_value: "正则提取数值",
  regex_count: "正则匹配计数",
  log_delta: "进出日志加减",
};

function isPlayerTargetedAction(act: AppConsoleAction, bindParam?: string): boolean {
  const params = act.params || [];
  if (params.length === 0) return false;
  const targetKey = (bindParam || "player").toLowerCase();
  return params.some((p) => {
    const n = p.name.toLowerCase();
    return (
      n === targetKey ||
      n === "player" ||
      n === "steamid" ||
      n === "userid" ||
      n === "username" ||
      n === "user" ||
      n === "target" ||
      p.label?.includes("玩家")
    );
  });
}

export function AppMetricsEditor({
  metrics,
  onChange,
  schema,
  consoleActions,
  jsonMode,
  onCloseJsonMode,
  editingId,
  onEditingIdChange,
  onToast,
}: {
  metrics: AppMetricSpec[];
  onChange: (next: AppMetricSpec[]) => void;
  schema: AppConfigField[];
  consoleActions: AppConsoleAction[];
  jsonMode: boolean;
  onCloseJsonMode: () => void;
  editingId: string | null;
  onEditingIdChange: (id: string | null) => void;
  onToast: (type: "success" | "error", msg: string) => void;
}) {
  const [draft, setDraft] = useState<AppMetricSpec | null>(null);
  const [jsonText, setJsonText] = useState<string>(() => JSON.stringify(metrics, null, 2));

  const numericSchemaFields = useMemo(
    () => schema.filter((f) => f.type === "number" || /max|limit|player|slot/i.test(f.name)),
    [schema],
  );

  const normalizeMaxKey = (rawKey?: string): string => {
    if (!rawKey) return "";
    if (numericSchemaFields.some((f) => f.name === rawKey)) return rawKey;
    if (rawKey === "players" && numericSchemaFields.some((f) => f.name === "maxPlayers")) {
      return "maxPlayers";
    }
    return rawKey;
  };

  const isDuplicateItemAction = (ia: AppMetricItemAction, quickIds?: string[]) => {
    const linked = consoleActions.filter((ca) => (quickIds || []).includes(ca.id));
    const norm = (c: string) =>
      c
        .replace(/\{\{\s*\w+\s*\}\}/g, "{{item}}")
        .replace(/["']/g, "")
        .trim()
        .toLowerCase();
    return linked.some(
      (ca) =>
        ca.id === ia.id ||
        ca.id.endsWith(`-${ia.id}`) ||
        ca.title === ia.title ||
        ca.title.includes(ia.title) ||
        ia.title.includes(ca.title) ||
        norm(ca.command) === norm(ia.command),
    );
  };

  const startInlineEdit = (metric: AppMetricSpec) => {
    const cloned: AppMetricSpec = JSON.parse(JSON.stringify(metric));
    if (cloned.maxConfigKey) {
      cloned.maxConfigKey = normalizeMaxKey(cloned.maxConfigKey);
    }
    if (!cloned.quickActionIds?.length && cloned.itemActions?.length) {
      const matchedIds = consoleActions
        .filter((ca) =>
          cloned.itemActions?.some(
            (ia) => ca.id.endsWith(`-${ia.id}`) || ca.id === ia.id || ca.title.includes(ia.title),
          ),
        )
        .map((ca) => ca.id);
      if (matchedIds.length > 0) {
        cloned.quickActionIds = matchedIds;
      }
    }
    if (cloned.quickActionIds?.length && cloned.itemActions?.length) {
      cloned.itemActions = cloned.itemActions.filter(
        (ia) => !isDuplicateItemAction(ia, cloned.quickActionIds),
      );
    }
    setDraft(cloned);
    onEditingIdChange(metric.id);
  };

  const activeDraft = useMemo(() => {
    if (!editingId) return null;
    if (draft && draft.id === editingId) return draft;
    const found = metrics.find((m) => m.id === editingId);
    if (!found) return null;
    const cloned = JSON.parse(JSON.stringify(found)) as AppMetricSpec;
    if (cloned.maxConfigKey) {
      cloned.maxConfigKey = normalizeMaxKey(cloned.maxConfigKey);
    }
    return cloned;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editingId, draft, metrics]);

  const commitInlineEdit = () => {
    if (!activeDraft) return;
    if (!activeDraft.id.trim() || !activeDraft.label.trim()) {
      onToast("error", "请填写指标名称与指标标识");
      return;
    }
    const cleanedItemActions = (activeDraft.itemActions || [])
      .filter((ia) => ia.title?.trim() && ia.command?.trim())
      .map((ia, idx) => ({
        id: ia.id?.trim() || `act_${idx + 1}`,
        title: ia.title.trim(),
        command: ia.command.trim(),
        danger: Boolean(ia.danger),
      }));

    const cleaned: AppMetricSpec = {
      ...activeDraft,
      id: activeDraft.id.trim(),
      label: activeDraft.label.trim(),
      unit: activeDraft.unit?.trim() || undefined,
      source: activeDraft.source || "stdin_log",
      mode: activeDraft.mode || "regex_value",
      filePath: activeDraft.source === "file" ? activeDraft.filePath?.trim() || undefined : undefined,
      probeCommand: activeDraft.source !== "file" ? activeDraft.probeCommand?.trim() || undefined : undefined,
      pattern: activeDraft.mode !== "log_delta" ? activeDraft.pattern?.trim() || undefined : undefined,
      itemPattern: activeDraft.mode !== "log_delta" ? activeDraft.itemPattern?.trim() || undefined : undefined,
      incPattern: activeDraft.incPattern?.trim() || undefined,
      decPattern: activeDraft.decPattern?.trim() || undefined,
      resetPattern: activeDraft.resetPattern?.trim() || undefined,
      maxConfigKey: activeDraft.maxConfigKey?.trim() || undefined,
      maxValue: activeDraft.maxConfigKey ? undefined : activeDraft.maxValue?.trim() || undefined,
      bindParam: activeDraft.bindParam?.trim() || undefined,
      quickActionIds: activeDraft.quickActionIds?.length ? activeDraft.quickActionIds : undefined,
      itemActions: cleanedItemActions.length ? cleanedItemActions : undefined,
    };
    const nextList = metrics.map((m) => (m.id === editingId ? cleaned : m));
    onChange(nextList);
    setDraft(null);
    onEditingIdChange(null);
  };

  const moveRow = (id: string, delta: number) => {
    const idx = metrics.findIndex((m) => m.id === id);
    if (idx < 0) return;
    const target = idx + delta;
    if (target < 0 || target >= metrics.length) return;
    const copy = [...metrics];
    const [item] = copy.splice(idx, 1);
    copy.splice(target, 0, item);
    onChange(copy);
  };

  const deleteRow = (id: string) => {
    onChange(metrics.filter((m) => m.id !== id));
    if (editingId === id) {
      setDraft(null);
      onEditingIdChange(null);
    }
  };

  if (jsonMode) {
    return (
      <div className="space-y-3 px-6 py-5">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-[#5B6075]">
            直接编辑 <code className="font-mono text-[#2B3AE8]">spec.metrics</code> 声明式采集规则 JSON 数组
          </span>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={onCloseJsonMode}>
              取消
            </Button>
            <Button
              size="sm"
              variant="primary"
              onClick={() => {
                try {
                  const parsed = JSON.parse(jsonText);
                  const list = Array.isArray(parsed) ? parsed : parsed.metrics;
                  if (!Array.isArray(list)) {
                    throw new Error("JSON 需为监控指标数组");
                  }
                  onChange(list);
                  onCloseJsonMode();
                  onToast("success", `已应用 ${list.length} 项监控指标配置`);
                } catch (err) {
                  onToast("error", err instanceof Error ? err.message : "JSON 格式有误");
                }
              }}
            >
              应用 JSON
            </Button>
          </div>
        </div>
        <Textarea
          rows={18}
          value={jsonText}
          onChange={(e) => setJsonText(e.target.value)}
          className="font-mono text-xs"
        />
      </div>
    );
  }

  return (
    <div>
      <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="监控指标列表">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-[#EEF0F7] bg-[#FAFBFD] text-xs text-[#8A90A6]">
            <tr>
              <th className="w-[24%] px-6 py-3 font-medium">指标名称</th>
              <th className="w-[28%] px-6 py-3 font-medium">采集方式</th>
              <th className="w-[34%] px-6 py-3 font-medium">解析与列表联动</th>
              <th className="w-[14%] px-6 py-3 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#EEF0F7] text-sm">
            {metrics.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-6 py-14 text-center text-sm text-[#8A90A6]">
                  当前应用暂未配置自定义监控指标，点击右上角「+ 新增指标」添加
                </td>
              </tr>
            ) : (
              metrics.map((item) => {
                const srcMeta = SOURCE_META[item.source || "stdin_log"] || SOURCE_META.stdin_log;
                const modeLabel = MODE_LABELS[item.mode || "regex_value"] || "正则提取数值";
                const hasEntityList = Boolean(
                  item.itemPattern ||
                    item.incPattern ||
                    item.mode === "log_delta" ||
                    item.quickActionIds?.length ||
                    item.itemActions?.length,
                );
                const normalizedKey = normalizeMaxKey(item.maxConfigKey);
                const nonDupItemActions = (item.itemActions || []).filter(
                  (ia) => !isDuplicateItemAction(ia, item.quickActionIds),
                );
                const actionCount =
                  (item.quickActionIds?.length || 0) + nonDupItemActions.length;

                const summaryParts: string[] = [modeLabel];
                if (normalizedKey) {
                  summaryParts.push(`上限关联 ${normalizedKey}`);
                } else if (item.maxValue) {
                  summaryParts.push(`固定上限 ${item.maxValue}${item.unit ? ` ${item.unit}` : ""}`);
                }
                if (hasEntityList) {
                  summaryParts.push(
                    actionCount > 0 ? `含玩家列表 (${actionCount} 项快捷指令)` : "含玩家列表",
                  );
                }

                return (
                  <tr key={item.id} className="transition-colors hover:bg-[#FAFBFD]">
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium text-[#14161F]">{item.label}</span>
                        {item.unit && (
                          <span className="text-xs text-[#8A90A6]">/ {item.unit}</span>
                        )}
                      </div>
                      <Mono className="mt-0.5 block text-xs text-[#8A90A6]">{item.id}</Mono>
                    </td>

                    <td className="px-6 py-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone={srcMeta.tone}>{srcMeta.badge}</Badge>
                        {item.source === "file" && item.filePath ? (
                          <Mono className="text-xs text-[#14161F]">{item.filePath}</Mono>
                        ) : item.probeCommand ? (
                          <Mono className="text-xs text-[#14161F]">$ {item.probeCommand}</Mono>
                        ) : (
                          <span className="text-xs text-[#8A90A6]">被动监听</span>
                        )}
                      </div>
                    </td>

                    <td className="px-6 py-4 text-xs text-[#5B6075]">
                      {summaryParts.join(" · ")}
                    </td>

                    <td className="whitespace-nowrap px-6 py-4 text-right">
                      <div className="inline-flex items-center gap-1">
                        <IconButton label={`上移指标 ${item.label}`} onClick={() => moveRow(item.id, -1)}>
                          <ArrowUp className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton label={`下移指标 ${item.label}`} onClick={() => moveRow(item.id, 1)}>
                          <ArrowDown className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton label={`编辑指标 ${item.label}`} onClick={() => startInlineEdit(item)}>
                          <Pencil className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton
                          label={`删除指标 ${item.label}`}
                          onClick={() => deleteRow(item.id)}
                          className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      <Modal
        open={Boolean(activeDraft)}
        onClose={() => {
          setDraft(null);
          onEditingIdChange(null);
        }}
        title={`编辑监控指标 · ${activeDraft?.label || "未命名指标"}`}
        description={activeDraft?.id ? `指标标识：${activeDraft.id}` : "配置实时监控指标采集方式与解析规则"}
        width="max-w-3xl"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setDraft(null);
                onEditingIdChange(null);
              }}
            >
              取消
            </Button>
            <Button variant="primary" onClick={commitInlineEdit}>
              完成
            </Button>
          </>
        }
      >
        {activeDraft && (() => {
          const candidateActions = [
            ...consoleActions.filter((a) => isPlayerTargetedAction(a, activeDraft.bindParam)),
            ...consoleActions.filter(
              (a) =>
                !isPlayerTargetedAction(a, activeDraft.bindParam) && (a.params?.length || 0) > 0,
            ),
          ];
          const customItemActions: AppMetricItemAction[] = activeDraft.itemActions || [];
          return (
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-3 md:grid-cols-12">
                <div className="md:col-span-3">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    指标名称
                  </label>
                  <Input
                    value={activeDraft.label}
                    onChange={(e) => setDraft({ ...activeDraft, label: e.target.value })}
                    placeholder="如：在线玩家"
                    className="h-9 bg-white text-xs"
                  />
                </div>

                <div className="md:col-span-3">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    指标标识 (ID)
                  </label>
                  <Input
                    value={activeDraft.id}
                    onChange={(e) =>
                      setDraft({
                        ...activeDraft,
                        id: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_"),
                      })
                    }
                    placeholder="online_players"
                    className="h-9 bg-white font-mono text-xs"
                  />
                </div>

                <div className="md:col-span-2">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    单位
                  </label>
                  <Input
                    value={activeDraft.unit || ""}
                    onChange={(e) => setDraft({ ...activeDraft, unit: e.target.value })}
                    placeholder="如：人 / TPS"
                    className="h-9 bg-white text-xs"
                  />
                </div>

                <div className="md:col-span-4">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    上限设置
                  </label>
                  <div className="flex items-center gap-2">
                    <Select
                      value={
                        activeDraft.maxConfigKey
                          ? `key:${normalizeMaxKey(activeDraft.maxConfigKey)}`
                          : activeDraft.maxValue !== undefined && activeDraft.maxValue !== ""
                            ? "__fixed__"
                            : ""
                      }
                      onChange={(e) => {
                        const v = e.target.value;
                        if (!v) {
                          setDraft({
                            ...activeDraft,
                            maxConfigKey: undefined,
                            maxValue: undefined,
                          });
                        } else if (v === "__fixed__") {
                          setDraft({
                            ...activeDraft,
                            maxConfigKey: undefined,
                            maxValue: activeDraft.maxValue || "20",
                          });
                        } else if (v.startsWith("key:")) {
                          setDraft({
                            ...activeDraft,
                            maxConfigKey: v.slice(4),
                            maxValue: undefined,
                          });
                        }
                      }}
                      className="h-9 flex-1 bg-white text-xs"
                    >
                      <option value="">无上限</option>
                      {numericSchemaFields.map((f) => (
                        <option key={f.name} value={`key:${f.name}`}>
                          关联配置项：{f.label} ({f.name})
                        </option>
                      ))}
                      <option value="__fixed__">固定数值上限</option>
                    </Select>
                    {!activeDraft.maxConfigKey &&
                      activeDraft.maxValue !== undefined &&
                      activeDraft.maxValue !== "" && (
                        <Input
                          value={activeDraft.maxValue}
                          onChange={(e) =>
                            setDraft({ ...activeDraft, maxValue: e.target.value })
                          }
                          placeholder="20"
                          className="h-9 w-24 shrink-0 bg-white font-mono text-xs"
                        />
                      )}
                  </div>
                </div>
              </div>

              <div className="space-y-3 border-t border-[#EEF0F7] pt-3.5">
                <div className="grid grid-cols-1 gap-3 md:grid-cols-12">
                  <div className="md:col-span-4">
                    <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                      采集方式
                    </label>
                    <Select
                      value={activeDraft.source || "stdin_log"}
                      onChange={(e) =>
                        setDraft({
                          ...activeDraft,
                          source: e.target.value as "stdin_log" | "exec" | "file",
                        })
                      }
                      className="h-9 bg-white text-xs"
                    >
                      <option value="stdin_log">控制台与日志流</option>
                      <option value="exec">后台命令探测</option>
                      <option value="file">读取工作目录文件</option>
                    </Select>
                  </div>

                  <div className="md:col-span-4">
                    <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                      解析模式
                    </label>
                    <Select
                      value={activeDraft.mode || "regex_value"}
                      onChange={(e) =>
                        setDraft({
                          ...activeDraft,
                          mode: e.target.value as "regex_value" | "regex_count" | "log_delta",
                        })
                      }
                      className="h-9 bg-white text-xs"
                    >
                      <option value="regex_value">正则提取数值</option>
                      <option value="regex_count">正则匹配行计数</option>
                      <option value="log_delta">进出日志加减计数</option>
                    </Select>
                  </div>

                  <div className="md:col-span-4">
                    {activeDraft.source === "file" ? (
                      <>
                        <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                          文件相对路径
                        </label>
                        <Input
                          value={activeDraft.filePath || ""}
                          onChange={(e) =>
                            setDraft({ ...activeDraft, filePath: e.target.value })
                          }
                          placeholder="如：logs/latest.log"
                          className="h-9 bg-white font-mono text-xs"
                        />
                      </>
                    ) : (
                      <>
                        <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                          定时探测指令 (选填)
                        </label>
                        <Input
                          value={activeDraft.probeCommand || ""}
                          onChange={(e) =>
                            setDraft({ ...activeDraft, probeCommand: e.target.value })
                          }
                          placeholder="如：list / ShowPlayers (留空则仅监听日志)"
                          className="h-9 bg-white font-mono text-xs"
                        />
                      </>
                    )}
                  </div>
                </div>

                {activeDraft.mode === "log_delta" ? (
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                    <div>
                      <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                        加入事件正则 (+1)
                      </label>
                      <Input
                        value={activeDraft.incPattern || ""}
                        onChange={(e) =>
                          setDraft({ ...activeDraft, incPattern: e.target.value })
                        }
                        placeholder="如：(.+) has joined"
                        className="h-9 bg-white font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                        离开事件正则 (-1)
                      </label>
                      <Input
                        value={activeDraft.decPattern || ""}
                        onChange={(e) =>
                          setDraft({ ...activeDraft, decPattern: e.target.value })
                        }
                        placeholder="如：(.+) has left"
                        className="h-9 bg-white font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                        重置清零正则 (选填)
                      </label>
                      <Input
                        value={activeDraft.resetPattern || ""}
                        onChange={(e) =>
                          setDraft({ ...activeDraft, resetPattern: e.target.value })
                        }
                        placeholder="如：Shutting down"
                        className="h-9 bg-white font-mono text-xs"
                      />
                    </div>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                    <div>
                      <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                        数值提取正则
                      </label>
                      <Input
                        value={activeDraft.pattern || ""}
                        onChange={(e) =>
                          setDraft({ ...activeDraft, pattern: e.target.value })
                        }
                        placeholder="如：There are (\d+) of a max of (?:\d+) players online"
                        className="h-9 bg-white font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                        在线列表提取正则 (选填，填入后开启玩家列表)
                      </label>
                      <Input
                        value={activeDraft.itemPattern || ""}
                        onChange={(e) =>
                          setDraft({ ...activeDraft, itemPattern: e.target.value })
                        }
                        placeholder="如：players online:\s*(.+)"
                        className="h-9 bg-white font-mono text-xs"
                      />
                    </div>
                  </div>
                )}
              </div>

              <div className="space-y-3 border-t border-[#EEF0F7] pt-3.5">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold text-[#14161F]">
                      在线列表快捷指令与轻量按钮
                    </span>
                    <span className="text-[11.5px] text-[#8A90A6]">
                      在玩家列表每行右侧显示一键操作按钮
                    </span>
                  </div>

                  <div className="flex items-center gap-2">
                    <span className="text-[11.5px] text-[#5B6075]">绑定参数变量:</span>
                    <Input
                      value={activeDraft.bindParam || "player"}
                      onChange={(e) =>
                        setDraft({ ...activeDraft, bindParam: e.target.value })
                      }
                      placeholder="player"
                      className="h-7.5 w-24 bg-white font-mono text-xs"
                    />
                    <Button
                      size="sm"
                      variant="secondary"
                      icon={<Plus className="h-3.5 w-3.5" aria-hidden />}
                      onClick={() => {
                        const nextItems: AppMetricItemAction[] = [
                          ...(activeDraft.itemActions || []),
                          {
                            id: `act_${(activeDraft.itemActions?.length || 0) + 1}`,
                            title: "踢出",
                            command: "kick {{item}}",
                            danger: true,
                          },
                        ];
                        setDraft({ ...activeDraft, itemActions: nextItems });
                      }}
                    >
                      添加自定义按钮
                    </Button>
                  </div>
                </div>

                {candidateActions.length > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="mr-1 text-[11.5px] text-[#5B6075]">
                      勾选关联「游戏指令」:
                    </span>
                    {candidateActions.map((act) => {
                      const selected = (activeDraft.quickActionIds || []).includes(act.id);
                      const paramNames = (act.params || []).map((p) => p.name).join(", ");
                      return (
                        <button
                          key={act.id}
                          type="button"
                          onClick={() => {
                            const cur = activeDraft.quickActionIds || [];
                            const next = selected
                              ? cur.filter((x) => x !== act.id)
                              : [...cur, act.id];
                            const paramKey =
                              act.params?.[0]?.name || activeDraft.bindParam || "player";
                            setDraft({
                              ...activeDraft,
                              bindParam: activeDraft.bindParam || paramKey,
                              quickActionIds: next,
                            });
                          }}
                          className={`inline-flex h-7.5 items-center gap-1.5 rounded-lg border px-2.5 text-xs transition ${
                            selected
                              ? "border-[#2B3AE8]/40 bg-[#EEF1FF]/70 font-medium text-[#2B3AE8]"
                              : "border-[#E3E6F2] bg-[#FAFBFD] text-[#3D4258] hover:border-[#C5CCDF] hover:bg-white"
                          }`}
                        >
                          {selected && <Check className="h-3.5 w-3.5 stroke-[2.5]" aria-hidden />}
                          <span>{act.title}</span>
                          {paramNames && (
                            <Mono className="text-[10.5px] text-[#8A90A6]">
                              ({`{{${paramNames}}}`})
                            </Mono>
                          )}
                        </button>
                      );
                    })}
                  </div>
                )}

                {customItemActions.length > 0 && (
                  <div className="space-y-2 pt-1">
                    <div className="grid grid-cols-[140px_1fr_110px_36px] gap-2.5 text-[11px] font-medium text-[#5B6075]">
                      <span>按钮文字</span>
                      <span>执行指令模板（使用 {`{{item}}`} 代表当前玩家/实体）</span>
                      <span>按钮样式</span>
                      <span />
                    </div>
                    {customItemActions.map((ia, idx) => (
                      <div
                        key={ia.id || idx}
                        className="grid grid-cols-[140px_1fr_110px_36px] items-center gap-2.5"
                      >
                        <Input
                          value={ia.title}
                          onChange={(e) => {
                            const copy = [...customItemActions];
                            copy[idx] = { ...ia, title: e.target.value };
                            setDraft({ ...activeDraft, itemActions: copy });
                          }}
                          placeholder="如：踢出"
                          className="h-8.5 bg-white text-xs"
                        />
                        <Input
                          value={ia.command}
                          onChange={(e) => {
                            const copy = [...customItemActions];
                            copy[idx] = { ...ia, command: e.target.value };
                            setDraft({ ...activeDraft, itemActions: copy });
                          }}
                          placeholder="如：kick {{item}}"
                          className="h-8.5 bg-white font-mono text-xs"
                        />
                        <Select
                          value={ia.danger ? "danger" : "default"}
                          onChange={(e) => {
                            const copy = [...customItemActions];
                            copy[idx] = { ...ia, danger: e.target.value === "danger" };
                            setDraft({ ...activeDraft, itemActions: copy });
                          }}
                          className="h-8.5 bg-white text-xs"
                        >
                          <option value="default">常规按钮</option>
                          <option value="danger">危险标红</option>
                        </Select>
                        <IconButton
                          label="删除此按钮"
                          onClick={() => {
                            const copy = customItemActions.filter((_, i) => i !== idx);
                            setDraft({ ...activeDraft, itemActions: copy });
                          }}
                          className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          );
        })()}
      </Modal>
    </div>
  );
}
