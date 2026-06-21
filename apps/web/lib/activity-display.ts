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
  "server.start.container.prepare": { zh: "准备容器", en: "Preparing Container" },
  "server.start.container.created": { zh: "容器已创建", en: "Container Created" },
  "server.runtime.created": { zh: "运行实例已创建", en: "Runtime Created" },
  "server.runtime.removed": { zh: "运行实例已移除", en: "Runtime Removed" },
  "server.reconcile.failed": { zh: "同步失败", en: "Reconcile Failed" },
  "server.start.container.ready": { zh: "容器已就绪", en: "Container Ready" },
  "server.start.runtime.starting": { zh: "启动容器", en: "Starting Container" },
  "server.started": { zh: "服务器启动", en: "Server Started" },
  "server.start.failed": { zh: "启动失败", en: "Start Failed" },
  "server.stop.queued": { zh: "停止排队", en: "Stop Queued" },
  "server.stopped": { zh: "服务器停止", en: "Server Stopped" },
  "server.stop.failed": { zh: "停止失败", en: "Stop Failed" },
  "server.restart.queued": { zh: "重启排队", en: "Restart Queued" },
  "server.restart.container.prepare": { zh: "准备容器", en: "Preparing Container" },
  "server.restart.container.created": { zh: "容器已创建", en: "Container Created" },
  "server.restart.container.ready": { zh: "容器已就绪", en: "Container Ready" },
  "server.restart.runtime.starting": { zh: "启动容器", en: "Starting Container" },
  "server.restarted": { zh: "服务器重启", en: "Server Restarted" },
  "server.restart.failed": { zh: "重启失败", en: "Restart Failed" },
  "server.delete.queued": { zh: "删除排队", en: "Delete Queued" },
  "server.deleted": { zh: "服务器删除", en: "Server Deleted" },
  "server.config.updated": { zh: "配置更新", en: "Config Updated" },
  "settings.locale": { zh: "语言设置", en: "Language Settings" },
  "settings.publicHost": { zh: "设置更新", en: "Settings Updated" },
  "world.imported": { zh: "世界导入", en: "World Imported" },
  "world.snapshot.created": { zh: "世界快照", en: "World Snapshot" },
  "world.assigned": { zh: "世界切换", en: "World Assigned" },
  "world.deleted": { zh: "世界删除", en: "World Deleted" },
  "backup.created": { zh: "备份创建", en: "Backup Created" },
  "backup.restored": { zh: "备份恢复", en: "Backup Restored" },
  "backup.deleted": { zh: "备份删除", en: "Backup Deleted" },
  "save.snapshot.created": { zh: "存档快照", en: "Save Snapshot" },
  "save.snapshot.restored": { zh: "存档恢复", en: "Save Restored" },
  "mod.uploaded": { zh: "模组上传", en: "Mod Uploaded" },
  "mod.workshop_imported": { zh: "创意工坊导入", en: "Workshop Imported" },
  "mod.updated": { zh: "模组更新", en: "Mod Updated" },
  "mod.assigned": { zh: "模组分配", en: "Mod Assigned" },
  "mod.deleted": { zh: "模组删除", en: "Mod Deleted" },
  "player.kicked": { zh: "玩家踢出", en: "Player Kicked" },
  "player.banned": { zh: "玩家封禁", en: "Player Banned" },
  "player.whitelisted": { zh: "白名单更新", en: "Whitelist Updated" },
  "player.whitelist.removed": { zh: "白名单更新", en: "Whitelist Updated" }
};

