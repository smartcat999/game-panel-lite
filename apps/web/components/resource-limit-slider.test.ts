import { describe, expect, it } from "vitest";
import { formatCpuResourceLimit, formatMemoryResourceLimit } from "./resource-limit-slider";

describe("resource limit slider helpers", () => {
  it("formats CPU values without redundant scale labels", () => {
    expect(formatCpuResourceLimit(0, "不限", "核")).toBe("不限");
    expect(formatCpuResourceLimit(3, "不限", "核")).toBe("3 核");
  });

  it("formats memory in readable MB and GB units", () => {
    expect(formatMemoryResourceLimit(0, "不限")).toBe("不限");
    expect(formatMemoryResourceLimit(512, "不限")).toBe("512 MB");
    expect(formatMemoryResourceLimit(6912, "不限")).toBe("6.75 GB");
  });
});
