import { spawn } from "node:child_process";
import http from "node:http";
import https from "node:https";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const LISTEN_HOST = process.env.HOSTNAME || "0.0.0.0";
const LISTEN_PORT = Number(process.env.PORT || 3000);
const INTERNAL_HOST = "127.0.0.1";
const INTERNAL_PORT =
  Number(process.env.INTERNAL_NEXT_PORT || (LISTEN_PORT === 3001 ? 3002 : 3001));
const APISERVER_URL = new URL(
  process.env.GAMEPANEL_APISERVER_URL || "http://apiserver:8080",
);
const NODE_AGENT_URL = new URL(
  process.env.GAMEPANEL_NODE_AGENT_URL || "http://node-agent:8082",
);
const S3_URL = new URL(
  process.env.GAMEPANEL_S3_URL || "http://seaweedfs:8333",
);
const S3_BUCKET = (process.env.GP_S3_BUCKET || "gamepanel-backups").replace(/^\/+|\/+$/g, "");
const INTERNAL_URL = new URL(`http://${INTERNAL_HOST}:${INTERNAL_PORT}`);

function isNodeAgentFileTicket(rawUrl) {
  const [pathname = "", query = ""] = (rawUrl || "/").split("?");
  if (!pathname.endsWith("/files")) return false;
  return new URLSearchParams(query).has("ticket");
}

function isS3BucketPath(rawUrl) {
  const pathname = (rawUrl || "/").split("?")[0];
  return S3_BUCKET !== "" && (pathname === `/${S3_BUCKET}` || pathname.startsWith(`/${S3_BUCKET}/`));
}

function isBackendPath(rawUrl) {
  const pathname = (rawUrl || "/").split("?")[0];
  return (
    pathname === "/apis" ||
    pathname.startsWith("/apis/") ||
    pathname === "/openapi" ||
    pathname.startsWith("/openapi/") ||
    pathname === "/livez" ||
    pathname === "/readyz"
  );
}

function resolveTarget(rawUrl) {
  if (isNodeAgentFileTicket(rawUrl)) return NODE_AGENT_URL;
  if (isS3BucketPath(rawUrl)) return S3_URL;
  return isBackendPath(rawUrl) ? APISERVER_URL : INTERNAL_URL;
}

function targetPort(target) {
  if (target.port) return Number(target.port);
  return target.protocol === "https:" ? 443 : 80;
}

function writeRawHeaders(statusCode, statusMessage, rawHeaders) {
  let head = `HTTP/1.1 ${statusCode || 502} ${statusMessage || ""}\r\n`;
  for (let i = 0; i < rawHeaders.length; i += 2) {
    head += `${rawHeaders[i]}: ${rawHeaders[i + 1]}\r\n`;
  }
  return `${head}\r\n`;
}

const nextChild = spawn(process.execPath, [path.join(__dirname, "server.js")], {
  env: {
    ...process.env,
    HOSTNAME: INTERNAL_HOST,
    PORT: String(INTERNAL_PORT),
  },
  stdio: "inherit",
});

nextChild.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});

const server = http.createServer((req, res) => {
  req.on("error", () => {});
  res.on("error", () => {});
  const target = resolveTarget(req.url);
  const client = target.protocol === "https:" ? https : http;
  const proxyReq = client.request(
    {
      hostname: target.hostname,
      port: targetPort(target),
      path: req.url || "/",
      method: req.method,
      headers: {
        ...req.headers,
        host: target.host,
      },
    },
    (proxyRes) => {
      proxyRes.on("error", () => {
        if (!res.destroyed) res.destroy();
      });
      res.writeHead(proxyRes.statusCode || 502, proxyRes.headers);
      proxyRes.pipe(res);
    },
  );
  proxyReq.on("error", () => {
    if (!res.headersSent) {
      res.writeHead(502, { "Content-Type": "text/plain; charset=utf-8" });
    }
    if (!res.writableEnded) {
      res.end("Bad Gateway");
    }
  });
  req.pipe(proxyReq);
});

server.requestTimeout = 0;

server.on("upgrade", (req, socket, head) => {
  const safeDestroy = () => {
    if (!socket.destroyed) socket.destroy();
  };
  req.on("error", safeDestroy);
  socket.on("error", safeDestroy);
  socket.setTimeout(0);
  socket.setNoDelay(true);
  socket.setKeepAlive(true);

  const target = resolveTarget(req.url);
  const client = target.protocol === "https:" ? https : http;
  const proxyReq = client.request({
    hostname: target.hostname,
    port: targetPort(target),
    path: req.url || "/",
    method: req.method || "GET",
    headers: {
      ...req.headers,
      host: target.host,
    },
  });

  proxyReq.on("upgrade", (proxyRes, proxySocket, proxyHead) => {
    proxySocket.setTimeout(0);
    proxySocket.setNoDelay(true);
    proxySocket.setKeepAlive(true);

    socket.write(
      writeRawHeaders(
        proxyRes.statusCode || 101,
        proxyRes.statusMessage || "Switching Protocols",
        proxyRes.rawHeaders,
      ),
    );
    if (proxyHead && proxyHead.length > 0) {
      socket.write(proxyHead);
    }
    proxySocket.pipe(socket);
    socket.pipe(proxySocket);

    const teardown = () => {
      if (!socket.destroyed) socket.destroy();
      if (!proxySocket.destroyed) proxySocket.destroy();
    };
    socket.on("error", teardown);
    proxySocket.on("error", teardown);
    socket.on("close", teardown);
    proxySocket.on("close", teardown);
  });

  proxyReq.on("response", (proxyRes) => {
    proxyRes.on("error", safeDestroy);
    socket.write(
      writeRawHeaders(proxyRes.statusCode, proxyRes.statusMessage, proxyRes.rawHeaders),
    );
    proxyRes.pipe(socket);
  });

  proxyReq.on("error", () => {
    if (!socket.destroyed) {
      socket.end("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n");
    }
  });

  if (head && head.length > 0) {
    proxyReq.write(head);
  }
  proxyReq.end();
});

server.listen(LISTEN_PORT, LISTEN_HOST);

function shutdown(signal) {
  server.close(() => {
    if (!nextChild.killed) {
      nextChild.kill(signal);
    }
  });
}

process.on("SIGINT", () => shutdown("SIGINT"));
process.on("SIGTERM", () => shutdown("SIGTERM"));
