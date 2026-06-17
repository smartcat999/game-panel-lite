import type { Server } from "./types";

type JoinPortInput = Pick<Server, "hostPort" | "port"> & Partial<Pick<Server, "joinInfo">>;
type JoinAddressInput = Partial<Pick<Server, "joinInfo">>;
type JoinPasswordInput = Pick<Server, "password"> & Partial<Pick<Server, "joinInfo">>;
type InviteInput = Pick<Server, "hostPort" | "name" | "password" | "port"> & Partial<Pick<Server, "joinInfo">>;

export function serverJoinPort(server: JoinPortInput): number {
  if (server.joinInfo?.port) return server.joinInfo.port;
  return server.hostPort > 0 ? server.hostPort : server.port;
}

export function serverJoinAddress(server: JoinAddressInput): string {
  return server.joinInfo?.address || "127.0.0.1";
}

export function serverJoinPassword(server: JoinPasswordInput): string {
  return server.joinInfo?.password ?? server.password;
}

export function serverInviteText(server: InviteInput): string {
  if (server.joinInfo?.inviteText) return server.joinInfo.inviteText;
  const password = server.password ? ` password: ${server.password}` : "";
  return `Join ${server.name} at 127.0.0.1:${serverJoinPort(server)}${password}`;
}
