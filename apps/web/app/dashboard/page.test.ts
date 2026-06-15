import { describe, expect, it } from "vitest";
import { dashboardQuickActionHrefs } from "../../lib/dashboard-quick-actions";

describe("dashboard quick actions", () => {
  it("preserves action intent when navigating to resource pages", () => {
    expect(dashboardQuickActionHrefs).toEqual({
      createServer: "/servers/new",
      createBackup: "/backups?action=create"
    });
  });
});
