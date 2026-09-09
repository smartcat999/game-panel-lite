import { describe, expect, it } from "vitest";
import { consoleSurfaceForPathname } from "./console-routing";

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
