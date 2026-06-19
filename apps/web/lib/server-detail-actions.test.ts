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
    expect(formatServerDetailError(new Error("failed to set up container networking: driver failed programming external connectivity on endpoint gamepanel: Bind for 0.0.0.0:7778 failed: port is already allocated"))).toBe(
      "外部端口 7778 已被占用。请在配置中改为自动分配或换一个端口，然后重新启动服务器。"
    );
    expect(formatServerDetailError(new Error("external port 7778 is already used"))).toBe(
      "外部端口 7778 已被占用。请在配置中改为自动分配或换一个端口，然后重新启动服务器。"
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

  it("allows mod edits while running and blocks lifecycle transitions", () => {
    expect(describeResourceAction({ kind: "modifyMods", serverStatus: "running" })).toEqual({
      disabled: false,
      reasonKey: undefined
    });
    expect(describeResourceAction({ kind: "modifyMods", serverStatus: "restarting" })).toEqual({
      disabled: true,
      reasonKey: "modChangesLifecycleBusy"
    });
    expect(describeResourceAction({ kind: "modifyMods", serverStatus: "stopped" })).toEqual({
      disabled: false,
      reasonKey: undefined
    });
  });

  it("locks resource changes while lifecycle commands are still running", () => {
    const pendingStatuses: ServerStatus[] = ["creating", "starting", "stopping", "restarting", "deleting"];
    expect(pendingStatuses.map((status) => isServerLockedForResourceChanges(status))).toEqual([true, true, true, true, true]);
    expect(isServerLockedForResourceChanges("stopped")).toBe(false);
  });
});
