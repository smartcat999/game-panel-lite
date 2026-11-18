import { readToken } from "./session";

export interface WatchEvent<T> {
  type?: "ADDED" | "MODIFIED" | "DELETED" | "ERROR" | string;
  object?: T;
}

export function buildWsUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>,
  token?: string | null,
): string {
  const origin =
    typeof window !== "undefined" ? window.location.origin : "http://127.0.0.1:3000";
  const url = new URL(path, origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined) {
        url.searchParams.set(k, String(v));
      }
    }
  }
  const authToken = token ?? readToken();
  if (authToken) {
    url.searchParams.set("access_token", authToken);
  }
  return url.toString();
}

export interface ResourceWatchOptions<T> {
  path: string;
  token?: string | null;
  params?: Record<string, string | number | boolean | undefined>;
  onEvent: (event: WatchEvent<T>) => void;
  onConnectionChange?: (connected: boolean) => void;
  onReconnect?: () => void;
}

export function subscribeResourceWatch<T>({
  path,
  token,
  params,
  onEvent,
  onConnectionChange,
  onReconnect,
}: ResourceWatchOptions<T>): () => void {
  let closed = false;
  let backoffMs = 1000;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let socket: WebSocket | null = null;
  let hasOpenedBefore = false;

  const clearTimer = () => {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  const scheduleReconnect = () => {
    if (closed || reconnectTimer !== null) return;
    const wait = backoffMs;
    backoffMs = Math.min(backoffMs * 2, 15000);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, wait);
  };

  const connect = () => {
    if (closed || typeof WebSocket === "undefined") return;
    clearTimer();
    const wsUrl = buildWsUrl(path, { ...params, watch: true }, token);
    const ws = new WebSocket(wsUrl);
    socket = ws;

    ws.onopen = () => {
      if (closed) {
        ws.close(1000);
        return;
      }
      const wasReconnect = hasOpenedBefore;
      hasOpenedBefore = true;
      backoffMs = 1000;
      onConnectionChange?.(true);
      if (wasReconnect) {
        onReconnect?.();
      }
    };

    ws.onmessage = (msg) => {
      if (closed || typeof msg.data !== "string") return;
      const lines = msg.data.split("\n");
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        try {
          const parsed = JSON.parse(trimmed) as WatchEvent<T>;
          onEvent(parsed);
        } catch {
          // ignore malformed frame
        }
      }
    };

    ws.onerror = () => {
      onConnectionChange?.(false);
    };

    ws.onclose = () => {
      if (socket === ws) {
        socket = null;
      }
      onConnectionChange?.(false);
      if (!closed) {
        scheduleReconnect();
      }
    };
  };

  const onVisibilityChange = () => {
    if (
      !closed &&
      typeof document !== "undefined" &&
      document.visibilityState === "visible" &&
      socket === null
    ) {
      backoffMs = 1000;
      connect();
    }
  };

  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", onVisibilityChange);
  }

  connect();

  return () => {
    closed = true;
    clearTimer();
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", onVisibilityChange);
    }
    if (socket) {
      const target = socket;
      socket = null;
      target.onopen = null;
      target.onmessage = null;
      target.onerror = null;
      target.onclose = null;
      if (target.readyState === WebSocket.OPEN || target.readyState === WebSocket.CONNECTING) {
        target.close(1000);
      }
    }
    onConnectionChange?.(false);
  };
}

export type LogStreamState = "connecting" | "live" | "closed" | "error";

export interface TaskLogStreamOptions {
  path: string;
  token?: string | null;
  follow?: boolean;
  tailLines?: number;
  onReset?: () => void;
  onLines: (lines: string[]) => void;
  onStateChange?: (state: LogStreamState) => void;
}

export function subscribeTaskLogStream({
  path,
  token,
  follow = true,
  tailLines = 300,
  onReset,
  onLines,
  onStateChange,
}: TaskLogStreamOptions): () => void {
  let closed = false;
  let backoffMs = 1500;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let socket: WebSocket | null = null;
  let openedOnce = false;

  const clearTimer = () => {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  const connect = () => {
    if (closed || typeof WebSocket === "undefined") return;
    clearTimer();
    onStateChange?.("connecting");
    const wsUrl = buildWsUrl(path, { follow, tailLines }, token);
    const ws = new WebSocket(wsUrl);
    socket = ws;

    ws.onopen = () => {
      if (closed) {
        ws.close(1000);
        return;
      }
      if (openedOnce) {
        onReset?.();
      }
      openedOnce = true;
      backoffMs = 1500;
      onStateChange?.("live");
    };

    ws.onmessage = (msg) => {
      if (closed || typeof msg.data !== "string") return;
      const parts = msg.data.split(/\r?\n/);
      if (parts.length > 0 && parts[parts.length - 1] === "") {
        parts.pop();
      }
      if (parts.length > 0) {
        onLines(parts);
      }
    };

    ws.onerror = () => {
      if (!closed) {
        onStateChange?.("error");
      }
    };

    ws.onclose = (ev) => {
      if (socket === ws) {
        socket = null;
      }
      if (closed) return;
      if (ev.code === 1000 || !follow) {
        onStateChange?.("closed");
        return;
      }
      onStateChange?.("error");
      const wait = backoffMs;
      backoffMs = Math.min(backoffMs * 2, 10000);
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, wait);
    };
  };

  connect();

  return () => {
    closed = true;
    clearTimer();
    if (socket) {
      const target = socket;
      socket = null;
      target.onopen = null;
      target.onmessage = null;
      target.onerror = null;
      target.onclose = null;
      if (target.readyState === WebSocket.OPEN || target.readyState === WebSocket.CONNECTING) {
        target.close(1000);
      }
    }
  };
}
