"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ArrowUp, Pencil, Star, Trash2 } from "lucide-react";
import type { AppConfigOption, AppConsoleAction, AppConsoleParam } from "../../../console/lib/types";
import { Badge, Button, IconButton, Input, Modal, Segmented, Select, Textarea } from "../ui";

const QUICK_VARS = ["item", "count", "player", "season", "msg"];

const DEFAULT_VAR_META: Record<
  string,
  { label: string; type: "text" | "number" | "select"; defaultValue: string; optionsText: string }
> = {
  item: { label: "物品代码", type: "text", defaultValue: "cutgrass", optionsText: "" },
  count: { label: "数量", type: "number", defaultValue: "10", optionsText: "" },
  player: { label: "目标玩家", type: "text", defaultValue: "@a", optionsText: "" },
  season: {
    label: "季节",
    type: "select",
    defaultValue: "autumn",
    optionsText: "春=spring, 夏=summer, 秋=autumn, 冬=winter",
  },
  msg: { label: "公告内容", type: "text", defaultValue: "服务器将在 5 分钟后重启维护", optionsText: "" },
};

function extractVars(tpl: string): string[] {
  const re = /\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g;
  const out: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(tpl || "")) !== null) {
    if (!out.includes(m[1])) out.push(m[1]);
  }
  return out;
}

export function optionsToText(options?: AppConfigOption[]): string {
  if (!options?.length) return "";
  return options.map((o) => (o.label && o.label !== o.value ? `${o.label}=${o.value}` : o.value)).join(", ");
}

export function parseOptionsText(raw: string): AppConfigOption[] {
  if (!raw || !raw.trim()) return [];
  return raw
    .split(/[,，]/)
    .map((part) => {
      const s = part.trim();
      if (!s) return null;
      const eq = s.indexOf("=");
      if (eq > 0) {
        return { label: s.slice(0, eq).trim(), value: s.slice(eq + 1).trim() };
      }
      return { label: s, value: s };
    })
    .filter((x): x is AppConfigOption => Boolean(x));
}

function renderHighlightedCommand(command: string) {
  const parts = (command || "").split(/(\{\{\s*[a-zA-Z0-9_]+\s*\}\})/g);
  return parts.map((part, idx) =>
    part.startsWith("{{") && part.endsWith("}}") ? (
      <span
        key={idx}
        className="mx-0.5 inline-block rounded bg-[#EEF1FF] px-1.5 py-0.2 font-mono font-semibold text-[#2B3AE8]"
      >
        {part}
      </span>
    ) : (
      <span key={idx}>{part}</span>
    ),
  );
}

function summarizeActionMode(action: AppConsoleAction): {
  tone: "neutral" | "success" | "accent" | "warning";
  badge: string;
  detail: string;
} {
  const params = action.params || [];
  if (params.length === 0) {
    return { tone: "neutral", badge: "一键执行", detail: "" };
  }
  if (params.length === 1 && params[0].type === "select" && (params[0].options?.length || 0) > 0) {
    return {
      tone: "success",
      badge: "枚举点选",
      detail: (params[0].options || []).map((o) => o.label).join(" / "),
    };
  }
  if (params.length === 1) {
    return {
      tone: "accent",
      badge: "单行填参",
      detail: params[0].label || params[0].name,
    };
  }
  return {
    tone: "warning",
    badge: `${params.length} 个参数`,
    detail: params.map((p) => p.label || p.name).join(" · "),
  };
}

