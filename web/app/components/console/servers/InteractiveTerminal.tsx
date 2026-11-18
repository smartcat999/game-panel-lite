"use client";

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import "@xterm/xterm/css/xterm.css";
import type { Terminal as XTermType } from "@xterm/xterm";
import type { FitAddon as FitAddonType } from "@xterm/addon-fit";
import { useConsole } from "../../../console/ConsoleProvider";
import { buildExecTtyWsUrl, buildNodeExecTtyWsUrl } from "./api";

export type TerminalConnectionStatus = "idle" | "connecting" | "connected" | "disconnected" | "error";

export interface InteractiveTerminalHandle {
  clear: () => void;
  reconnect: () => void;
  focus: () => void;
  fit: () => void;
}

interface InteractiveTerminalProps {
  name: string;
  targetScope?: "instance" | "node";
  running: boolean;
  activated: boolean;
  visible: boolean;
  onStatusChange?: (status: TerminalConnectionStatus, detail?: string) => void;
}

const LIGHT_THEME = {
  background: "#F7F8FC",
  foreground: "#14161F",
  cursor: "#1E293B",
  cursorAccent: "#FFFFFF",
  selectionBackground: "rgba(37, 99, 235, 0.2)",
  black: "#14161F",
  red: "#DC2626",
  green: "#15803D",
  yellow: "#B45309",
  blue: "#1D4ED8",
  magenta: "#7E22CE",
  cyan: "#0E7490",
  white: "#475569",
  brightBlack: "#64748B",
  brightRed: "#EF4444",
  brightGreen: "#16A34A",
  brightYellow: "#D97706",
  brightBlue: "#2563EB",
  brightMagenta: "#9333EA",
  brightCyan: "#0891B2",
  brightWhite: "#0F172A",
};

const MONO_FONT_FAMILY =
  'ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", monospace';

