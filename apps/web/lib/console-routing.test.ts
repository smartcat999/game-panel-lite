import { describe, expect, it } from "vitest";
import { consoleSurfaceForPathname, platformAreaForPathname } from "./console-routing";

describe("consoleSurfaceForPathname", () => {
  it("classifies the platform root and descendants as the platform application", () => {
    expect(consoleSurfaceForPathname("/platform")).toBe("platform");
    expect(consoleSurfaceForPathname("/platform/regions/region-a")).toBe("platform");
  });

  it("keeps tenant and similarly prefixed routes outside the platform application", () => {
    expect(consoleSurfaceForPathname("/servers")).toBe("tenant");
    expect(consoleSurfaceForPathname("/platforms")).toBe("tenant");
  });

  it("keeps account preferences and public pages outside both products", () => {
    expect(consoleSurfaceForPathname("/account/settings")).toBe("account");
    expect(consoleSurfaceForPathname("/")).toBe("public");
    expect(consoleSurfaceForPathname("/share/example")).toBe("public");
  });
});

describe("platformAreaForPathname", () => {
  it("keeps Region operations in infrastructure and global records in business control", () => {
    expect(platformAreaForPathname("/platform")).toBe("business");
    expect(platformAreaForPathname("/platform/organizations")).toBe("business");
    expect(platformAreaForPathname("/platform/instances/instance-a")).toBe("business");
    expect(platformAreaForPathname("/platform/regions")).toBe("infrastructure");
    expect(platformAreaForPathname("/platform/regions/east")).toBe("infrastructure");
  });

  it("does not invent a platform area outside the platform console", () => {
    expect(platformAreaForPathname("/servers")).toBeUndefined();
    expect(platformAreaForPathname("/account/settings")).toBeUndefined();
  });
});
