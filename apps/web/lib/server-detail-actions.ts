import type { Server } from "./types";
import type { MessageKey } from "./i18n";

type ResourceActionKind = "assignWorld" | "restoreBackup" | "migrate" | "modifyMods";

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
  serverStatus,
  targetCount
}: {
  kind: ResourceActionKind;
  serverStatus?: Server["status"];
  targetCount?: number;
}): { disabled: boolean; reasonKey?: MessageKey } {
  if ((kind === "assignWorld" || kind === "restoreBackup") && serverStatus === "running") {
    return { disabled: true, reasonKey: kind === "assignWorld" ? "assignWorldRequiresStopped" : "restoreRequiresStopped" };
  }
  if (kind === "modifyMods" && serverStatus === "running") {
    return { disabled: true, reasonKey: "modChangesRequireStopped" };
  }
  if (kind === "migrate" && (targetCount ?? 0) === 0) {
    return { disabled: true, reasonKey: "noMigrationTargetHint" };
  }
  return { disabled: false, reasonKey: undefined };
}
