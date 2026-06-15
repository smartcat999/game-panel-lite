import { describe, expect, it } from "vitest";
import { describeResourceAction, formatServerDetailError, isServerLockedForResourceChanges } from "./server-detail-actions";
import type { ServerStatus } from "./types";

describe("server detail action feedback", () => {
  it("turns raw Docker runtime errors into user-facing guidance", () => {
    expect(formatServerDetailError(new Error("Docker runtime unavailable: Cannot connect to Docker daemon"))).toBe(
      "Docker 未连接，请先在设置页完成 Docker Host 配置。"
    );
    expect(formatServerDetailError(new Error("Error response from daemon: page not found"))).toBe(
      "Docker 容器不可用或已被外部删除，请重新启动服务器让面板恢复运行容器。"
    );
    expect(formatServerDetailError(new Error("Error response from daemon: manifest for radioactivehydra/tmodloader:2024.10 not found: manifest unknown: manifest unknown"))).toBe(
      "Error response from daemon: manifest for radioactivehydra/tmodloader:2024.10 not found: manifest unknown: manifest unknown"
    );
  });

  it("explains why state-dependent resource actions are unavailable", () => {
    expect(describeResourceAction({ kind: "restoreBackup", serverStatus: "running" })).toEqual({
      disabled: true,
      reasonKey: "restoreRequiresStopped"
    });
  });

  it("requires stopped servers before modifying runtime mod files", () => {
    expect(describeResourceAction({ kind: "modifyMods", serverStatus: "running" })).toEqual({
      disabled: true,
      reasonKey: "modChangesRequireStopped"
    });
    expect(describeResourceAction({ kind: "modifyMods", serverStatus: "stopped" })).toEqual({
      disabled: false,
      reasonKey: undefined
    });
  });

  it("locks resource changes while lifecycle commands are still running", () => {
    const pendingStatuses: ServerStatus[] = ["creating", "starting", "restarting", "deleting"];
    expect(pendingStatuses.map((status) => isServerLockedForResourceChanges(status))).toEqual([true, true, true, true]);
    expect(isServerLockedForResourceChanges("stopped")).toBe(false);
  });
});
