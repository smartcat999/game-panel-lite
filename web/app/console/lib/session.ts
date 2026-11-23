export const TOKEN_STORAGE_KEY = "gp_session_token";
export const USER_STORAGE_KEY = "gp_session_user";
export const SESSION_STORAGE_KEY = "gp_session_name";
export const TENANT_STORAGE_KEY = "gp_selected_tenant";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export function readToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_STORAGE_KEY);
}

export function parseTokenClaims(token: string | null): { sid?: string; sub?: string } {
  if (!token) return {};
  const parts = token.split(".");
  if (parts.length !== 3 || !parts[1]) return {};
  try {
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const bin = atob(padded);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) {
      bytes[i] = bin.charCodeAt(i);
    }
    const payload = JSON.parse(new TextDecoder().decode(bytes)) as Record<string, unknown>;
    const sid =
      typeof payload.sid === "string" && payload.sid
        ? payload.sid
        : typeof payload.jti === "string" && payload.jti
          ? payload.jti
          : undefined;
    const sub = typeof payload.sub === "string" && payload.sub ? payload.sub : undefined;
    return { sid, sub };
  } catch {
    return {};
  }
}

export function readSessionName(tokenOverride?: string | null): string {
  if (typeof window === "undefined") return "";
  const stored = (localStorage.getItem(SESSION_STORAGE_KEY) || "").trim();
  if (stored) return stored;
  const sid = parseTokenClaims(tokenOverride ?? readToken()).sid || "";
  if (sid) localStorage.setItem(SESSION_STORAGE_KEY, sid);
  return sid;
}

export function readUserName(tokenOverride?: string | null): string {
  if (typeof window === "undefined") return "";
  const stored = (localStorage.getItem(USER_STORAGE_KEY) || "").trim();
  if (stored) return stored;
  const sub = parseTokenClaims(tokenOverride ?? readToken()).sub || "";
  if (sub) localStorage.setItem(USER_STORAGE_KEY, sub);
  return sub;
}

export function saveSession(opts: { token: string; userName?: string; sessionName?: string }) {
  if (typeof window === "undefined") return;
  localStorage.setItem(TOKEN_STORAGE_KEY, opts.token);
  const claims = parseTokenClaims(opts.token);
  const resolvedUser = (opts.userName || claims.sub || "").trim();
  if (resolvedUser) localStorage.setItem(USER_STORAGE_KEY, resolvedUser);
  else localStorage.removeItem(USER_STORAGE_KEY);
  const resolvedSession = (opts.sessionName || claims.sid || "").trim();
  if (resolvedSession) localStorage.setItem(SESSION_STORAGE_KEY, resolvedSession);
  else localStorage.removeItem(SESSION_STORAGE_KEY);
}

export function clearSession() {
  localStorage.removeItem(TOKEN_STORAGE_KEY);
  localStorage.removeItem(USER_STORAGE_KEY);
  localStorage.removeItem(SESSION_STORAGE_KEY);
}

