import { describe, expect, it } from "vitest";
import { formatTransferBytes, runtimeInstallProgress } from "./runtime-install-progress";

describe("runtime install phase progress", () => {
  it("does not use legacy estimated percentages when no fixed total exists", () => {
    const p = runtimeInstallProgress({ image: "test", status: "preparing", phase: "downloading", progress: 95, currentBytes: 1024 });
    expect(p.percent).toBeUndefined();
    expect(p.current).toBe(1024);
  });
  it("measures only the current stage when a fixed total is provided", () => {
    const status = { image: "test", status: "preparing", phase: "downloading" as const, currentBytes: 200, totalBytes: 400 };
    expect(runtimeInstallProgress(status).percent).toBe(50);
    expect(runtimeInstallProgress({ ...status, phase: "extracting" }).percent).toBeUndefined();
  });
  it("hides stale speed and keeps a single phase label", () => {
    const status = { image: "test", status: "preparing", phase: "downloading" as const, bytesPerSecond: 1024, updatedAt: "2026-10-27T01:00:00Z" };
    const now = Date.parse(status.updatedAt);
    expect(runtimeInstallProgress(status, now + 1000).speed).toBe(1024);
    expect(runtimeInstallProgress(status, now + 6000).speed).toBe(0);
    expect(runtimeInstallProgress({ image: "test", status: "preparing", progress: 95 }).label).toBe("runtimeInstallResolving");
  });
  it("labels binary transfer units accurately", () => {
    expect(formatTransferBytes(1024 ** 3)).toBe("1.0 GiB");
    expect(formatTransferBytes(0)).toBe("0 B");
  });
});
