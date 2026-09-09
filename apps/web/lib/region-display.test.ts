import { describe, expect, it } from "vitest";
import { regionDisplayName } from "./region-display";

describe("region display names", () => {
  it("localizes the compatibility default without mixing languages", () => {
    const region = { id: "default", name: "默认区域 (Default Region)" };
    expect(regionDisplayName(region, "zh")).toBe("默认区域");
    expect(regionDisplayName(region, "en")).toBe("Default Region");
  });

  it("preserves operator-defined Region names", () => {
    expect(regionDisplayName({ id: "east", name: "East" }, "zh")).toBe("East");
  });
});
