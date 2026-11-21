"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import {
  ArrowLeft,
  ChevronRight,
  RotateCw,
  Terminal,
  Wrench,
  X,
} from "lucide-react";
import { useConsole } from "../../console/ConsoleProvider";
import { isTenantServerDetailPath } from "../../console/lib/routes";
import {
  InteractiveTerminal,
  type InteractiveTerminalHandle,
  type TerminalConnectionStatus,
} from "./servers/InteractiveTerminal";
import {
  Button,
  Select,
  focusRing,
} from "./ui";

export type ToolboxTool = "terminal";

export interface OpenToolboxDetail {
  tool?: ToolboxTool;
  node?: string;
}

export interface ToolboxDirectoryItem {
  key: string;
  icon: ReactNode;
  title: string;
  description?: string;
  active?: boolean;
  onSelect: () => void;
}

export interface ToolboxDirectoryGroup {
  title: string;
  badge?: ReactNode;
  items: ToolboxDirectoryItem[];
}

export function ToolboxDirectoryCard({
  subtitle,
  groups,
}: {
  subtitle?: string;
  groups: ToolboxDirectoryGroup[];
}) {
  return (
    <aside
      role="dialog"
      aria-label="工具箱目录"
      className="fixed bottom-20 right-6 z-50 flex w-[min(360px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white shadow-[0_20px_44px_-18px_rgba(36,46,66,0.25)]"
    >
      {/* KubeSphere Dark Banner Header */}
      <div className="border-b border-[#EDF0F7] bg-[#F5F7FA] p-3">
        <div className="flex items-center gap-3 rounded-[10px] bg-[#242E42] px-3.5 py-3 text-white">
          <span className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-[9px] bg-white/12 text-[#C7D4FF]">
            <Wrench className="h-4 w-4" aria-hidden />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[14px] font-bold leading-tight text-white">工具箱</div>
            {subtitle && <div className="mt-0.5 truncate text-[11px] text-[#9AA5B8]">{subtitle}</div>}
          </div>
        </div>
      </div>

      {/* Grouped Utility Items */}
      <div className="max-h-[min(380px,calc(100dvh-14rem))] space-y-3 overflow-y-auto bg-white p-3">
        {groups.map((group) => (
          <div key={group.title}>
            <div className="flex items-center justify-between px-1.5 pb-1.5 text-[11px] font-semibold text-[#8C93A8]">
              <span>{group.title}</span>
              {group.badge}
            </div>
            <div className="space-y-1">
              {group.items.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onClick={item.onSelect}
                  className={cx(
                    "flex w-full items-center gap-2.5 rounded-[11px] border px-2.5 py-2 text-left transition-all",
                    item.active
                      ? "border-[#CBD3FF] bg-[#EEF1FF]"
                      : "border-transparent bg-transparent hover:border-[#EDF0F7] hover:bg-[#F6F8FC]",
                    focusRing,
                  )}
                >
                  <span
                    className={cx(
                      "flex h-8 w-8 shrink-0 items-center justify-center rounded-[9px]",
                      item.active
                        ? "bg-white text-[#2B3AE8] shadow-[0_2px_6px_rgba(43,58,232,0.14)]"
                        : "bg-[#F1F4FA] text-[#46506B]",
                    )}
                  >
                    {item.icon}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span
                      className={cx(
                        "block truncate text-[13px] font-semibold leading-snug",
                        item.active ? "text-[#2B3AE8]" : "text-[#14161F]",
                      )}
                    >
                      {item.title}
                    </span>
                  </span>
                  <ChevronRight
                    className={cx(
                      "h-3.5 w-3.5 shrink-0",
                      item.active ? "text-[#2B3AE8]" : "text-[#8C93A8]",
                    )}
                    aria-hidden
                  />
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
}

export function openFloatingToolbox(detail: OpenToolboxDetail = {}): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<OpenToolboxDetail>("gp:open-toolbox", { detail }));
}

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export default function FloatingToolbox() {
  const pathname = usePathname();
  const { isAdmin, tenant, nodes } = useConsole();

  const isPlatformMode = Boolean(pathname?.startsWith("/console/admin")) && isAdmin;
  const hideDock = isTenantServerDetailPath(pathname);

  const [open, setOpen] = useState(false);
  const [stage, setStage] = useState<"menu" | "panel">("menu");
  const [selectedNode, setSelectedNode] = useState("");
  const [terminalActivated, setTerminalActivated] = useState(false);
  const [termStatus, setTermStatus] = useState<TerminalConnectionStatus>("idle");
  const terminalRef = useRef<InteractiveTerminalHandle>(null);

  const resolvedNodeName = useMemo(() => {
    if (selectedNode && nodes.some((n) => n.metadata.name === selectedNode)) {
      return selectedNode;
    }
    const readyNode = nodes.find((n) => (n.status?.phase || "") === "Ready");
    return readyNode?.metadata.name || nodes[0]?.metadata.name || selectedNode || "";
  }, [nodes, selectedNode]);

  const selectedNodeObj = useMemo(
    () => nodes.find((n) => n.metadata.name === resolvedNodeName),
    [nodes, resolvedNodeName],
  );
  const selectedNodeReady = (selectedNodeObj?.status?.phase || "") === "Ready";

  useEffect(() => {
    const onCustomOpen = (ev: Event) => {
      if (!isAdmin) return;
      const detail = (ev as CustomEvent<OpenToolboxDetail>).detail || {};
      if (detail.node) setSelectedNode(detail.node);
      setTerminalActivated(true);
      setStage("panel");
      setOpen(true);
    };
    window.addEventListener("gp:open-toolbox", onCustomOpen);
    return () => window.removeEventListener("gp:open-toolbox", onCustomOpen);
  }, [isAdmin]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && stage === "menu") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, stage]);

  useEffect(() => {
    if (!isAdmin || !open || stage !== "panel") return;
    setTerminalActivated(true);
    const id = setTimeout(() => {
      terminalRef.current?.fit();
      terminalRef.current?.focus();
    }, 20);
    return () => clearTimeout(id);
  }, [isAdmin, open, stage, resolvedNodeName]);

  const directoryGroups: ToolboxDirectoryGroup[] = useMemo(() => {
    if (!isAdmin) return [];
    return [
      {
        title: "节点运维",
        badge: nodes.length > 0 ? <span className="font-mono">{nodes.length} 台</span> : undefined,
        items: [
          {
            key: "terminal",
            icon: <Terminal className="h-4 w-4" />,
            title: "节点终端",
            active: stage === "panel",
            onSelect: () => {
              setStage("panel");
              setTerminalActivated(true);
            },
          },
        ],
      },
    ];
  }, [stage, isAdmin, nodes.length]);

  if (!isAdmin || hideDock) {
    return null;
  }

  const scopeLabel = isPlatformMode ? "全平台" : tenant || "当前工作区";
  const showPanel = open && stage === "panel";

  return (
    <>
      {open && stage === "menu" && (
        <ToolboxDirectoryCard
          subtitle={scopeLabel}
          groups={directoryGroups}
        />
      )}

      {(showPanel || terminalActivated) && (
        <aside
          role="dialog"
          aria-hidden={!showPanel}
          aria-label="节点终端"
          className={cx(
            "fixed bottom-20 right-6 z-50 flex h-[min(480px,calc(100dvh-8.5rem))] w-[min(620px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[16px] border border-[#E3E6F2] bg-white shadow-[0_20px_50px_-16px_rgba(20,22,31,0.22)]",
            !showPanel && "hidden",
          )}
        >
          {/* KubeSphere Stage 2 Single-Row Light-Gray Modal Header (46px) */}
          <div className="flex h-[46px] shrink-0 items-center justify-between gap-2 border-b border-[#E3E6F2] bg-[#F8F9FC] px-3">
            <div className="flex min-w-0 items-center gap-2">
              <button
                type="button"
                onClick={() => setStage("menu")}
                className={cx(
                  "inline-flex h-[26px] shrink-0 items-center gap-1 rounded-[7px] border border-[#E3E6F2] bg-white px-2 text-[11px] font-medium text-[#5B6075] transition-colors hover:border-[#CDD3E4] hover:text-[#14161F]",
                  focusRing,
                )}
              >
                <ArrowLeft className="h-3 w-3" />
                <span>工具箱</span>
              </button>
              <span className="h-4 w-px shrink-0 bg-[#E3E6F2]" aria-hidden />
              <div className="flex min-w-0 items-center gap-2 text-[13px] font-semibold text-[#14161F]">
                <span
                  className={cx(
                    "h-2 w-2 shrink-0 rounded-full",
                    !selectedNodeReady
                      ? "bg-[#8A90A6]"
                      : termStatus === "connected"
                        ? "bg-[#1F9D55]"
                        : termStatus === "connecting"
                          ? "animate-pulse bg-[#D97706]"
                          : termStatus === "error"
                            ? "bg-[#C83A2B]"
                            : "bg-[#8A90A6]",
                  )}
                  aria-hidden
                />
                <span className="hidden shrink-0 whitespace-nowrap truncate sm:inline">
                  节点终端
                </span>
                <Select
                  aria-label="切换管理节点"
                  value={resolvedNodeName}
                  onChange={(e) => setSelectedNode(e.target.value)}
                  disabled={nodes.length === 0}
                  className="!h-[26px] w-auto min-w-[124px] max-w-[180px] shrink-0 rounded-[7px] border-[#E3E6F2] bg-white !py-0 !pl-2.5 !pr-7 font-mono text-[11.5px] font-medium leading-normal text-[#14161F]"
                >
                  {nodes.length === 0 ? (
                    <option value="">暂无节点</option>
                  ) : (
                    nodes.map((n) => (
                      <option key={n.metadata.name} value={n.metadata.name}>
                        {n.metadata.name}
                        {(n.status?.phase || "") === "Ready" ? "" : " (离线)"}
                      </option>
                    ))
                  )}
                </Select>
                {terminalActivated &&
                  selectedNodeReady &&
                  termStatus !== "connected" &&
                  termStatus !== "idle" && (
                    <span
                      className={cx(
                        "shrink-0 text-[11px] font-medium",
                        termStatus === "connecting"
                          ? "text-[#B45309]"
                          : termStatus === "error"
                            ? "text-[#C83A2B]"
                            : "text-[#5B6075]",
                      )}
                    >
                      {termStatus === "connecting" ? "连接中" : termStatus === "error" ? "连接异常" : "已断开"}
                    </span>
                  )}
              </div>
            </div>

            <div className="flex shrink-0 items-center gap-1.5">
              {selectedNodeReady && (termStatus === "disconnected" || termStatus === "error") && (
                <Button
                  variant="secondary"
                  size="sm"
                  icon={<RotateCw className="h-3 w-3" />}
                  onClick={() => terminalRef.current?.reconnect()}
                  className="!h-[26px] !gap-1 !rounded-[7px] !border-[#C6D4FF] !bg-[#EEF1FF] !px-2 !text-[11px] !font-semibold !text-[#2B3AE8] hover:!bg-[#2B3AE8] hover:!text-white"
                >
                  重新连接
                </Button>
              )}
              <button
                type="button"
                title="清空屏幕 (Ctrl+L)"
                onClick={() => terminalRef.current?.clear()}
                className={cx(
                  "inline-flex h-[26px] items-center rounded-[7px] border border-[#E3E6F2] bg-white px-2.5 text-[11px] font-medium text-[#14161F] transition-colors hover:bg-[#F2F4FA]",
                  focusRing,
                )}
              >
                清屏
              </button>
              <button
                type="button"
                title="收起工具箱"
                aria-label="收起工具箱"
                onClick={() => setOpen(false)}
                className={cx(
                  "inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-[#5B6075] transition-colors hover:bg-[#E8ECF5] hover:text-[#14161F]",
                  focusRing,
                )}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          {terminalActivated && (
            <div className="min-h-0 flex-1 overflow-hidden bg-[#F7F8FC]">
              {nodes.length === 0 ? (
                <div className="flex h-full items-center justify-center p-6 text-xs text-[#8A90A6]">
                  暂无已注册的计算节点
                </div>
              ) : (
                <InteractiveTerminal
                  ref={terminalRef}
                  targetScope="node"
                  name={resolvedNodeName}
                  running={selectedNodeReady && Boolean(resolvedNodeName)}
                  activated={terminalActivated}
                  visible={showPanel}
                  onStatusChange={setTermStatus}
                />
              )}
            </div>
          )}
        </aside>
      )}

      {/* KubeSphere-Style Single Bottom-Right Icon Trigger Button */}
      <div className="fixed bottom-6 right-6 z-40">
        <button
          type="button"
          title={open ? "收起工具箱" : "打开工具箱"}
          aria-label={open ? "收起工具箱" : "打开工具箱"}
          aria-expanded={open}
          onClick={() => {
            if (open) {
              setOpen(false);
            } else {
              setStage("menu");
              setOpen(true);
            }
          }}
          className={cx(
            "inline-flex h-11 w-11 items-center justify-center rounded-full border shadow-[0_16px_38px_-12px_rgba(20,22,31,0.18)] backdrop-blur-md transition-all",
            open
              ? "border-[#2B3AE8] bg-[#2B3AE8] text-white hover:bg-[#2332D6]"
              : "border-[#E3E6F2] bg-white/95 text-[#14161F] hover:bg-[#F2F4FB]",
            focusRing,
          )}
        >
          <Wrench className={cx("h-4.5 w-4.5", open ? "text-white" : "text-[#2B3AE8]")} />
        </button>
      </div>
    </>
  );
}
