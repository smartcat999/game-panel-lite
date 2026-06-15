import type { Server } from "./types";
import type { MessageKey } from "./i18n";

type ResourceActionKind = "restoreBackup" | "modifyMods";

export function isServerLifecyclePending(status?: Server["status"]) {
  return status === "creating" || status === "starting" || status === "restarting" || status === "deleting";
}

export function isServerLockedForResourceChanges(status?: Server["status"]) {
  return status === "running" || isServerLifecyclePending(status);
}

export function formatServerDetailError(
  error: unknown,
  copy: {
    dockerUnavailable: string;
    containerUnavailable: string;
  } = {
    dockerUnavailable: "Docker 未连接，请先在设置页完成 Docker Host 配置。",
    containerUnavailable: "Docker 容器不可用或已被外部删除，请重新启动服务器让面板恢复运行容器。"
  }
): string {
  const message = error instanceof Error ? error.message : String(error || "");
  const normalized = message.toLowerCase();
  if (normalized.includes("docker runtime unavailable") || normalized.includes("cannot connect to docker")) {
    return copy.dockerUnavailable;
  }
  if (normalized.includes("page not found") || normalized.includes("no docker container found")) {
    return copy.containerUnavailable;
  }
  return message;
}

export function describeResourceAction({
  kind,
  serverStatus
}: {
  kind: ResourceActionKind;
  serverStatus?: Server["status"];
}): { disabled: boolean; reasonKey?: MessageKey } {
  if (kind === "restoreBackup" && isServerLockedForResourceChanges(serverStatus)) {
    return { disabled: true, reasonKey: "restoreRequiresStopped" };
  }
  if (kind === "modifyMods" && isServerLockedForResourceChanges(serverStatus)) {
    return { disabled: true, reasonKey: "modChangesRequireStopped" };
  }
  return { disabled: false, reasonKey: undefined };
}
