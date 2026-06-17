import type { ActivityEvent } from "./types";
import { formatServerDetailError } from "./server-detail-actions";

type Locale = "zh" | "en";

type ActivityDisplay = {
  message: string;
  typeLabel: string;
};

const labels: Record<string, Record<Locale, string>> = {
  "server.created": { zh: "服务器创建", en: "Server Created" },
  "server.start.queued": { zh: "启动排队", en: "Start Queued" },
  "server.started": { zh: "服务器启动", en: "Server Started" },
  "server.start.failed": { zh: "启动失败", en: "Start Failed" },
  "server.stop.queued": { zh: "停止排队", en: "Stop Queued" },
  "server.stopped": { zh: "服务器停止", en: "Server Stopped" },
  "server.stop.failed": { zh: "停止失败", en: "Stop Failed" },
  "server.restart.queued": { zh: "重启排队", en: "Restart Queued" },
  "server.restarted": { zh: "服务器重启", en: "Server Restarted" },
  "server.restart.failed": { zh: "重启失败", en: "Restart Failed" },
  "server.delete.queued": { zh: "删除排队", en: "Delete Queued" },
  "server.deleted": { zh: "服务器删除", en: "Server Deleted" },
  "server.config.updated": { zh: "配置更新", en: "Config Updated" },
  "world.imported": { zh: "世界导入", en: "World Imported" },
  "world.snapshot.created": { zh: "世界快照", en: "World Snapshot" },
  "world.assigned": { zh: "世界切换", en: "World Assigned" },
  "world.deleted": { zh: "世界删除", en: "World Deleted" },
  "backup.created": { zh: "备份创建", en: "Backup Created" },
  "backup.restored": { zh: "备份恢复", en: "Backup Restored" },
  "backup.deleted": { zh: "备份删除", en: "Backup Deleted" },
  "mod.uploaded": { zh: "模组上传", en: "Mod Uploaded" },
  "mod.updated": { zh: "模组更新", en: "Mod Updated" },
  "mod.assigned": { zh: "模组分配", en: "Mod Assigned" },
  "mod.deleted": { zh: "模组删除", en: "Mod Deleted" }
};

const zhTemplates: Record<string, (message: string) => string | undefined> = {
  "server.created": (message) => withMatch(message, /^Created server (.+)$/, ([name]) => `已创建服务器 ${name}`),
  "server.start.queued": (message) => withMatch(message, /^Queued start for server (.+)$/, ([name]) => `已提交启动服务器 ${name}`),
  "server.started": (message) => withMatch(message, /^Started server (.+)$/, ([name]) => `已启动服务器 ${name}`),
  "server.start.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 启动失败：${formatServerDetailError(new Error(reason))}`),
  "server.stop.queued": (message) => withMatch(message, /^Queued stop for server (.+)$/, ([name]) => `已提交停止服务器 ${name}`),
  "server.stopped": (message) => withMatch(message, /^Stopped server (.+)$/, ([name]) => `已停止服务器 ${name}`),
  "server.stop.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 停止失败：${formatServerDetailError(new Error(reason))}`),
  "server.restart.queued": (message) => withMatch(message, /^Queued restart for server (.+)$/, ([name]) => `已提交重启服务器 ${name}`),
  "server.restarted": (message) => withMatch(message, /^Restarted server (.+)$/, ([name]) => `已重启服务器 ${name}`),
  "server.restart.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 重启失败：${formatServerDetailError(new Error(reason))}`),
  "server.delete.queued": (message) => withMatch(message, /^Queued delete for server (.+)$/, ([name]) => `已提交删除服务器 ${name}`),
  "server.deleted": (message) => withMatch(message, /^Deleted server (.+)$/, ([name]) => `已删除服务器 ${name}`),
  "server.config.updated": (message) => withMatch(message, /^Updated config for (.+)$/, ([name]) => `已更新 ${name} 的配置`),
  "world.imported": (message) => withMatch(message, /^Imported world (.+)$/, ([name]) => `已导入世界 ${name}`),
  "world.snapshot.created": (message) => withMatch(message, /^Saved world snapshot (.+) from (.+)$/, ([world, server]) => `已从 ${server} 保存世界快照 ${world}`),
  "world.assigned": (message) => withMatch(message, /^Assigned world (.+) to (.+)$/, ([world, server]) => `已将世界 ${world} 设为 ${server} 的当前世界`),
  "world.deleted": (message) => withMatch(message, /^Deleted world (.+)$/, ([name]) => `已删除世界 ${name}`),
  "backup.created": (message) => withMatch(message, /^Created backup (.+) for (.+)$/, ([backup, server]) => `已为 ${server} 创建备份 ${backup}`),
  "backup.restored": (message) => withMatch(message, /^Restored backup (.+) for (.+)$/, ([backup, server]) => `已为 ${server} 恢复备份 ${backup}`),
  "backup.deleted": (message) => withMatch(message, /^Deleted backup (.+)$/, ([name]) => `已删除备份 ${name}`),
  "mod.uploaded": (message) => withMatch(message, /^Uploaded mod (.+) to (.+)$/, ([mod, server]) => `已上传模组 ${mod} 到 ${server}`),
  "mod.updated": (message) => withMatch(message, /^Updated mod (.+)$/, ([name]) => `已更新模组 ${name}`),
  "mod.assigned": (message) =>
    withMatch(message, /^Assigned mod (.+) to (.+)$/, ([mod, server]) => `已分配模组 ${mod} 到 ${server}`) ??
    withMatch(message, /^Updated assigned mod (.+) for (.+)$/, ([mod, server]) => `已更新 ${server} 的模组 ${mod}`),
  "mod.deleted": (message) => withMatch(message, /^Deleted mod (.+)$/, ([name]) => `已删除模组 ${name}`)
};

export function formatActivityEvent(event: ActivityEvent, locale: Locale): ActivityDisplay {
  const typeLabel = labels[event.type]?.[locale] ?? event.type;
  if (locale === "en") {
    if (event.type === "server.start.failed" || event.type === "server.stop.failed" || event.type === "server.restart.failed") {
      return {
        message: withMatch(event.message, /^(.+): (.+)$/, ([name, reason]) => `${name}: ${formatServerDetailError(new Error(reason), {
          dockerUnavailable: "Docker is not connected. Configure Docker Host in Settings first.",
          containerUnavailable: "The Docker container is unavailable or was removed outside the panel. Start the server again so GamePanel can recreate it."
        })}`) ?? event.message,
        typeLabel
      };
    }
    return { message: event.message, typeLabel };
  }
  return {
    message: zhTemplates[event.type]?.(event.message) ?? event.message,
    typeLabel
  };
}

function withMatch(matchValue: string, pattern: RegExp, format: (groups: string[]) => string): string | undefined {
  const match = matchValue.match(pattern);
  if (!match) return undefined;
  return format(match.slice(1));
}
