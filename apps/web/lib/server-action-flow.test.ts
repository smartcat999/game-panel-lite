import { describe, expect, it } from "vitest";
import { serverActionRedirectPath, type ServerActionName } from "./server-action-flow";

describe("server action flow", () => {
  it("keeps lifecycle actions on the current route so query updates do not shake the page", () => {
    const actions: ServerActionName[] = ["start", "stop", "restart"];

    expect(actions.map((action) => serverActionRedirectPath(action, "/servers/server-1", "server-1"))).toEqual([null, null, null]);
  });

  it("returns to the server list after deleting the current detail server", () => {
    expect(serverActionRedirectPath("delete", "/servers/server-1", "server-1")).toBe("/servers");
    expect(serverActionRedirectPath("delete", "/servers", "server-1")).toBeNull();
  });
});