const zhTemplates: Record<string, (message: string) => string | undefined> = {
  "server.created": (message) => withMatch(message, /^Created server (.+)$/, ([name]) => `已创建服务器 ${name}`),
  "server.start.queued": (message) => withMatch(message, /^Queued start for server (.+)$/, ([name]) => `已提交启动服务器 ${name}`),
  "server.start.container.prepare": (message) => withMatch(message, /^Preparing runtime container for server (.+)$/, ([name]) => `正在准备 ${name} 的运行容器`),
  "server.start.container.created": (message) => withMatch(message, /^Created runtime container for server (.+)$/, ([name]) => `已创建 ${name} 的运行容器`),
  "server.start.container.ready": (message) => withMatch(message, /^Runtime container ready for server (.+)$/, ([name]) => `${name} 的运行容器已就绪`),
  "server.start.runtime.starting": (message) => withMatch(message, /^Starting runtime container for server (.+)$/, ([name]) => `正在启动 ${name} 的运行容器`),
  "server.started": (message) => withMatch(message, /^Started server (.+)$/, ([name]) => `已启动服务器 ${name}`),
  "server.start.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 启动失败：${formatServerDetailError(new Error(reason))}`),
  "server.stop.queued": (message) => withMatch(message, /^Queued stop for server (.+)$/, ([name]) => `已提交停止服务器 ${name}`),
  "server.stopped": (message) => withMatch(message, /^Stopped server (.+)$/, ([name]) => `已停止服务器 ${name}`),
  "server.stop.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 停止失败：${formatServerDetailError(new Error(reason))}`),
  "server.restart.queued": (message) => withMatch(message, /^Queued restart for server (.+)$/, ([name]) => `已提交重启服务器 ${name}`),
  "server.restart.container.prepare": (message) => withMatch(message, /^Preparing runtime container for server (.+)$/, ([name]) => `正在准备 ${name} 的运行容器`),
  "server.restart.container.created": (message) => withMatch(message, /^Created runtime container for server (.+)$/, ([name]) => `已创建 ${name} 的运行容器`),
  "server.restart.container.ready": (message) => withMatch(message, /^Runtime container ready for server (.+)$/, ([name]) => `${name} 的运行容器已就绪`),
  "server.restart.runtime.starting": (message) => withMatch(message, /^Starting runtime container for server (.+)$/, ([name]) => `正在启动 ${name} 的运行容器`),
  "server.restarted": (message) => withMatch(message, /^Restarted server (.+)$/, ([name]) => `已重启服务器 ${name}`),
  "server.restart.failed": (message) => withMatch(message, /^(.+): (.+)$/, ([name, reason]) => `${name} 重启失败：${formatServerDetailError(new Error(reason))}`),
  "server.delete.queued": (message) => withMatch(message, /^Queued delete for server (.+)$/, ([name]) => `已提交删除服务器 ${name}`),
  "server.deleted": (message) => withMatch(message, /^Deleted server (.+)$/, ([name]) => `已删除服务器 ${name}`),
  "server.config.updated": (message) => withMatch(message, /^Updated config for (.+)$/, ([name]) => `已更新 ${name} 的配置`),
  "settings.locale": (message) => withMatch(message, /^Updated locale to "(.+)"$/, ([locale]) => `已将界面语言切换为${locale === "zh" ? "中文" : "英文"}`),
  "world.imported": (message) => withMatch(message, /^Imported world (.+)$/, ([name]) => `已导入世界 ${name}`),
  "world.snapshot.created": (message) => withMatch(message, /^Saved world snapshot (.+) from (.+)$/, ([world, server]) => `已从 ${server} 保存世界快照 ${world}`),
  "world.assigned": (message) => withMatch(message, /^Assigned world (.+) to (.+)$/, ([world, server]) => `已将世界 ${world} 设为 ${server} 的当前世界`),
  "world.deleted": (message) => withMatch(message, /^Deleted world (.+)$/, ([name]) => `已删除世界 ${name}`),
  "backup.created": (message) => withMatch(message, /^Created backup (.+) for (.+)$/, ([backup, server]) => `已为 ${server} 创建备份 ${backup}`),
  "backup.restored": (message) => withMatch(message, /^Restored backup (.+) for (.+)$/, ([backup, server]) => `已为 ${server} 恢复备份 ${backup}`),
  "backup.deleted": (message) => withMatch(message, /^Deleted backup (.+)$/, ([name]) => `已删除备份 ${name}`),
  "save.snapshot.created": (message) => withMatch(message, /^Created (.+) snapshot (.+) for (.+)$/, ([saveName, backup, server]) => `已为 ${server} 创建${saveName}快照 ${backup}`),
  "save.snapshot.restored": (message) => withMatch(message, /^Restored (.+) snapshot (.+) for (.+)$/, ([saveName, backup, server]) => `已为 ${server} 恢复${saveName}快照 ${backup}`),
  "mod.uploaded": (message) => withMatch(message, /^Uploaded mod (.+) to (.+)$/, ([mod, server]) => `已上传模组 ${mod} 到 ${server}`),
  "mod.workshop_imported": (message) =>
    withMatch(message, /^Imported (\d+) workshop mod IDs for (.+)$/, ([count, server]) => `已为 ${server} 导入 ${count} 个创意工坊模组`) ??
    withMatch(message, /^Imported (\d+) workshop mod IDs into mod library$/, ([count]) => `已导入 ${count} 个创意工坊模组到模组库`),
  "mod.updated": (message) => withMatch(message, /^Updated mod (.+)$/, ([name]) => `已更新模组 ${name}`),
  "mod.assigned": (message) =>
    withMatch(message, /^Assigned mod (.+) to (.+)$/, ([mod, server]) => `已分配模组 ${mod} 到 ${server}`) ??
    withMatch(message, /^Updated assigned mod (.+) for (.+)$/, ([mod, server]) => `已更新 ${server} 的模组 ${mod}`),
  "mod.deleted": (message) => withMatch(message, /^Deleted mod (.+)$/, ([name]) => `已删除模组 ${name}`),
  "player.kicked": (message) => withMatch(message, /^Kicked player (.+) from (.+)$/, ([player, server]) => `已将玩家 ${player} 从 ${server} 踢出`),
  "player.banned": (message) => withMatch(message, /^Banned player (.+) from (.+)$/, ([player, server]) => `已在 ${server} 封禁玩家 ${player}`),
  "player.whitelisted": (message) => withMatch(message, /^Added player (.+) to (.+) whitelist$/, ([player, server]) => `已将玩家 ${player} 加入 ${server} 白名单`),
  "player.whitelist.removed": (message) => withMatch(message, /^Removed player (.+) from (.+) whitelist$/, ([player, server]) => `已将玩家 ${player} 从 ${server} 白名单移除`)
};

