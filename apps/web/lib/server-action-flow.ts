export type ServerActionName = "start" | "stop" | "restart" | "delete";

export function serverActionRedirectPath(action: ServerActionName, pathname: string, serverId: string): string | null {
  if (action === "delete" && pathname === `/servers/${serverId}`) {
    return "/servers";
  }
  return null;
}
