import { describe, expect, it } from "vitest";

import { dstConfiguredWorkshopIds, dstModScope, isServerAssignableMod, mergeConfiguredWorkshopMods, modRuntimeState } from "./mod-display";
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
    expect(dstModScope(mod(["server_only_mod", "utility"]))).toBe("server");
  });

  it("only allows classified DST server mods to be assigned", () => {
    expect(isServerAssignableMod(mod(["client_only_mod"]))).toBe(false);
    expect(isServerAssignableMod(mod([]))).toBe(false);
    expect(isServerAssignableMod(mod(["server_only_mod"]))).toBe(true);
    expect(isServerAssignableMod(mod(["all_clients_require_mod"]))).toBe(true);
  });
});

describe("DST configured mod display", () => {
  it("reads unique Workshop IDs from the saved server config", () => {
    expect(dstConfiguredWorkshopIds({ mods: { workshopIds: [" 376333686 ", "376333686", 378160973] } })).toEqual([
      "376333686",
      "378160973"
    ]);
  });

  it("fills server mods from the library while lifecycle materialization is incomplete", () => {
    const installed = mod(["server_only_mod"]);
    const configured = { ...mod(["all_clients_require_mod"]), id: "library-2", instanceId: "unassigned", workshopId: "378160973" };

    expect(mergeConfiguredWorkshopMods([installed], [configured], ["376333686", "378160973"])).toEqual([
      installed,
      { ...configured, enabled: true }
    ]);
  });
});

describe("modRuntimeState", () => {
  it("does not claim client-only DST mods are pending a server restart", () => {
    expect(modRuntimeState(mod(["client_only_mod", "interface"]))).toBeNull();
  });

  it("describes DST server mods without runtime inspection as configured", () => {
    expect(modRuntimeState(mod(["all_clients_require_mod", "utility"]))).toBe("configured");
  });

  it("prefers runtime evidence when it is available", () => {
    expect(modRuntimeState({ ...mod(["all_clients_require_mod"]), runtimeEnabled: true })).toBe("enabled");
    expect(modRuntimeState({ ...mod(["all_clients_require_mod"]), runtimeEnabled: false })).toBe("notApplied");
  });

  it("keeps the pending state for providers that support runtime synchronization", () => {
    expect(modRuntimeState({ ...mod([]), providerKey: "terraria-tmodloader" })).toBe("pendingRestart");
  });

  it("describes a missing runtime package without implying cloud synchronization", () => {
    expect(modRuntimeState({ ...mod([]), providerKey: "terraria-tmodloader", runtimePresent: false })).toBe("runtimeFileMissing");
  });
});