export const InteractiveTerminal = forwardRef<InteractiveTerminalHandle, InteractiveTerminalProps>(
  function InteractiveTerminal({ name, targetScope = "instance", running, activated, visible, onStatusChange }, ref) {
    const { tenant, token } = useConsole();
    const hostRef = useRef<HTMLDivElement>(null);
    const termRef = useRef<XTermType | null>(null);
    const fitRef = useRef<FitAddonType | null>(null);
    const wsRef = useRef<WebSocket | null>(null);
    const pingTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const encoderRef = useRef<TextEncoder | null>(null);
    const connectSeqRef = useRef(0);
    const hadErrorRef = useRef(false);
    const intentionalCloseRef = useRef(false);
    const prevTargetRef = useRef(targetScope === "node" ? `node/${name}` : `${tenant}/${name}`);
    const prevRunningRef = useRef(running);
    const cleanupSubsRef = useRef<(() => void) | null>(null);

    const tenantRef = useRef(tenant);
    const nameRef = useRef(name);
    const targetScopeRef = useRef(targetScope);
    const tokenRef = useRef(token);
    const runningRef = useRef(running);
    const visibleRef = useRef(visible);
    const onStatusRef = useRef(onStatusChange);

    tenantRef.current = tenant;
    nameRef.current = name;
    targetScopeRef.current = targetScope;
    tokenRef.current = token;
    runningRef.current = running;
    visibleRef.current = visible;
    onStatusRef.current = onStatusChange;

    const [termReady, setTermReady] = useState(false);

    const setStatus = useCallback((next: TerminalConnectionStatus, detail?: string) => {
      onStatusRef.current?.(next, detail);
    }, []);

    const stopHeartbeat = useCallback(() => {
      if (pingTimerRef.current !== null) {
        clearInterval(pingTimerRef.current);
        pingTimerRef.current = null;
      }
    }, []);

    const sendResizeFrame = useCallback(() => {
      const ws = wsRef.current;
      const term = termRef.current;
      if (!ws || ws.readyState !== WebSocket.OPEN || !term) return;
      if (term.cols <= 0 || term.rows <= 0) return;
      ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
    }, []);

    const fitTerminal = useCallback(() => {
      const host = hostRef.current;
      const fitAddon = fitRef.current;
      if (!host || !fitAddon || !visibleRef.current) return;
      if (host.clientWidth <= 0 || host.clientHeight <= 0) return;
      try {
        fitAddon.fit();
        sendResizeFrame();
      } catch {
        // Ignore transient measurement errors during portal/dock layout transitions.
      }
    }, [sendResizeFrame]);

    const closeSocket = useCallback(
      (quiet: boolean) => {
        connectSeqRef.current += 1;
        stopHeartbeat();
        const ws = wsRef.current;
        wsRef.current = null;
        if (!ws) return;
        intentionalCloseRef.current = quiet;
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
          ws.close(1000, "client reset");
        }
      },
      [stopHeartbeat],
    );

    const connect = useCallback(() => {
      const term = termRef.current;
      if (!term) return;
      closeSocket(true);

      if (!nameRef.current) {
        setStatus("disconnected", "暂无可用节点");
        return;
      }

      if (!runningRef.current) {
        const offlineTitle = targetScopeRef.current === "node" ? "节点未在线" : "服务器未运行";
        const offlineHint =
          targetScopeRef.current === "node"
            ? "\r\n\x1b[90m[节点未在线，恢复在线后可连接终端]\x1b[0m"
            : "\r\n\x1b[90m[服务器未运行，启动后可连接终端]\x1b[0m";
        setStatus("disconnected", offlineTitle);
        term.writeln(offlineHint);
        return;
      }

      fitTerminal();
      const cols = term.cols > 0 ? term.cols : 80;
      const rows = term.rows > 0 ? term.rows : 24;
      const url =
        targetScopeRef.current === "node"
          ? buildNodeExecTtyWsUrl(nameRef.current, cols, rows, tokenRef.current)
          : buildExecTtyWsUrl(tenantRef.current, nameRef.current, cols, rows, tokenRef.current);
      const seq = ++connectSeqRef.current;
      hadErrorRef.current = false;
      intentionalCloseRef.current = false;
      setStatus("connecting");

      const ws = new WebSocket(url);
      ws.binaryType = "arraybuffer";
      wsRef.current = ws;

      ws.addEventListener("open", () => {
        if (seq !== connectSeqRef.current) return;
        setStatus("connected");
        fitTerminal();
        sendResizeFrame();
        if (visibleRef.current) term.focus();
        stopHeartbeat();
        pingTimerRef.current = setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: "ping" }));
          }
        }, 25_000);
      });

      ws.addEventListener("message", (ev: MessageEvent) => {
        if (seq !== connectSeqRef.current) return;
        if (ev.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(ev.data));
          return;
        }
        if (ev.data instanceof Blob) {
          void ev.data.arrayBuffer().then((buf) => {
            if (seq === connectSeqRef.current) term.write(new Uint8Array(buf));
          });
          return;
        }
        if (typeof ev.data !== "string") return;
        try {
          const ctrl = JSON.parse(ev.data) as { type?: string; message?: string };
          if (ctrl?.type === "pong") return;
          if (ctrl?.type === "error") {
            hadErrorRef.current = true;
            const detail = ctrl.message || "终端会话异常";
            setStatus("error", detail);
            term.writeln(`\r\n\x1b[31m[终端错误] ${detail}\x1b[0m`);
            return;
          }
        } catch {
          // Non-JSON text frame; write verbatim below.
        }
        term.write(ev.data);
      });

      ws.addEventListener("error", () => {
        if (seq !== connectSeqRef.current) return;
        hadErrorRef.current = true;
        setStatus("error", "终端连接异常");
      });

      ws.addEventListener("close", () => {
        if (seq !== connectSeqRef.current) return;
        stopHeartbeat();
        if (wsRef.current === ws) wsRef.current = null;
        if (intentionalCloseRef.current) return;
        if (hadErrorRef.current) {
          setStatus("error", "终端连接已中断");
          term.writeln("\r\n\x1b[31m[连接已中断，按 Enter 或点击重连]\x1b[0m");
        } else {
          setStatus("disconnected");
          term.writeln("\r\n\x1b[90m[会话已结束，按 Enter 或点击重连]\x1b[0m");
        }
      });
    }, [closeSocket, fitTerminal, sendResizeFrame, setStatus, stopHeartbeat]);

    const connectRef = useRef(connect);
    connectRef.current = connect;

    useImperativeHandle(
      ref,
      () => ({
        clear: () => {
          termRef.current?.clear();
          termRef.current?.focus();
        },
        reconnect: () => {
          connectRef.current();
        },
        focus: () => {
          termRef.current?.focus();
        },
        fit: () => {
          fitTerminal();
        },
      }),
      [fitTerminal],
    );

    useEffect(() => {
      if (!activated || termRef.current) return;
      let disposed = false;
      let observer: ResizeObserver | null = null;
      let rafId = 0;

      void Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit")]).then(
        ([{ Terminal }, { FitAddon }]) => {
          if (disposed || !hostRef.current) return;
          const encoder = new TextEncoder();
          encoderRef.current = encoder;

          const term = new Terminal({
            cursorBlink: true,
            cursorStyle: "block",
            fontFamily: MONO_FONT_FAMILY,
            fontSize: 13,
            lineHeight: 1.4,
            scrollback: 5000,
            convertEol: false,
            theme: LIGHT_THEME,
          });
          const fitAddon = new FitAddon();
          term.loadAddon(fitAddon);
          term.open(hostRef.current);

          term.attachCustomKeyEventHandler((ev: KeyboardEvent) => {
            const key = ev.key.toLowerCase();
            if ((ev.metaKey || ev.ctrlKey) && !ev.altKey && (key === "i" || key === "j")) {
              return false;
            }
            if (ev.ctrlKey && !ev.metaKey && !ev.altKey && ev.key === "`") {
              return false;
            }
            const isCopyShortcut =
              ((ev.ctrlKey || ev.metaKey) && ev.shiftKey && key === "c") ||
              (ev.metaKey && !ev.ctrlKey && !ev.shiftKey && key === "c");
            if (isCopyShortcut && term.hasSelection()) {
              if (ev.type === "keydown" && typeof navigator !== "undefined") {
                void navigator.clipboard?.writeText(term.getSelection()).catch(() => undefined);
              }
              return false;
            }
            return true;
          });

          const dataSub = term.onData((data: string) => {
            const ws = wsRef.current;
            if (ws && ws.readyState === WebSocket.OPEN) {
              ws.send(encoder.encode(data));
              return;
            }
            if (ws && ws.readyState === WebSocket.CONNECTING) return;
            if (runningRef.current && data === "\r") {
              connectRef.current();
            }
          });

          const resizeSub = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
            const ws = wsRef.current;
            if (ws && ws.readyState === WebSocket.OPEN && cols > 0 && rows > 0) {
              ws.send(JSON.stringify({ type: "resize", cols, rows }));
            }
          });

          termRef.current = term;
          fitRef.current = fitAddon;

          if (typeof ResizeObserver !== "undefined") {
            observer = new ResizeObserver(() => {
              if (!visibleRef.current) return;
              cancelAnimationFrame(rafId);
              rafId = requestAnimationFrame(() => {
                fitTerminal();
              });
            });
            observer.observe(hostRef.current);
          }

          setTermReady(true);

          // Store disposables on closure cleanup ref by attaching listener cleanup
          const cleanupSubs = () => {
            dataSub.dispose();
            resizeSub.dispose();
            term.dispose();
          };
          cleanupSubsRef.current = cleanupSubs;
        },
      );

      return () => {
        disposed = true;
        cancelAnimationFrame(rafId);
        observer?.disconnect();
        closeSocket(true);
        cleanupSubsRef.current?.();
        cleanupSubsRef.current = null;
        termRef.current = null;
        fitRef.current = null;
        setTermReady(false);
      };
    }, [activated, closeSocket, fitTerminal]);

    useEffect(() => {
      if (!termReady) return;
      const targetKey = targetScope === "node" ? `node/${name}` : `${tenant}/${name}`;
      const targetChanged = prevTargetRef.current !== targetKey;
      prevTargetRef.current = targetKey;

      if (targetChanged) {
        prevRunningRef.current = running;
        termRef.current?.clear();
        connect();
        return;
      }

      const wasRunning = prevRunningRef.current;
      prevRunningRef.current = running;

      if (!running) {
        if (wasRunning || wsRef.current) {
          closeSocket(true);
          setStatus("disconnected", targetScope === "node" ? "节点未在线" : "服务器未运行");
          termRef.current?.writeln(
            targetScope === "node"
              ? "\r\n\x1b[90m[节点已离线，恢复在线后可重连终端]\x1b[0m"
              : "\r\n\x1b[90m[服务器已停止，启动后可重连终端]\x1b[0m",
          );
        }
        return;
      }

      if (!wsRef.current && (!wasRunning || activated)) {
        connect();
      }
    }, [activated, closeSocket, connect, name, running, setStatus, targetScope, tenant, termReady]);

    useEffect(() => {
      if (!termReady || !visible) return;
      const rafId = requestAnimationFrame(() => {
        fitTerminal();
        termRef.current?.focus();
      });
      return () => cancelAnimationFrame(rafId);
    }, [fitTerminal, termReady, visible]);

    return (
      <div
        className="gp-xterm relative h-full w-full bg-[#F7F8FC] px-3.5 py-3"
        onClick={() => termRef.current?.focus()}
      >
        <div ref={hostRef} className="h-full w-full" />
        {!running && (
          <div className="pointer-events-none absolute inset-x-0 bottom-3 flex justify-center">
            <span className="rounded-full border border-[#E3E6F2] bg-white/95 px-3 py-1 text-[11px] font-medium text-[#5B6075] shadow-xs">
              {targetScope === "node"
                ? "节点未在线，恢复在线后可连接终端"
                : "服务器未运行，请先启动服务器再连接终端"}
            </span>
          </div>
        )}
      </div>
    );
  },
);
