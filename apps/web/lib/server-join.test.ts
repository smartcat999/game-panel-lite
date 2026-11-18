import { describe, expect, it } from "vitest";
import { gameServerMaxPlayers } from "./game-server-resource";
import { serverInviteText, serverJoinAddress, serverJoinPassword, serverJoinPort } from "./server-join";

describe("server join helpers", () => {
  it("reads join details directly from GameServer resources", () => {
    const server = {
      id: "server-1",
      name: "Resource Server",
      gameKey: "terraria",
      providerKey: "terraria-vanilla",
      spec: {
        generation: 1,
        desiredState: "running",
        config: { password: "secret", port: 7777 },
        network: { port: 7777, hostPort: 30001 }
      },
      status: {
        phase: "running",
        actualState: "running",
        observedGeneration: 1,
        appliedGeneration: 1
      },
      createdAt: "2026-06-21T00:00:00Z",
      updatedAt: "2026-06-21T00:00:00Z"
    } as const;

    expect(serverJoinAddress(server)).toBe("127.0.0.1");
    expect(serverJoinPort(server)).toBe(30001);
    expect(serverJoinPassword(server)).toBe("secret");
    expect(serverInviteText(server)).toBe("Join Resource Server at 127.0.0.1:30001 password: secret");
  });
});


it("uses PZ server.Password for joining and never the initial administrator password", () => {
  const server = {
    id: "pz-1", name: "PZ room", gameKey: "project-zomboid", providerKey: "project-zomboid",
    spec: { generation: 1, desiredState: "running", config: { adminPassword: "private-admin", server: { PublicName: "PZ room", Password: "join-only", MaxPlayers: 24 } }, network: { port: 16261, hostPort: 30002 } },
    status: { phase: "running", actualState: "running", observedGeneration: 1, appliedGeneration: 1 },
    createdAt: "2026-10-28T00:00:00Z", updatedAt: "2026-10-28T00:00:00Z"
  } as const;
  expect(serverJoinPassword(server)).toBe("join-only");
  expect(serverJoinPort(server)).toBe(30002);
  expect(gameServerMaxPlayers(server)).toBe(24);
  expect(serverInviteText(server)).not.toContain("private-admin");
  expect(serverJoinPassword({ ...server, spec: { ...server.spec, config: { adminPassword: "private-admin", server: { Password: "" } } } })).toBe("");
});
