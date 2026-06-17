import { describe, expect, it } from "vitest";
import { serverInviteText, serverJoinPort } from "./server-join";

describe("server join helpers", () => {
  it("uses the host port when Docker maps the server to an external port", () => {
    expect(serverJoinPort({ hostPort: 30001, port: 7777 })).toBe(30001);
  });

  it("falls back to the configured container port before a runtime port is assigned", () => {
    expect(serverJoinPort({ hostPort: 0, port: 7777 })).toBe(7777);
  });

  it("copies invite text with the external join port", () => {
    expect(serverInviteText({ hostPort: 30001, name: "Friends", password: "secret", port: 7777 })).toBe(
      "Join Friends at 127.0.0.1:30001 password: secret"
    );
  });
});