const ERROR_PATTERNS: Array<[RegExp, (m: RegExpMatchArray) => string]> = [
  [/user "([^"]+)" not found/i, (m) => `找不到平台账号「${m[1]}」，请确认对方已在平台注册且用户名拼写正确`],
  [/email "([^"]+)" is already registered to another user/i, (m) => `邮箱「${m[1]}」已被其他账号使用，请更换邮箱`],
  [/user spec\.email must be a valid email address/i, () => "请输入有效的邮箱地址"],
  [/tenant not found:\s*(.+)/i, (m) => `工作区「${m[1].trim()}」不存在`],
  [/tenant (\S+) is terminating/i, (m) => `工作区「${m[1]}」正在删除中，无法执行该操作`],
  [/tenant resource quota exceeded/i, () => "已超出当前工作区的资源配额上限"],
  [/tenant backup count quota exceeded/i, () => "已达到当前工作区的备份数量上限，请先删除旧备份"],
  [/tenant backup storage quota exceeded/i, () => "已超出当前工作区的备份存储空间配额"],
  [/another backup "([^"]+)" is already in progress/i, () => "当前已有另一个备份正在执行中，请等待其完成后再试"],
  [/cannot start backup while a restore is in progress/i, () => "当前正在恢复备份，请等待恢复完成后再创建新备份"],
  [/another restore is already in progress/i, () => "当前已有备份恢复正在执行中，请等待其完成后再试"],
  [/cannot restore while a backup archive is in progress/i, () => "当前正在打包备份，请等待备份完成后再执行恢复"],
  [/cannot delete backup while restore is in progress/i, () => "该备份正在恢复中，无法删除"],
  [/cannot delete built-in administrator user "([^"]+)"/i, (m) => `系统内置管理员「${m[1]}」不可删除`],
  [/cannot disable built-in administrator user "([^"]+)"/i, (m) => `系统内置管理员「${m[1]}」不可禁用`],
  [/cannot delete built-in globalrole "([^"]+)"/i, (m) => `系统内置全局角色「${m[1]}」不可删除`],
  [/cannot delete built-in globalrolebinding "([^"]+)"/i, (m) => `系统内置全局角色绑定「${m[1]}」不可删除`],
  [/cannot delete built-in tenantrole "([^"]+)"/i, (m) => `系统内置角色「${m[1]}」不可删除`],
  [/cannot delete built-in owner binding "([^"]+)"/i, () => "不能移除工作区所有者的管理员权限"],
  [/personal workspace "([^"]+)" does not support inviting members/i, (m) => `个人工作区「${m[1]}」不支持邀请其他成员，请切换到团队工作区后再试`],
  [/cannot delete personal workspace "([^"]+)" while owner user "([^"]+)" exists/i, (m) => `个人工作区「${m[1]}」与账号「${m[2]}」绑定，无法单独删除`],
  [/cannot delete built-in administrator tenant "([^"]+)"/i, (m) => `系统内置工作区「${m[1]}」不可删除`],
  [/cannot delete built-in runtimeclass "([^"]+)"/i, (m) => `系统内置运行环境「${m[1]}」不可删除`],
  [/still referenced by globalrolebinding "([^"]+)"/i, (m) => `该角色仍被绑定「${m[1]}」使用中，无法删除`],
  [/still referenced by tenantrolebinding "([^"]+)"/i, (m) => `该角色仍被成员绑定「${m[1]}」使用中，无法删除`],
  [/cannot delete tenant "([^"]+)" because it still contains (\d+) instance/i, (m) => `工作区「${m[1]}」下还有 ${m[2]} 台服务器，请先删除所有服务器`],
  [/cannot delete user "([^"]+)" because personal workspace "([^"]+)" still contains (\d+) instance/i, (m) => `用户「${m[1]}」的个人工作区下还有 ${m[3]} 台服务器，请先删除所有服务器`],
  [/cannot delete node "([^"]+)" because it still hosts (\d+) instance/i, (m) => `节点「${m[1]}」上仍托管着 ${m[2]} 台服务器，请先删除所有服务器`],
  [/runtimeclass "([^"]+)" is (?:still )?used by (\d+) instance/i, (m) => `运行环境「${m[1]}」仍被 ${m[2]} 台服务器使用中，无法删除`],
  [/runtimeclass "([^"]+)" is in use by (\d+) instance/i, (m) => `运行环境「${m[1]}」仍被 ${m[2]} 台服务器使用中，无法删除`],
  [/only platform administrators may modify user state/i, () => "只有平台管理员可以修改账号状态"],
  [/newPassword must be at least (\d+) characters/i, (m) => `密码长度至少为 ${m[1]} 位`],
  [/invalid user metadata\.name .*disallowed character/i, () => "用户名只能包含字母、数字和 . _ -"],
  [/username already taken/i, () => "该用户名已被注册，请换一个，或切换到「已有账号」登录"],
  [/already exists/i, () => "同名资源已存在，请换一个名称"],
  [/RBAC: access denied/i, () => "当前账号没有执行该操作的权限"],
  [/forbidden/i, () => "当前账号没有执行该操作的权限"],
  [/config field "([^"]+)" \(([^)]+)\) must be a valid number/i, (m) => `参数「${m[2]}」需填写有效数字`],
  [/config field "([^"]+)" \(([^)]+)\) is required/i, (m) => `请填写必填参数「${m[2]}」`],
  [/config schema validation failed:\s*(.+)/i, (m) => `游戏参数校验未通过：${m[1]}`],
  [/target path already exists/i, () => "同名文件或文件夹已存在，请换一个名称"],
  [/source file not found/i, () => "原文件或文件夹不存在，请刷新后重试"],
  [/^resource version conflict$/i, () => "服务器状态刚刚更新，请重试"],
  [/invalid credentials/i, () => "用户名或密码错误"],
  [/bad gateway:.*(?:api\.steampowered\.com|steamcommunity\.com)/i, () => "连接 Steam 创意工坊失败，请检查节点网络或代理配置后重试"],
  [/bad gateway:.*modrinth/i, () => "连接 Modrinth 模组/插件源失败，请稍后重试"],
  [/bad gateway:.*thunderstore/i, () => "连接 Thunderstore 模组源失败，请稍后重试"],
  [/bad gateway:.*umod/i, () => "连接 uMod 插件源失败，请稍后重试"],
  [/bad gateway:.*(?:upstream .* request failed|upstream status \d+|EOF|timeout|deadline exceeded|connection refused|tls)/i, () => "上游软件源连接中断或响应超时，请稍后重试"],
];

export function localizeApiError(raw: string, status?: number, fallback?: string): string {
  const trimmed = (raw || "").trim();
  for (const [re, fn] of ERROR_PATTERNS) {
    const m = trimmed.match(re);
    if (m) return fn(m);
  }
  if (status === 403) return "当前账号没有执行该操作的权限";
  const cleaned = trimmed
    .replace(/^resource not found:\s*/i, "")
    .replace(/^resource version conflict:\s*/i, "")
    .replace(/^bad request:\s*/i, "")
    .replace(/^bad gateway:\s*/i, "");
  return cleaned || (fallback ? `${fallback}${status ? ` (${status})` : ""}` : "操作失败");
}

/** Extracts a human readable message from an apiserver error body. */
export async function errorMessage(res: Response, fallback: string): Promise<string> {
  const body = await res.json().catch(() => null);
  const raw = (body && (body.message || body.error)) || "";
  return localizeApiError(raw, res.status, fallback);
}
