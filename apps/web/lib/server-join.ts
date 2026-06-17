import type { Server } from "./types";

export function serverJoinPort(server: Pick<Server, "hostPort" | "port">): number {
  return server.hostPort > 0 ? server.hostPort : server.port;
}

export function serverInviteText(server: Pick<Server, "hostPort" | "name" | "password" | "port">): string {
  const password = server.password ? ` password: ${server.password}` : "";
  return `Join ${server.name} at 127.0.0.1:${serverJoinPort(server)}${password}`;
}