export function AppConsoleActionsEditor({
  actions,
  onChange,
  jsonMode,
  onCloseJsonMode,
  editingId,
  onEditingIdChange,
  onToast,
}: {
  actions: AppConsoleAction[];
  onChange: (next: AppConsoleAction[]) => void;
  jsonMode: boolean;
  onCloseJsonMode: () => void;
  editingId: string | null;
  onEditingIdChange: (id: string | null) => void;
  onToast: (type: "success" | "error", msg: string) => void;
}) {
  const [catFilter, setCatFilter] = useState<string>("all");
  const [draft, setDraft] = useState<AppConsoleAction | null>(null);
  const [optionsTextMap, setOptionsTextMap] = useState<Record<string, string>>({});
  const [jsonText, setJsonText] = useState<string>(() => JSON.stringify(actions, null, 2));

  const cmdInputRef = useRef<HTMLInputElement>(null);
  const cursorRef = useRef<{ start: number; end: number } | null>(null);

  const categories = useMemo(() => {
    const counts: Record<string, number> = {};
    const order: string[] = [];
    for (const a of actions) {
      const g = (a.group || "常用指令").trim();
      if (!counts[g]) {
        counts[g] = 0;
        order.push(g);
      }
      counts[g] += 1;
    }
    return order.map((name) => ({ name, count: counts[name] }));
  }, [actions]);

  useEffect(() => {
    if (catFilter !== "all" && !categories.some((c) => c.name === catFilter)) {
      setCatFilter("all");
    }
  }, [catFilter, categories]);

  const filteredActions = useMemo(
    () => actions.filter((a) => catFilter === "all" || (a.group || "常用指令") === catFilter),
    [actions, catFilter],
  );

  const startInlineEdit = (action: AppConsoleAction) => {
    const cloned: AppConsoleAction = JSON.parse(JSON.stringify(action));
    const map: Record<string, string> = {};
    for (const p of cloned.params || []) {
      map[p.name] = p.type === "select" ? optionsToText(p.options) : (p.defaultValue || "");
    }
    setOptionsTextMap(map);
    setDraft(cloned);
    onEditingIdChange(action.id);
  };

  // Sync draft if parent triggered "+ 新增指令" by setting editingId to a newly prepended action
  const activeDraft = useMemo(() => {
    if (!editingId) return null;
    if (draft && draft.id === editingId) return draft;
    const found = actions.find((a) => a.id === editingId);
    if (!found) return null;
    return JSON.parse(JSON.stringify(found)) as AppConsoleAction;
  }, [editingId, draft, actions]);

  const updateDraftCommand = (nextCmd: string) => {
    const base = activeDraft;
    if (!base) return;
    const vars = extractVars(nextCmd);
    const oldMap = new Map((base.params || []).map((p) => [p.name, p]));
    const nextParams: AppConsoleParam[] = vars.map((v) => {
      const existing = oldMap.get(v);
      if (existing) return existing;
      const meta = DEFAULT_VAR_META[v] || { label: v, type: "text", defaultValue: "", optionsText: "" };
      return {
        name: v,
        label: meta.label,
        type: meta.type,
        defaultValue: meta.defaultValue,
        options: meta.type === "select" ? parseOptionsText(meta.optionsText) : undefined,
      };
    });
    const nextMap = { ...optionsTextMap };
    for (const p of nextParams) {
      if (nextMap[p.name] === undefined) {
        nextMap[p.name] = p.type === "select" ? optionsToText(p.options) : (p.defaultValue || "");
      }
    }
    setOptionsTextMap(nextMap);
    setDraft({
      ...base,
      command: nextCmd,
      params: nextParams,
    });
  };

  const insertVarAtCursor = (varName: string) => {
    if (!activeDraft) return;
    const input = cmdInputRef.current;
    const token = `{{${varName}}}`;
    const cur = activeDraft.command || "";
    const start =
      input && typeof input.selectionStart === "number"
        ? input.selectionStart
        : (cursorRef.current?.start ?? cur.length);
    const end =
      input && typeof input.selectionEnd === "number" ? input.selectionEnd : (cursorRef.current?.end ?? start);
    const nextCmd = cur.slice(0, start) + token + cur.slice(end);
    updateDraftCommand(nextCmd);
    const nextPos = start + token.length;
    cursorRef.current = { start: nextPos, end: nextPos };
    setTimeout(() => {
      if (cmdInputRef.current) {
        cmdInputRef.current.focus();
        cmdInputRef.current.setSelectionRange(nextPos, nextPos);
      }
    }, 0);
  };

  const commitInlineEdit = () => {
    if (!activeDraft) return;
    if (!activeDraft.title.trim() || !activeDraft.command.trim()) {
      onToast("error", "请填写指令名称与游戏内指令模板");
      return;
    }
    const nextList = actions.map((a) => (a.id === activeDraft.id ? activeDraft : a));
    onChange(nextList);
    setDraft(null);
    onEditingIdChange(null);
  };

  const moveRow = (id: string, delta: number) => {
    const idx = actions.findIndex((a) => a.id === id);
    if (idx < 0) return;
    const target = idx + delta;
    if (target < 0 || target >= actions.length) return;
    const copy = [...actions];
    const [item] = copy.splice(idx, 1);
    copy.splice(target, 0, item);
    onChange(copy);
  };

  const deleteRow = (id: string) => {
    onChange(actions.filter((a) => a.id !== id));
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
            直接编辑 <code className="font-mono text-[#2B3AE8]">spec.consoleActions</code> JSON 数组
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
                  const list = Array.isArray(parsed) ? parsed : parsed.consoleActions;
                  if (!Array.isArray(list)) {
                    throw new Error("JSON 需为指令数组");
                  }
                  onChange(list);
                  onCloseJsonMode();
                  onToast("success", `已应用 ${list.length} 条游戏指令配置`);
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
      {categories.length > 1 && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-3.5">
          {categories.length <= 5 ? (
            <Segmented
              size="sm"
              label="指令分类筛选"
              value={catFilter}
              onChange={setCatFilter}
              items={[
                { value: "all", label: "全部" },
                ...categories.map((c) => ({ value: c.name, label: c.name })),
              ]}
            />
          ) : (
            <Select
              aria-label="指令分类筛选"
              value={catFilter}
              onChange={(e) => setCatFilter(e.target.value)}
              className="h-8 w-auto text-xs"
            >
              <option value="all">全部分类</option>
              {categories.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </Select>
          )}
          <span className="text-xs text-[#8A90A6]">共 {filteredActions.length} 条指令</span>
        </div>
      )}

      <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="游戏指令列表">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-[#EEF0F7] bg-[#FAFBFD] text-xs text-[#8A90A6]">
            <tr>
              <th className="w-[24%] px-6 py-3 font-medium">指令名称</th>
              <th className="w-[38%] px-6 py-3 font-medium">游戏内指令模板</th>
              <th className="w-[24%] px-6 py-3 font-medium">交互方式与参数</th>
              <th className="w-[14%] px-6 py-3 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#EEF0F7] text-sm">
            {filteredActions.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-6 py-14 text-center text-sm text-[#8A90A6]">
                  当前分类下暂无游戏指令模板，点击右上角「+ 新增指令」创建
                </td>
              </tr>
            ) : (
              filteredActions.map((item) => {
                const summary = summarizeActionMode(item);
                return (
                  <tr key={item.id} className="transition-colors hover:bg-[#FAFBFD]">
                    <td className="px-6 py-4">
                      <div className="font-medium text-[#14161F]">{item.title}</div>
                      {item.group && (
                        <div className="mt-0.5 text-xs text-[#8A90A6]">{item.group}</div>
                      )}
                    </td>
                    <td className="px-6 py-4 font-mono text-xs text-[#14161F]">
                      {renderHighlightedCommand(item.command)}
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone={summary.tone}>{summary.badge}</Badge>
                        {summary.detail && (
                          <span className="text-xs text-[#5B6075]">{summary.detail}</span>
                        )}
                      </div>
                    </td>
                    <td className="whitespace-nowrap px-6 py-4 text-right">
                      <div className="inline-flex items-center gap-1">
                        <IconButton label={`上移指令 ${item.title}`} onClick={() => moveRow(item.id, -1)}>
                          <ArrowUp className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton label={`下移指令 ${item.title}`} onClick={() => moveRow(item.id, 1)}>
                          <ArrowDown className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton label={`编辑指令 ${item.title}`} onClick={() => startInlineEdit(item)}>
                          <Pencil className="h-3.5 w-3.5" aria-hidden />
                        </IconButton>
                        <IconButton
                          label={`删除指令 ${item.title}`}
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
        title={`编辑指令 · ${activeDraft?.title || "未命名指令"}`}
        description={activeDraft?.group ? `所属分类：${activeDraft.group}` : "配置游戏指令模板与交互参数"}
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
          let previewResolvedCmd = activeDraft.command || "";
          for (const p of activeDraft.params || []) {
            const sample =
              p.defaultValue || (p.type === "select" && p.options?.[0]?.value) || `<${p.label || p.name}>`;
            previewResolvedCmd = previewResolvedCmd.replace(
              new RegExp(`\\{\\{\\s*${p.name}\\s*\\}\\}`, "g"),
              sample,
            );
          }
          return (
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-3 md:grid-cols-12">
                <div className="md:col-span-3">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    指令名称
                  </label>
                  <Input
                    value={activeDraft.title}
                    onChange={(e) => setDraft({ ...activeDraft, title: e.target.value })}
                    placeholder="如：给指定玩家刷物品"
                    className="h-9 bg-white text-xs"
                  />
                </div>
                <div className="md:col-span-3">
                  <label className="mb-1.5 block text-[11.5px] font-medium text-[#5B6075]">
                    所属分类
                  </label>
                  <Input
                    list="adm-cmd-groups"
                    value={activeDraft.group || ""}
                    onChange={(e) => setDraft({ ...activeDraft, group: e.target.value })}
                    placeholder="如：玩家管理"
                    className="h-9 bg-white text-xs"
                  />
                  <datalist id="adm-cmd-groups">
                    {categories.map((c) => (
                      <option key={c.name} value={c.name} />
                    ))}
                  </datalist>
                </div>
                <div className="md:col-span-6">
                  <div className="mb-1.5 flex flex-wrap items-center justify-between gap-2">
                    <label htmlFor="console-action-cmd-tpl" className="text-[11.5px] font-medium text-[#5B6075]">
                      游戏内指令模板
                    </label>
                    <div className="flex flex-wrap items-center gap-1">
                      <span className="text-[11px] text-[#8A90A6]">插入变量:</span>
                      {QUICK_VARS.map((v) => (
                        <button
                          key={v}
                          type="button"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => insertVarAtCursor(v)}
                          className="rounded bg-[#F2F4F9] px-1.5 py-0.5 font-mono text-[10.5px] font-medium text-[#5B6075] transition-colors hover:bg-[#EEF1FF] hover:text-[#2B3AE8]"
                        >
                          +{`{{${v}}}`}
                        </button>
                      ))}
                    </div>
                  </div>
                  <Input
                    id="console-action-cmd-tpl"
                    ref={cmdInputRef}
                    type="text"
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    value={activeDraft.command}
                    onChange={(e) => updateDraftCommand(e.target.value)}
                    onKeyUp={(e) => {
                      cursorRef.current = {
                        start: e.currentTarget.selectionStart ?? e.currentTarget.value.length,
                        end: e.currentTarget.selectionEnd ?? e.currentTarget.value.length,
                      };
                    }}
                    onClick={(e) => {
                      cursorRef.current = {
                        start: e.currentTarget.selectionStart ?? e.currentTarget.value.length,
                        end: e.currentTarget.selectionEnd ?? e.currentTarget.value.length,
                      };
                    }}
                    placeholder='如：c_give("{{item}}", {{count}})'
                    className="h-9 border-[#DCE1EE] px-3 font-mono text-xs"
                  />
                </div>
              </div>

              {(activeDraft.params?.length || 0) > 0 && (
                <div className="space-y-2 border-t border-[#EEF0F7] pt-3.5">
                  <div className="grid grid-cols-[96px_1.1fr_0.9fr_1.8fr] gap-2.5 text-[11px] font-medium text-[#5B6075]">
                    <span>模板变量</span>
                    <span>输入框中文标题</span>
                    <span>控件类型</span>
                    <span>默认值或枚举项 (如 春=spring, 夏=summer)</span>
                  </div>
                  {(activeDraft.params || []).map((param, pIdx) => {
                    const rawValOrOpts =
                      optionsTextMap[param.name] ??
                      (param.type === "select"
                        ? optionsToText(param.options)
                        : (param.defaultValue ?? ""));
                    return (
                      <div
                        key={param.name}
                        className="grid grid-cols-[96px_1.1fr_0.9fr_1.8fr] items-center gap-2.5"
                      >
                        <code className="rounded-lg bg-[#F3F5FA] px-2.5 py-1.5 text-center font-mono text-xs font-medium text-[#3D4258]">
                          {`{{${param.name}}}`}
                        </code>
                        <Input
                          value={param.label}
                          onChange={(e) => {
                            const nextParams = [...(activeDraft.params || [])];
                            nextParams[pIdx] = { ...param, label: e.target.value };
                            setDraft({ ...activeDraft, params: nextParams });
                          }}
                          placeholder="参数中文标题"
                          className="h-8.5 bg-white text-xs"
                        />
                        <Select
                          value={param.type || "text"}
                          onChange={(e) => {
                            const nextType = e.target.value as "text" | "number" | "select";
                            const nextParams = [...(activeDraft.params || [])];
                            const curText = optionsTextMap[param.name] || "";
                            const seededText =
                              nextType === "select" && !curText.includes("=")
                                ? "选项1=val1, 选项2=val2"
                                : curText;
                            setOptionsTextMap((prev) => ({ ...prev, [param.name]: seededText }));
                            nextParams[pIdx] = {
                              ...param,
                              type: nextType,
                              options: nextType === "select" ? parseOptionsText(seededText) : undefined,
                              defaultValue: nextType === "select" ? undefined : seededText,
                            };
                            setDraft({ ...activeDraft, params: nextParams });
                          }}
                          className="h-8.5 bg-white text-xs"
                        >
                          <option value="text">文本输入</option>
                          <option value="number">数字输入</option>
                          <option value="select">枚举点选 / 下拉</option>
                        </Select>
                        <Input
                          value={rawValOrOpts}
                          onChange={(e) => {
                            const val = e.target.value;
                            setOptionsTextMap((prev) => ({ ...prev, [param.name]: val }));
                            const nextParams = [...(activeDraft.params || [])];
                            if (param.type === "select") {
                              const parsedOpts = parseOptionsText(val);
                              nextParams[pIdx] = {
                                ...param,
                                options: parsedOpts,
                                defaultValue: parsedOpts[0]?.value,
                              };
                            } else {
                              nextParams[pIdx] = { ...param, defaultValue: val };
                            }
                            setDraft({ ...activeDraft, params: nextParams });
                          }}
                          placeholder={
                            param.type === "select"
                              ? "如 春=spring, 夏=summer"
                              : "选填默认初始值"
                          }
                          className="h-8.5 bg-white text-xs"
                        />
                      </div>
                    );
                  })}
                </div>
              )}

              <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-[#F7F8FC] px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-[11.5px] font-medium text-[#5B6075]">实时预览：</span>
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-[#14161F]">
                    <Star className="h-3.5 w-3.5 text-[#F59E0B]" aria-hidden />
                    {activeDraft.title || "未命名指令"}
                  </span>
                  <code className="rounded border border-[#E5E8F0] bg-white px-2 py-0.5 font-mono text-[11.5px] text-[#3D4258]">
                    {previewResolvedCmd}
                  </code>
                </div>
              </div>
            </div>
          );
        })()}
      </Modal>
    </div>
  );
}
