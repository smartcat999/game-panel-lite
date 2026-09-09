import { describe, expect, it } from "vitest";
import { permissionsForPlatformRole, permissionsForRole } from "./permissions";

describe("permissionsForRole", () => {
  it("keeps viewers read only", () => {
    expect(permissionsForRole("viewer")).toEqual(["server.view"]);
  });

  it("lets members operate servers without destructive or system access", () => {
    const permissions = permissionsForRole("member");
    expect(permissions).toContain("server.control");
    expect(permissions).toContain("mod.manage");
    expect(permissions).not.toContain("server.delete");
    expect(permissions).not.toContain("settings.manage");
  });

  it("gives tenant administrators resource permissions without infrastructure access", () => {
    const permissions = permissionsForRole("admin");
    expect(permissions).toContain("server.delete");
    expect(permissions).toContain("settings.manage");
    expect(permissions).not.toContain("node.manage");
    expect(permissions).not.toContain("system.manage");
  });

  it("keeps infrastructure permissions in the platform role", () => {
    expect(permissionsForPlatformRole("user")).toEqual([]);
    expect(permissionsForPlatformRole("platform_admin")).toContain("node.manage");
    expect(permissionsForPlatformRole("platform_admin")).toContain("system.manage");
  });
});
