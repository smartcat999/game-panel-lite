import type { AppItem, InstanceItem } from "../../../console/lib/types";
import { addonBindingMeta, allAddonBindings } from "./shared";

export type InstanceBannerKind =
  | "suspended"
  | "artifact"
  | "quota"
  | "oom"
  | "unknown"
  | "timeout"
  | "crash"
  | "image"
  | "disk"
  | "runtime";

export interface InstanceAlertBanner {
  kind: InstanceBannerKind;
  isError: boolean;
  title: string;
  desc: string;
  detail?: string;
}

const CJK_RE = /[\u4e00-\u9fff]/;

function humanizeArtifactSyncError(
  rawDesc: string,
  inst: InstanceItem,
  matchedApp: AppItem | undefined,
  addonTabLabel: string,
): InstanceAlertBanner | null {
  if (!rawDesc.includes("failed in registry")) return null;
  const m = rawDesc.match(/artifact\s+([^.\s]+)\.([^.\s]+)\.([^\s:]+)\s+failed in registry/i);
  if (m) {
    const [, fRepo, fPkg, fVer] = m;
    const matchedAddon = inst.spec.addons?.find(
      (a) => a.repository === fRepo && a.package === fPkg,
    );
    const matchedBinding = allAddonBindings(matchedApp?.addonBinding).find((b) => b.repository === fRepo);
    if (matchedAddon || matchedBinding) {
      const displayTitle =
        matchedAddon?.title && matchedAddon.title !== fPkg ? `《${matchedAddon.title}》` : `（${fPkg}）`;
      const meta = addonBindingMeta(matchedBinding || matchedApp?.addonBinding);
      return {
        kind: "artifact",
        isError: true,
        title: `${meta.noun}${displayTitle}下载未完成，服务器暂未启动`,
        desc: `连接${meta.marketLabel}超时或网络受限，可前往「${addonTabLabel}」列表点击重新下载，或移除该${addonTabLabel}后重新启动服务器。`,
      };
    }
    return {
      kind: "artifact",
      isError: true,
      title: `游戏服务端版本（${fVer}）下载未完成，服务器暂未启动`,
      desc: "平台正在准备该版本的官方服务端文件，请稍后重新启动服务器或联系管理员检查版本下载状态。",
    };
  }
  return {
    kind: "artifact",
    isError: true,
    title: `${addonTabLabel}或游戏版本下载未完成，服务器暂未启动`,
    desc: `部分文件尚未完成下载，可前往「${addonTabLabel}」检查下载状态并重试。`,
  };
}

function humanizeFailedDescription(rawDesc: string): InstanceAlertBanner {
  if (/context deadline exceeded|i\/o timeout|timeout exceeded|deadline exceeded/i.test(rawDesc)) {
    return {
      kind: "timeout",
      isError: true,
      title: "服务端文件准备超时，本次启动未完成",
      desc: "首次拉取或解压大型游戏服务端耗时较长，节点会保留已下载的缓存文件。点击「重试启动」即可继续完成启动。",
    };
  }
  if (/Container is not running after execution|exitCode=/i.test(rawDesc)) {
    const exitMatch = rawDesc.match(/exitCode=(\d+)/i);
    return {
      kind: "crash",
      isError: true,
      title: "游戏服务端进程启动后异常退出",
      desc: "容器未能保持运行，通常由启动参数、配置文件或模组冲突导致。请打开「控制台」查看输出日志并调整后重试启动。",
      detail: exitMatch ? `容器退出码：${exitMatch[1]}` : undefined,
    };
  }
  if (/pull image|manifest unknown|pull access denied|tls handshake|no such host/i.test(rawDesc)) {
    return {
      kind: "image",
      isError: true,
      title: "运行环境镜像拉取失败，服务器暂未启动",
      desc: "计算节点拉取运行镜像超时或网络连接受限，请稍后点击「重试启动」或联系管理员检查节点镜像预热状态。",
    };
  }
  if (/no space left on device|disk quota exceeded/i.test(rawDesc)) {
    return {
      kind: "disk",
      isError: true,
      title: "实例存储空间不足，无法写入服务端文件",
      desc: "当前服务器磁盘空间已满，请前往「文件」清理无用文件或在「设置」中扩容存储配额后重试启动。",
    };
  }
  if (/address already in use|port is already allocated/i.test(rawDesc)) {
    return {
      kind: "runtime",
      isError: true,
      title: "网络端口被占用，服务器启动受阻",
      desc: "实例分配的宿主机端口暂时被占用，请稍后点击「重试启动」或联系管理员检查节点端口状态。",
    };
  }
  if (rawDesc && CJK_RE.test(rawDesc)) {
    return {
      kind: "runtime",
      isError: true,
      title: "服务器运行异常",
      desc: rawDesc,
    };
  }
  return {
    kind: "runtime",
    isError: true,
    title: "服务器运行异常",
    desc: "容器启动或运行编排遇到异常，请点击「重试启动」或打开「控制台」查看日志排查原因。",
    detail: rawDesc || undefined,
  };
}

