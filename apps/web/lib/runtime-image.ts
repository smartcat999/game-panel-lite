import type { MessageKey } from "./i18n";
import type { RuntimeImageStatus } from "./types";

export function isRuntimeImageReady(status?: RuntimeImageStatus) {
  return status?.status === "ready";
}

export function isRuntimeImagePreparing(status?: RuntimeImageStatus) {
  return status?.status === "preparing";
}

export function runtimeImageLabelKey(status?: RuntimeImageStatus): MessageKey {
  switch (status?.status) {
    case "ready":
      return "gameLibraryInstalled";
    case "preparing":
      return "gameLibraryInstalling";
    case "failed":
      return "gameLibraryInstallFailed";
    case "unsupported":
      return "gameLibraryUnsupported";
    default:
      return "gameLibraryNotInstalled";
  }
}

export function runtimeImageTone(status?: RuntimeImageStatus) {
  switch (status?.status) {
    case "ready":
      return "success";
    case "preparing":
      return "info";
    case "failed":
    case "unsupported":
      return "warning";
    default:
      return "neutral";
  }
}
