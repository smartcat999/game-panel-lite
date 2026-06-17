import { describe, expect, it } from "vitest";
import { serverInviteText, serverJoinAddress, serverJoinPassword, serverJoinPort } from "./server-join";

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

  it("uses provider join info when the API returns game-specific invite details", () => {
    const server = {
      hostPort: 30001,
      name: "Pal Friends",
      password: "fallback",
      port: 8211,
      joinInfo: {
        address: "10.0.0.5",
        port: 18211,
        password: "pal-secret",
        inviteText: "Join Pal Friends in Palworld at 10.0.0.5:18211 password: pal-secret"
      }
    };

    expect(serverJoinAddress(server)).toBe("10.0.0.5");
    expect(serverJoinPort(server)).toBe(18211);
    expect(serverJoinPassword(server)).toBe("pal-secret");
    expect(serverInviteText(server)).toBe("Join Pal Friends in Palworld at 10.0.0.5:18211 password: pal-secret");
  });
});
