"use client";

import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { formatBytes } from "../../../console/lib/format";
import { LogStreamState, subscribeTaskLogStream } from "../../../console/lib/ws";
import { Badge, Button, Modal, Switch } from "../ui";

const MAX_LINES = 1200;
const MAX_PAUSED_LINES = 5000;

interface TaskLogLine {
  id: number;
  text: string;
}

export function formatTaskByteValue(bytes?: number): string {
  if (bytes === undefined || bytes === null || !Number.isFinite(bytes) || bytes < 0) {
    return "-";
  }
  return formatBytes(bytes);
}

export function TaskLogModal({
  open,
  onClose,
  wsPath,
  token,
  title,
  description,
  badgeTone,
  badgeLabel,
  message,
  progress = 0,
  isIndeterminate = false,
  isActive = false,
  byteSummary,
  speedBytesPerSec = 0,
  backLabel = "关闭",
}: {
  open: boolean;
  onClose: () => void;
  wsPath: string;
  token?: string | null;
  title: string;
  description?: string;
  badgeTone: "success" | "warning" | "danger" | "accent" | "neutral";
  badgeLabel: string;
  message?: string;
  progress?: number;
  isIndeterminate?: boolean;
  isActive?: boolean;
  byteSummary?: string;
  speedBytesPerSec?: number;
  backLabel?: string;
}) {
  const [lines, setLines] = useState<TaskLogLine[]>([]);
  const [streamState, setStreamState] = useState<LogStreamState>("connecting");
  const [autoScroll, setAutoScroll] = useState(true);
  const autoScrollId = useId();
  const terminalRef = useRef<HTMLDivElement | null>(null);
  const autoScrollRef = useRef(true);
  const seqRef = useRef(0);
  const hasLinesRef = useRef(false);
  const displayedCountRef = useRef(0);
  const pausedResetRef = useRef(false);
  const pausedLinesRef = useRef<TaskLogLine[]>([]);

  useEffect(() => {
    if (!open || !wsPath) {
      hasLinesRef.current = false;
      displayedCountRef.current = 0;
      pausedResetRef.current = false;
      pausedLinesRef.current = [];
      setLines([]);
      return;
    }
    autoScrollRef.current = true;
    setAutoScroll(true);
    hasLinesRef.current = false;
    displayedCountRef.current = 0;
    pausedResetRef.current = false;
    pausedLinesRef.current = [];
    setLines([]);
    setStreamState("connecting");

    const stop = subscribeTaskLogStream({
      path: wsPath,
      token,
      follow: true,
      tailLines: 400,
      onReset: () => {
        if (!autoScrollRef.current && hasLinesRef.current) {
          pausedResetRef.current = true;
          pausedLinesRef.current = [];
          return;
        }
        hasLinesRef.current = false;
        displayedCountRef.current = 0;
        pausedResetRef.current = false;
        pausedLinesRef.current = [];
        setLines([]);
      },
      onLines: (incoming) => {
        if (incoming.length === 0) return;
        hasLinesRef.current = true;
        const nextItems: TaskLogLine[] = incoming.map((text) => ({ id: ++seqRef.current, text }));

        if (!autoScrollRef.current) {
          if (
            pausedResetRef.current ||
            pausedLinesRef.current.length > 0 ||
            displayedCountRef.current >= MAX_PAUSED_LINES
          ) {
            const mergedPaused = pausedLinesRef.current.concat(nextItems);
            pausedLinesRef.current =
              mergedPaused.length > MAX_LINES ? mergedPaused.slice(mergedPaused.length - MAX_LINES) : mergedPaused;
            return;
          }
          displayedCountRef.current += nextItems.length;
          setLines((prev) => prev.concat(nextItems));
          return;
        }

        displayedCountRef.current = Math.min(MAX_LINES, displayedCountRef.current + nextItems.length);
        setLines((prev) => {
          const merged = prev.concat(nextItems);
          return merged.length > MAX_LINES ? merged.slice(merged.length - MAX_LINES) : merged;
        });
      },
      onStateChange: (nextState) => setStreamState(nextState),
    });

    return () => {
      stop();
    };
  }, [open, wsPath, token]);

  useLayoutEffect(() => {
    if (!autoScroll || !terminalRef.current) return;
    terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
  }, [lines, autoScroll]);

  function handleAutoScrollChange(next: boolean) {
    autoScrollRef.current = next;
    setAutoScroll(next);
    if (!next) return;

    const wasReset = pausedResetRef.current;
    const pending = pausedLinesRef.current;
    pausedResetRef.current = false;
    pausedLinesRef.current = [];

    setLines((prev) => {
      const base = wasReset ? [] : prev;
      const merged = pending.length > 0 ? base.concat(pending) : base;
      return merged.length > MAX_LINES ? merged.slice(merged.length - MAX_LINES) : merged;
    });
    const baseCount = wasReset ? 0 : displayedCountRef.current;
    displayedCountRef.current = Math.min(MAX_LINES, baseCount + pending.length);
  }

  const clampedProgress = Math.min(100, Math.max(0, Math.round(progress)));
  const cleanMessage = message && message.trim() && message.trim() !== badgeLabel.trim() ? message.trim() : "";
  const progressToneClass = `gp-progress-${badgeTone}`;
  const barValue =
    !isActive && badgeTone === "success"
      ? 100
      : !isActive && badgeTone === "danger"
        ? clampedProgress > 0
          ? clampedProgress
          : 6
        : clampedProgress;

  const telemetrySubheader = (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-1 items-center gap-2.5">
          <span role="status" aria-live="polite" className="shrink-0">
            <Badge tone={badgeTone}>{badgeLabel}</Badge>
          </span>
          {cleanMessage && (
            <span
              className={
                badgeTone === "danger"
                  ? "truncate text-[13px] text-[#B42318]"
                  : "truncate text-[13px] text-[#5B6075]"
              }
              title={cleanMessage}
            >
              {cleanMessage}
            </span>
          )}
        </div>
        {(byteSummary || (isActive && speedBytesPerSec > 0) || (isActive && !isIndeterminate)) && (
          <div className="flex shrink-0 items-center gap-2.5 text-[13px] tabular-nums">
            {byteSummary && <span className="font-medium text-[#5B6075]">{byteSummary}</span>}
            {isActive && speedBytesPerSec > 0 && (
              <span className="text-[#8A90A6]">{formatTaskByteValue(speedBytesPerSec)}/s</span>
            )}
            {isActive && !isIndeterminate && (
              <span className="min-w-[3ch] text-right font-semibold text-[#14161F]">
                {clampedProgress}%
              </span>
            )}
          </div>
        )}
      </div>
      <div className="mt-2.5">
        {isIndeterminate ? (
          <progress
            max={100}
            className={`gp-progress ${progressToneClass} w-full`}
            aria-label="任务执行进度"
          />
        ) : (
          <progress
            max={100}
            value={barValue}
            className={`gp-progress ${progressToneClass} w-full`}
            aria-label="任务执行进度"
          />
        )}
      </div>
    </div>
  );

  return (
    <Modal
      open={open}
      onClose={onClose}
      width="max-w-3xl"
      title={title}
      description={description}
      subheader={telemetrySubheader}
      footer={
        <div className="flex w-full items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2">
              <Switch id={autoScrollId} label="自动滚动" checked={autoScroll} onChange={handleAutoScrollChange} />
              <label htmlFor={autoScrollId} className="cursor-pointer select-none text-xs font-medium text-[#5B6075]">
                自动滚动
              </label>
            </div>
            {streamState === "error" && (
              <span className="text-xs text-[#B42318]">连接中断，正在重连...</span>
            )}
          </div>
          <Button variant="secondary" onClick={onClose}>
            {backLabel}
          </Button>
        </div>
      }
    >
      <div
        ref={terminalRef}
        tabIndex={0}
        role="region"
        aria-label="日志内容"
        className="h-[380px] overflow-y-auto [overflow-anchor:none] rounded-2xl bg-[#0F1322] px-4 py-3.5 font-mono text-xs leading-relaxed text-[#E6EBFF] [color-scheme:dark] [scrollbar-width:thin] [scrollbar-color:#2D3554_transparent] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#2B3AE8]"
      >
        {lines.length === 0 ? (
          <div className="flex h-full items-center justify-center text-[#7D859F]">
            {streamState === "connecting"
              ? "正在加载日志..."
              : isActive
                ? "等待日志输出..."
                : "暂无日志输出"}
          </div>
        ) : (
          <div className="space-y-0.5 whitespace-pre-wrap [overflow-wrap:anywhere]">
            {lines.map((line) => (
              <div key={line.id} className="hover:bg-white/[0.04]">
                {line.text}
              </div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}