export function resolveInstanceAlertBanner(opts: {
  inst: InstanceItem;
  matchedApp?: AppItem;
  addonTabLabel: string;
  phase: string;
  unschedulable: boolean;
  readyReason?: string;
  rawBannerDesc: string;
  tenantSuspended?: boolean;
}): InstanceAlertBanner | null {
  const { inst, matchedApp, addonTabLabel, phase, unschedulable, readyReason, rawBannerDesc, tenantSuspended } = opts;
  if (tenantSuspended && inst.spec.state === "Running") {
    return {
      kind: "suspended",
      isError: false,
      title: "工作区已暂停，服务器处于暂停运行状态",
      desc: "存档数据与网络端口完整保留；工作区恢复启用后服务器将自动恢复运行。",
    };
  }
  const activeIssue =
    unschedulable ||
    phase === "Failed" ||
    phase === "Unknown" ||
    readyReason === "OOMKilled" ||
    readyReason === "OOMRecovered";
  if (!activeIssue) return null;

  const artifactBanner = humanizeArtifactSyncError(rawBannerDesc, inst, matchedApp, addonTabLabel);
  if (artifactBanner) return artifactBanner;

  if (unschedulable) {
    return {
      kind: "quota",
      isError: true,
      title: "节点资源不足或暂无匹配的计算节点",
      desc:
        rawBannerDesc && CJK_RE.test(rawBannerDesc)
          ? rawBannerDesc
          : "当前暂无满足该配置的计算节点，请在「设置」中适当调小规格后重试。",
      detail: rawBannerDesc && !CJK_RE.test(rawBannerDesc) ? rawBannerDesc : undefined,
    };
  }
  if (readyReason === "OOMKilled") {
    return {
      kind: "oom",
      isError: true,
      title: "进程因内存不足异常退出",
      desc:
        rawBannerDesc && CJK_RE.test(rawBannerDesc)
          ? rawBannerDesc
          : "服务端进程超出内存上限被系统终止，请在「设置」中调大内存规格后重新启动。",
    };
  }
  if (readyReason === "OOMRecovered") {
    return {
      kind: "oom",
      isError: false,
      title: "曾发生内存不足，已自动重启恢复",
      desc:
        rawBannerDesc && CJK_RE.test(rawBannerDesc)
          ? rawBannerDesc
          : "建议在「设置」中适当提高内存规格，避免高峰期再次触发自动重启。",
    };
  }
  if (phase === "Unknown") {
    return {
      kind: "unknown",
      isError: false,
      title: "所在计算节点暂时失联，状态未知",
      desc: "宿主机心跳暂未恢复，运行中的游戏进程通常不受影响，节点重连后将自动恢复状态显示。",
    };
  }
  return humanizeFailedDescription(rawBannerDesc);
}
