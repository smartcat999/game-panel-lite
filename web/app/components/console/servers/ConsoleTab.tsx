"use client";

import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { RotateCw, Terminal } from "lucide-react";
import { Button, Card, CardHeader, Switch, focusRing } from "../ui";
import { useServerShell } from "./api";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");
const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*[a-zA-Z]|\[\d+(?:;\d+)*m/g, "");

/** Keep the DOM light on long-running servers without shifting the viewport when paused. */
const MAX_LINES = 2000;
const MAX_PAUSED_LINES = 8000;
const RECONNECT_MS = 3000;

interface LogChunk {
  id: number;
  text: string;
  lines: number;
}

function countNewlines(text: string): number {
  let n = 0;
  for (let i = 0; i < text.length; i += 1) {
    if (text.charCodeAt(i) === 10) n += 1;
  }
  return n;
}

function trimTextToLastLines(text: string, maxLines: number): string {
  if (!text) return "";
  const lines = text.split("\n");
  return lines.length > maxLines ? lines.slice(-maxLines).join("\n") : text;
}

function trimChunksToMaxLines(chunks: LogChunk[], maxLines: number, nextId: () => number): LogChunk[] {
  let total = 0;
  for (const c of chunks) total += c.lines;
  if (total <= maxLines) return chunks;

  let start = 0;
  while (start < chunks.length && total - chunks[start].lines > maxLines) {
    total -= chunks[start].lines;
    start += 1;
  }
  const sliced = chunks.slice(start);
  if (sliced.length === 0) return [];
  const excess = total - maxLines;
  if (excess > 0) {
    const first = sliced[0];
    const trimmedText = first.text.split("\n").slice(excess).join("\n");
    sliced[0] = { id: nextId(), text: trimmedText, lines: countNewlines(trimmedText) };
  }
  return sliced;
}

type StreamState = "connecting" | "live" | "offline";

function LiveDot({ state }: { state: StreamState }) {
  const map: Record<StreamState, { dot: string; text: string; label: string }> = {
    live: { dot: "bg-[#1F9D55] animate-pulse", text: "text-[#1F7A45]", label: "实时" },
    connecting: { dot: "bg-[#B54708]", text: "text-[#B54708]", label: "连接中" },
    offline: { dot: "bg-[#8A90A6]", text: "text-[#8A90A6]", label: "已断开" },
  };
  const s = map[state];
  return (
    <span className={cx("inline-flex items-center gap-1.5 text-[13px] font-medium", s.text)} aria-live="polite">
      <span className={cx("h-1.5 w-1.5 rounded-full", s.dot)} aria-hidden />
      {s.label}
    </span>
  );
}

