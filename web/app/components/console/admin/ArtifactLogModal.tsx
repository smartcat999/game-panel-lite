"use client";

import { useMemo } from "react";
import type { RegistryArtifact } from "../../../console/lib/types";
import { TaskLogModal, formatTaskByteValue } from "../shared/TaskLogModal";
import { formatAddonVersion } from "./RegistryVersionModal";

function stageLabel(artifact: RegistryArtifact | null): string {
  const stage = artifact?.status?.stage;
  switch (stage) {
    case "Queued":
      return "排队等待中";
    case "Resolving":
      return "解析上游资源";
    case "Downloading":
      return "正在下载";
    case "Archiving":
      return "校验打包中";
    case "Uploading":
      return "上传至镜像存储";
    case "Completed":
      return "已完成";
    case "Failed":
      return "同步失败";
    default:
      break;
  }
  const phase = artifact?.status?.phase;
  if (phase === "Ready") return "已完成";
  if (phase === "Failed") return "同步失败";
  if (phase === "Pending") return "排队等待中";
  if (phase === "Syncing") return "同步中";
  return "等待状态";
}

function stageTone(
  artifact: RegistryArtifact | null,
): "success" | "warning" | "danger" | "accent" | "neutral" {
  const phase = artifact?.status?.phase;
  if (phase === "Ready" || artifact?.status?.stage === "Completed") return "success";
  if (phase === "Failed" || artifact?.status?.stage === "Failed") return "danger";
  if (phase === "Syncing" || phase === "Pending") return "accent";
  return "neutral";
}

export function ArtifactLogModal({
  open,
  onClose,
  artifact,
  title,
  token,
  backLabel = "关闭",
}: {
  open: boolean;
  onClose: () => void;
  artifact: RegistryArtifact | null;
  title?: string;
  token?: string | null;
  backLabel?: string;
}) {
  const artifactName = artifact?.metadata.name ?? "";
  const isActive =
    artifact?.status?.phase === "Pending" || artifact?.status?.phase === "Syncing";
  const progress = artifact?.status?.progress ?? 0;
  const processedBytes = artifact?.status?.processedBytes ?? 0;
  const totalBytes = artifact?.status?.totalBytes ?? 0;
  const speedBytesPerSec = artifact?.status?.speedBytesPerSec ?? 0;
  const isIndeterminate =
    isActive &&
    (artifact?.status?.stage === "Queued" ||
      artifact?.status?.stage === "Resolving" ||
      artifact?.status?.stage === "Archiving" ||
      (progress <= 0 && totalBytes <= 0));

  const versionLabel = artifact ? formatAddonVersion(artifact.spec.version) : "";
  const description = artifact
    ? artifact.status?.build
      ? `版本 ${versionLabel} · Build ${artifact.status.build}`
      : `版本 ${versionLabel}`
    : undefined;

  const byteSummary = useMemo(() => {
    if (isActive) {
      if (totalBytes > 0) {
        return `${formatTaskByteValue(processedBytes)} / ${formatTaskByteValue(totalBytes)}`;
      }
      if (processedBytes > 0) {
        return `已处理 ${formatTaskByteValue(processedBytes)}`;
      }
      return "";
    }
    const finalSize =
      artifact?.status?.descriptor?.sizeBytes ??
      artifact?.spec.descriptor?.sizeBytes ??
      (totalBytes > 0 ? totalBytes : processedBytes);
    if (finalSize && finalSize > 0) {
      return formatTaskByteValue(finalSize);
    }
    return "";
  }, [
    isActive,
    processedBytes,
    totalBytes,
    artifact?.status?.descriptor?.sizeBytes,
    artifact?.spec.descriptor?.sizeBytes,
  ]);

  const statusMessage =
    !isActive && artifact?.status?.phase === "Ready"
      ? artifact?.status?.message?.trim() || undefined
      : artifact?.status?.message;

  return (
    <TaskLogModal
      open={open}
      onClose={onClose}
      wsPath={
        artifactName
          ? `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(artifactName)}/log`
          : ""
      }
      token={token}
      title={title ? `${title} · 日志` : "同步日志"}
      description={description}
      badgeTone={stageTone(artifact)}
      badgeLabel={stageLabel(artifact)}
      message={statusMessage}
      progress={progress}
      isIndeterminate={isIndeterminate}
      isActive={isActive}
      byteSummary={byteSummary}
      speedBytesPerSec={speedBytesPerSec}
      backLabel={backLabel}
    />
  );
}
