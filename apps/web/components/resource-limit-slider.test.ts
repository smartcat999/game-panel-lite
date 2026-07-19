import { describe, expect, it } from "vitest";
import { cpuResourceMarkers, formatMemoryResourceLimit, memoryResourceMarkers } from "./resource-limit-slider";

describe("resource limit slider helpers", () => {
  it("marks every whole CPU core on common hosts", () => {
    expect(cpuResourceMarkers(8).map((marker) => marker.value)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("marks every whole GB and annotates the recommended memory node", () => {
    expect(memoryResourceMarkers(8192, 6144, "建议")).toEqual([
      { label: "1G", value: 1024 },
      { label: "2G", value: 2048 },
      { label: "3G", value: 3072 },
      { label: "4G", value: 4096 },
      { label: "5G", value: 5120 },
      { label: "建议 6G", tone: "recommended", value: 6144 },
      { label: "7G", value: 7168 }
    ]);
  });

  it("formats memory in readable MB and GB units", () => {
    expect(formatMemoryResourceLimit(0, "不限")).toBe("不限");
    expect(formatMemoryResourceLimit(512, "不限")).toBe("512 MB");
    expect(formatMemoryResourceLimit(6912, "不限")).toBe("6.75 GB");
  });
});