export function ConsoleTab({
  name,
  running = true,
  onOpenTerminal,
}: {
  name: string;
  running?: boolean;
  onOpenTerminal?: () => void;
}) {
  const { streamLogs } = useServerShell(name);
  const autoScrollId = useId();
  const [chunks, setChunks] = useState<LogChunk[]>([]);
  const [state, setState] = useState<StreamState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [session, setSession] = useState(0);
  const logRef = useRef<HTMLPreElement>(null);
  const autoScrollRef = useRef(true);
  const seqRef = useRef(0);
  const hasChunksRef = useRef(false);
  const displayedLinesRef = useRef(0);
  const pausedResetRef = useRef(false);
  const pausedTailRef = useRef("");

  useEffect(() => {
    const ctrl = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;

    const connect = async () => {
      if (ctrl.signal.aborted) return;
      setState("connecting");
      setError(null);
      // the follow stream replays the tail first, so start each connection from a clean buffer
      let first = true;
      try {
        await streamLogs((chunk) => {
          setState("live");
          const text = stripAnsi(chunk);
          if (!text) return;
          const isReplay = first;
          first = false;

          if (isReplay) {
            if (!autoScrollRef.current && hasChunksRef.current) {
              // Keep paused viewport frozen on background reconnect; buffer fresh tail until resumed.
              pausedResetRef.current = true;
              pausedTailRef.current = trimTextToLastLines(text, MAX_LINES);
              return;
            }
            hasChunksRef.current = true;
            pausedResetRef.current = false;
            pausedTailRef.current = "";
            const initialChunks = trimChunksToMaxLines(
              [{ id: ++seqRef.current, text, lines: countNewlines(text) }],
              MAX_LINES,
              () => ++seqRef.current,
            );
            displayedLinesRef.current = initialChunks.reduce((acc, c) => acc + c.lines, 0);
            setChunks(initialChunks);
            return;
          }

          hasChunksRef.current = true;
          const chunkLines = countNewlines(text);

          if (!autoScrollRef.current) {
            // Never slice from the top while autoScroll is off; head truncation shifts lines upward under fixed scrollTop.
            if (pausedResetRef.current || pausedTailRef.current || displayedLinesRef.current >= MAX_PAUSED_LINES) {
              pausedTailRef.current = trimTextToLastLines(pausedTailRef.current + text, MAX_LINES);
              return;
            }
            displayedLinesRef.current += chunkLines;
            setChunks((prev) => [...prev, { id: ++seqRef.current, text, lines: chunkLines }]);
            return;
          }

          displayedLinesRef.current = Math.min(MAX_LINES, displayedLinesRef.current + chunkLines);
          setChunks((prev) =>
            trimChunksToMaxLines(
              [...prev, { id: ++seqRef.current, text, lines: chunkLines }],
              MAX_LINES,
              () => ++seqRef.current,
            ),
          );
        }, ctrl.signal);
      } catch (err) {
        if (ctrl.signal.aborted) return;
        setError(err instanceof Error ? err.message : "读取日志失败");
      }
      if (ctrl.signal.aborted) return;
      // stream ended: container stopped or restarted — retry while the server should be running
      setState("offline");
      if (running) timer = setTimeout(() => void connect(), RECONNECT_MS);
    };

    void connect();
    return () => {
      ctrl.abort();
      if (timer) clearTimeout(timer);
    };
  }, [streamLogs, running, session]);

  useLayoutEffect(() => {
    if (autoScroll && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [chunks, autoScroll]);

  function handleAutoScrollChange(next: boolean) {
    autoScrollRef.current = next;
    setAutoScroll(next);
    if (!next) return;

    const wasReset = pausedResetRef.current;
    const pending = pausedTailRef.current;
    pausedResetRef.current = false;
    pausedTailRef.current = "";

    setChunks((prev) => {
      const base = wasReset ? [] : prev;
      const combined = pending
        ? [...base, { id: ++seqRef.current, text: pending, lines: countNewlines(pending) }]
        : base;
      return trimChunksToMaxLines(combined, MAX_LINES, () => ++seqRef.current);
    });
    const baseCount = wasReset ? 0 : displayedLinesRef.current;
    displayedLinesRef.current = Math.min(MAX_LINES, baseCount + (pending ? countNewlines(pending) : 0));
  }

  const placeholder =
    error ? `读取日志失败：${error}` : state === "connecting" ? "正在连接日志…" : running ? "暂无日志输出" : "服务器未运行";

  return (
    <Card className="overflow-hidden">
      <CardHeader
        title="服务器日志"
        actions={
          <>
            <LiveDot state={state} />
            <div className="inline-flex items-center gap-2">
              <Switch id={autoScrollId} checked={autoScroll} onChange={handleAutoScrollChange} />
              <label htmlFor={autoScrollId} className="cursor-pointer select-none text-[13px] text-[#5B6075]">
                自动滚动
              </label>
            </div>
            {onOpenTerminal && (
              <Button
                size="sm"
                variant="secondary"
                icon={<Terminal className="h-3.5 w-3.5" />}
                onClick={onOpenTerminal}
              >
                打开终端
              </Button>
            )}
            {state === "offline" && (
              <Button
                size="sm"
                icon={<RotateCw className="h-3.5 w-3.5" />}
                onClick={() => {
                  hasChunksRef.current = false;
                  pausedResetRef.current = false;
                  pausedTailRef.current = "";
                  setSession((s) => s + 1);
                }}
              >
                重新连接
              </Button>
            )}
          </>
        }
      />
      <div className="p-5">
        <pre
          ref={logRef}
          aria-label="服务器日志"
          tabIndex={0}
          className={cx(
            "h-[440px] overflow-auto [overflow-anchor:none] rounded-[16px] border border-[#E3E6F2] bg-[#F7F8FC] px-5 py-4 font-mono text-[12.5px] leading-relaxed whitespace-pre-wrap break-all text-[#14161F]",
            chunks.length === 0 && "text-[#8A90A6]",
            focusRing,
          )}
        >
          {chunks.length > 0 ? chunks.map((c) => <span key={c.id}>{c.text}</span>) : placeholder}
        </pre>
      </div>
    </Card>
  );
}
