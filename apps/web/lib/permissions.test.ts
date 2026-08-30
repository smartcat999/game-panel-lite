import { describe, expect, it } from "vitest";
import { permissionsForRole } from "./permissions";

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

  it("gives administrators the release and deletion permissions", () => {
    const permissions = permissionsForRole("admin");
    expect(permissions).toContain("server.delete");
    expect(permissions).toContain("system.manage");
  });
});