export function formatActivityEvent(event: ActivityEvent, locale: Locale): ActivityDisplay {
  const typeLabel = labels[event.type]?.[locale] ?? event.type;
  const payloadMessage = formatActivityPayloadMessage(event, locale);
  if (payloadMessage) {
    return { message: payloadMessage, typeLabel };
  }
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

function formatActivityPayloadMessage(event: ActivityEvent, locale: Locale): string | undefined {
  const serverName = payloadString(event, "serverName");
  if (serverName) {
    const messages: Record<string, Record<Locale, string>> = {
      "server.created": { zh: `已创建服务器 ${serverName}`, en: `Created server ${serverName}` },
      "server.config.updated": { zh: `已更新 ${serverName} 的配置`, en: `Updated config for ${serverName}` },
      "server.start.queued": { zh: `已提交启动服务器 ${serverName}`, en: `Queued start for server ${serverName}` },
      "server.stop.queued": { zh: `已提交停止服务器 ${serverName}`, en: `Queued stop for server ${serverName}` },
      "server.restart.queued": { zh: `已提交重启服务器 ${serverName}`, en: `Queued restart for server ${serverName}` },
      "server.delete.queued": { zh: `已提交删除服务器 ${serverName}`, en: `Queued delete for server ${serverName}` },
      "server.share.enabled": { zh: `已启用 ${serverName} 的分享页面`, en: `Enabled share page for ${serverName}` },
      "server.share.disabled": { zh: `已关闭 ${serverName} 的分享页面`, en: `Disabled share page for ${serverName}` },
      "server.started": { zh: `已启动服务器 ${serverName}`, en: `Started server ${serverName}` },
      "server.stopped": { zh: `已停止服务器 ${serverName}`, en: `Stopped server ${serverName}` },
      "server.deleted": { zh: `已删除服务器 ${serverName}`, en: `Deleted server ${serverName}` },
      "server.runtime.created": { zh: `已创建 ${serverName} 的运行实例`, en: `Created runtime workload for server ${serverName}` },
      "server.runtime.removed": { zh: `已移除 ${serverName} 的运行实例`, en: `Removed runtime workload for server ${serverName}` },
      "server.reconcile.failed": {
        zh: `${serverName} 同步失败：${formatServerDetailError(new Error(payloadString(event, "lastError") ?? event.message))}`,
        en: `${serverName}: ${formatServerDetailError(new Error(payloadString(event, "lastError") ?? event.message), {
          dockerUnavailable: "Docker is not connected. Configure Docker Host in Settings first.",
          containerUnavailable: "The Docker container is unavailable or was removed outside the panel. Start the server again so GamePanel can recreate it."
        })}`
      }
    };
    const message = messages[event.type]?.[locale];
    if (message) return message;
  }
  const worldName = payloadString(event, "worldName");
  if (worldName) {
    const messages: Record<string, Record<Locale, string>> = {
      "world.imported": { zh: `已导入世界 ${worldName}`, en: `Imported world ${worldName}` },
      "world.snapshot.created": {
        zh: serverName ? `已从 ${serverName} 保存世界快照 ${worldName}` : `已保存世界快照 ${worldName}`,
        en: serverName ? `Saved world snapshot ${worldName} from ${serverName}` : `Saved world snapshot ${worldName}`
      },
      "world.assigned": {
        zh: serverName ? `已将世界 ${worldName} 设为 ${serverName} 的当前世界` : `已切换当前世界为 ${worldName}`,
        en: serverName ? `Assigned world ${worldName} to ${serverName}` : `Assigned world ${worldName}`
      },
      "world.deleted": { zh: `已删除世界 ${worldName}`, en: `Deleted world ${worldName}` }
    };
    const message = messages[event.type]?.[locale];
    if (message) return message;
  }
  const backupName = payloadString(event, "fileName");
  if (backupName) {
    const saveName = payloadString(event, "saveName") ?? (locale === "zh" ? "存档" : "save");
    const messages: Record<string, Record<Locale, string>> = {
      "backup.created": {
        zh: serverName ? `已为 ${serverName} 创建备份 ${backupName}` : `已创建备份 ${backupName}`,
        en: serverName ? `Created backup ${backupName} for ${serverName}` : `Created backup ${backupName}`
      },
      "backup.restored": {
        zh: serverName ? `已为 ${serverName} 恢复备份 ${backupName}` : `已恢复备份 ${backupName}`,
        en: serverName ? `Restored backup ${backupName} for ${serverName}` : `Restored backup ${backupName}`
      },
      "backup.deleted": { zh: `已删除备份 ${backupName}`, en: `Deleted backup ${backupName}` },
      "save.snapshot.created": {
        zh: serverName ? `已为 ${serverName} 创建${saveName}快照 ${backupName}` : `已创建${saveName}快照 ${backupName}`,
        en: serverName ? `Created ${saveName} snapshot ${backupName} for ${serverName}` : `Created ${saveName} snapshot ${backupName}`
      },
      "save.snapshot.restored": {
        zh: serverName ? `已为 ${serverName} 恢复${saveName}快照 ${backupName}` : `已恢复${saveName}快照 ${backupName}`,
        en: serverName ? `Restored ${saveName} snapshot ${backupName} for ${serverName}` : `Restored ${saveName} snapshot ${backupName}`
      }
    };
    const message = messages[event.type]?.[locale];
    if (message) return message;
  }
  const modName = payloadString(event, "title") || payloadString(event, "modName") || payloadString(event, "fileName") || payloadString(event, "workshopId");
  if (modName) {
    const messages: Record<string, Record<Locale, string>> = {
      "mod.uploaded": {
        zh: serverName ? `已上传模组 ${modName} 到 ${serverName}` : `已上传模组 ${modName}`,
        en: serverName ? `Uploaded mod ${modName} to ${serverName}` : `Uploaded mod ${modName}`
      },
      "mod.updated": { zh: `已更新模组 ${modName}`, en: `Updated mod ${modName}` },
      "mod.assigned": {
        zh: serverName ? `已分配模组 ${modName} 到 ${serverName}` : `已分配模组 ${modName}`,
        en: serverName ? `Assigned mod ${modName} to ${serverName}` : `Assigned mod ${modName}`
      },
      "mod.deleted": { zh: `已删除模组 ${modName}`, en: `Deleted mod ${modName}` }
    };
    const message = messages[event.type]?.[locale];
    if (message) return message;
  }
  if (event.type === "mod.workshop_imported") {
    const count = payloadNumber(event, "workshopCount");
    if (count !== undefined) {
      if (serverName) {
        return locale === "zh" ? `已为 ${serverName} 导入 ${count} 个创意工坊模组` : `Imported ${count} workshop mod IDs for ${serverName}`;
      }
      return locale === "zh" ? `已导入 ${count} 个创意工坊模组到模组库` : `Imported ${count} workshop mod IDs into mod library`;
    }
  }
  const playerName = payloadString(event, "playerName");
  if (playerName && serverName) {
    const messages: Record<string, Record<Locale, string>> = {
      "player.kicked": { zh: `已将玩家 ${playerName} 从 ${serverName} 踢出`, en: `Kicked player ${playerName} from ${serverName}` },
      "player.banned": { zh: `已在 ${serverName} 封禁玩家 ${playerName}`, en: `Banned player ${playerName} from ${serverName}` },
      "player.whitelisted": { zh: `已将玩家 ${playerName} 加入 ${serverName} 白名单`, en: `Added player ${playerName} to ${serverName} whitelist` },
      "player.whitelist.removed": { zh: `已将玩家 ${playerName} 从 ${serverName} 白名单移除`, en: `Removed player ${playerName} from ${serverName} whitelist` }
    };
    const message = messages[event.type]?.[locale];
    if (message) return message;
  }
  if (event.type === "settings.locale") {
    const localeValue = payloadString(event, "locale");
    if (localeValue) {
      return locale === "zh" ? `已将界面语言切换为${localeValue === "zh" ? "中文" : "英文"}` : `Updated locale to ${localeValue}`;
    }
  }
  if (event.type === "settings.publicHost") {
    const publicHost = payloadString(event, "publicHost");
    if (publicHost !== undefined) {
      return locale === "zh" ? `已更新公开访问地址为 ${publicHost || "默认地址"}` : `Updated public host to ${publicHost || "default host"}`;
    }
  }
  return undefined;
}

function payloadString(event: ActivityEvent, key: string): string | undefined {
  const value = event.payload?.[key];
  if (typeof value !== "string") return undefined;
  return value;
}

function payloadNumber(event: ActivityEvent, key: string): number | undefined {
  const value = event.payload?.[key];
  if (typeof value !== "number" || Number.isNaN(value)) return undefined;
  return value;
}

function withMatch(matchValue: string, pattern: RegExp, format: (groups: string[]) => string): string | undefined {
  const match = matchValue.match(pattern);
  if (!match) return undefined;
  return format(match.slice(1));
}
