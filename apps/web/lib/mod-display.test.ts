import { describe, expect, it } from "vitest";

import { dstModScope } from "./mod-display";
import type { ModFile } from "./types";

function mod(tags: string[]): ModFile {
  return {
    id: "mod-1",
    instanceId: "server-1",
    providerKey: "dont-starve-together",
    source: "workshop",
    workshopId: "376333686",
    fileName: "workshop-376333686",
    size: "1 MB",
    sizeBytes: 1,
    enabled: true,
    created: "2026-07-19T00:00:00Z",
    tags
  };
}

describe("dstModScope", () => {
  it("separates client-only mods from server-required mods", () => {
    expect(dstModScope(mod(["client_only_mod", "interface"]))).toBe("client");
    expect(dstModScope(mod(["all_clients_require_mod", "utility"]))).toBe("required");
  });
});
