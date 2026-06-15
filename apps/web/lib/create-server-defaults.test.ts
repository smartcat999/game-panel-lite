import { describe, expect, it } from "vitest";
import { defaultCreateServerConfig, defaultCreateServerMode, defaultCreateServerPreset } from "./create-server-defaults";

describe("create server defaults", () => {
  it("starts new servers as vanilla with Chinese config defaults", () => {
    expect(defaultCreateServerMode).toBe("vanilla");
    expect(defaultCreateServerPreset).toBe("friends-casual");
    expect(defaultCreateServerConfig.language).toBe("zh-Hans");
    expect(defaultCreateServerConfig.port).toBe(7777);
  });
});
