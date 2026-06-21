import type { ServerStatus } from "./types";
import type { MessageKey } from "./i18n";

type ResourceActionKind = "restoreBackup" | "modifyMods";

type ServerDetailErrorCopy = {
  dockerUnavailable: string;
  containerUnavailable: string;
  portAlreadyAllocated?: (port: string) => string;
};

export function isServerLifecyclePending(status?: ServerStatus) {
  return status === "creating" || status === "starting" || status === "stopping" || status === "restarting" || status === "deleting";
}

export function isServerLockedForResourceChanges(status?: ServerStatus) {
  return status === "running" || isServerLifecyclePending(status);
}

export function formatServerDetailError(
  error: unknown,
  copy: ServerDetailErrorCopy = {
    dockerUnavailable: "Docker 未连接，请先在设置页完成 Docker Host 配置。",
    containerUnavailable: "Docker 容器不可用或已被外部删除，请重新启动服务器让面板恢复运行容器。",
    portAlreadyAllocated: (port) => `外部端口 ${port} 已被占用。请在配置中改为自动分配或换一个端口，然后重新启动服务器。`
  }
): string {
  const message = error instanceof Error ? error.message : String(error || "");
  const normalized = message.toLowerCase();
  const allocatedPort = extractAllocatedPort(message);
  if (allocatedPort) {
    return copy.portAlreadyAllocated?.(allocatedPort) ?? `外部端口 ${allocatedPort} 已被占用。请在配置中改为自动分配或换一个端口，然后重新启动服务器。`;
  }
  if (normalized.includes("docker runtime unavailable") || normalized.includes("cannot connect to docker")) {
    return copy.dockerUnavailable;
  }
  if (normalized.includes("page not found") || normalized.includes("no docker container found")) {
    return copy.containerUnavailable;
  }
  return message;
}

function extractAllocatedPort(message: string) {
  const normalized = message.toLowerCase();
  const isPortConflict =
    normalized.includes("port is already allocated") ||
    normalized.includes("address already in use") ||
    normalized.includes("external port") && normalized.includes("already used");

  if (!isPortConflict) return "";

  const explicitPort = message.match(/external port\s+(\d{2,5})\s+is already used/i);
  if (explicitPort) return explicitPort[1];

  const bindPort = message.match(/bind for\s+[^:]+:(\d{2,5})\s+failed/i);
  if (bindPort) return bindPort[1];

  const exposedPort = message.match(/(?:0\.0\.0\.0:|listen tcp .*?:)(\d{2,5})/i);
  return exposedPort?.[1] ?? "";
}

export function describeResourceAction({
  kind,
  serverStatus
}: {
  kind: ResourceActionKind;
  serverStatus?: ServerStatus;
}): { disabled: boolean; reasonKey?: MessageKey } {
  if (kind === "restoreBackup" && isServerLockedForResourceChanges(serverStatus)) {
    return { disabled: true, reasonKey: "restoreRequiresStopped" };
  }
  if (kind === "modifyMods" && isServerLifecyclePending(serverStatus)) {
    return { disabled: true, reasonKey: "modChangesLifecycleBusy" };
  }
  return { disabled: false, reasonKey: undefined };
}
