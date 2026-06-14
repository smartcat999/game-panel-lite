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
