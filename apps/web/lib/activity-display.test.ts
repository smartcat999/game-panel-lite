import { describe, expect, it } from "vitest";
import { formatActivityEvent } from "./activity-display";
import type { ActivityEvent } from "./types";

const baseEvent: ActivityEvent = {
  id: "activity-1",
  instanceId: "server-1",
  type: "server.started",
  message: "Started server Friends",
  created: "Just now"
};

describe("formatActivityEvent", () => {
  it("localizes common server activity in Chinese", () => {
    expect(formatActivityEvent(baseEvent, "zh")).toEqual({
      message: "已启动服务器 Friends",
      typeLabel: "服务器启动"
    });
  });

  it("localizes resource activity messages with extracted names", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "world.assigned",
      message: "Assigned world Journey to Friends"
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已将世界 Journey 设为 Friends 的当前世界",
      typeLabel: "世界切换"
    });
  });

  it("keeps the backend message in English locale while presenting a friendly type label", () => {
    expect(formatActivityEvent(baseEvent, "en")).toEqual({
      message: "Started server Friends",
      typeLabel: "Server Started"
    });
  });

  it("localizes queued lifecycle activity in Chinese", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.restart.queued",
      message: "Queued restart for server Friends Server"
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已提交重启服务器 Friends Server",
      typeLabel: "重启排队"
    });
  });

  it("prefers structured server payload over parsing backend messages", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.start.queued",
      message: "Queued start for server stale-name",
      payload: { serverName: "Friends Server" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已提交启动服务器 Friends Server",
      typeLabel: "启动排队"
    });
  });

  it("localizes world snapshot activity in Chinese", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "world.snapshot.created",
      message: "Saved world snapshot Friends World from Friends Server"
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已从 Friends Server 保存世界快照 Friends World",
      typeLabel: "世界快照"
    });
  });

  it("localizes settings locale activity in Chinese", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      instanceId: "",
      type: "settings.locale",
      message: "Updated locale to \"zh\""
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已将界面语言切换为中文",
      typeLabel: "语言设置"
    });
  });

  it("formats settings locale from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      instanceId: "",
      type: "settings.locale",
      message: "Updated locale to stale",
      payload: { locale: "en" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已将界面语言切换为英文",
      typeLabel: "语言设置"
    });
  });

  it("formats world activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "world.assigned",
      message: "Assigned world stale-world to stale-server",
      payload: { worldName: "旅途世界", serverName: "朋友服务器" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已将世界 旅途世界 设为 朋友服务器 的当前世界",
      typeLabel: "世界切换"
    });
  });

  it("formats backup activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "backup.created",
      message: "Created backup stale.zip for stale-server",
      payload: { fileName: "friends-backup.zip", serverName: "朋友服务器" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已为 朋友服务器 创建备份 friends-backup.zip",
      typeLabel: "备份创建"
    });
  });

  it("formats workshop import activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "mod.workshop_imported",
      message: "Imported 1 workshop mod IDs for stale-server",
      payload: { workshopCount: 3, serverName: "模组服务器" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已为 模组服务器 导入 3 个创意工坊模组",
      typeLabel: "创意工坊导入"
    });
  });

  it("formats player activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "player.whitelist.removed",
      message: "Removed player stale-player from stale-server whitelist",
      payload: { playerName: "Alex", serverName: "朋友服务器" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已将玩家 Alex 从 朋友服务器 白名单移除",
      typeLabel: "白名单更新"
    });
  });

  it("formats controller runtime activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.runtime.created",
      message: "Created runtime workload for server stale-server",
      payload: { serverName: "朋友服务器", runtimeId: "runtime-1" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "已创建 朋友服务器 的运行实例",
      typeLabel: "运行实例已创建"
    });
  });

  it("formats reconcile failure activity from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.reconcile.failed",
      message: "stale-server: stale error",
      payload: { serverName: "朋友服务器", lastError: "Docker runtime unavailable" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "朋友服务器 同步失败：Docker 未连接，请先在设置页完成 Docker Host 配置。",
      typeLabel: "同步失败"
    });
  });

  it("formats container lifecycle failure details from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.container.start.failed",
      message: "Start runtime container failed for server stale-server: Docker runtime unavailable",
      payload: { serverName: "朋友服务器", runtimeId: "runtime-1", error: "Docker runtime unavailable" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "朋友服务器 容器启动失败：Docker 未连接，请先在设置页完成 Docker Host 配置。（容器：runtime-1）",
      typeLabel: "容器启动失败"
    });
  });

  it("formats image lifecycle details from structured payload", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "server.image.load.failed",
      message: "Load runtime image failed for server stale-server: runtime image archive is missing",
      payload: { serverName: "朋友服务器", image: "gamepanel/dst:latest", error: "runtime image archive is missing" }
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "朋友服务器 镜像加载失败：runtime image archive is missing（镜像：gamepanel/dst:latest）",
      typeLabel: "镜像加载失败"
    });
  });

  it("falls back to raw values for unknown activity types", () => {
    const event: ActivityEvent = {
      ...baseEvent,
      type: "custom.event",
      message: "Custom event message"
    };

    expect(formatActivityEvent(event, "zh")).toEqual({
      message: "Custom event message",
      typeLabel: "custom.event"
    });
  });
});
