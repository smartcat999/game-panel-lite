import type { MessageKey } from "./i18n";

export type RuntimeErrorTranslator = (key: MessageKey, params?: Record<string, string | number>) => string;

export function formatRuntimeInstallError(error: unknown, t: RuntimeErrorTranslator) {
  const message = errorMessage(error);
  const key = classifyRuntimeError(message);
  return key ? t(key) : message;
}

export function formatCreateServerError(error: unknown, t: RuntimeErrorTranslator) {
  const message = errorMessage(error);
  const runtimeErrorKey = classifyRuntimeError(message);
  if (runtimeErrorKey) {
    return t(runtimeErrorKey);
  }

  const normalized = message.toLowerCase();
  if (normalized.includes("admin password is required")) return t("requiredFieldError", { field: t("adminPassword") });
  if (normalized.includes("server name is required")) return t("requiredFieldError", { field: t("serverName") });
  if (normalized.includes("save name is required")) return t("requiredFieldError", { field: t("saveName") });
  if (normalized.includes("world name is required")) return t("requiredFieldError", { field: t("worldName") });
  if (normalized.includes("cluster name is required")) return t("requiredFieldError", { field: t("clusterName") });
  if (normalized.includes("klei server token is required")) return t("requiredFieldError", { field: t("clusterToken") });
  if (normalized.includes("eula must be accepted")) return t("requiredAgreementError", { field: t("minecraftEulaAccepted") });
  return message;
}

export function classifyRuntimeError(message: string): MessageKey | undefined {
  const normalized = message.toLowerCase();
  if (!normalized) {
    return undefined;
  }
  if (
    normalized.includes("server runtime is not installed") ||
    normalized.includes("runtime image archive is missing") ||
    normalized.includes("runtime image archive is empty") ||
    normalized.includes("image archive path is empty")
  ) {
    return "runtimeInstallIncomplete";
  }
  if (
    normalized.includes("pull access denied") ||
    normalized.includes("repository does not exist") ||
    normalized.includes("requested access to the resource is denied") ||
    normalized.includes("manifest unknown") ||
    normalized.includes("not available from the registry") ||
    normalized.includes("no matching manifest")
  ) {
    return "runtimeImageUnavailable";
  }
  if (
    normalized.includes("supported only on amd64") ||
    normalized.includes("amd64 docker host") ||
    normalized.includes("qemu") && normalized.includes("steamcmd")
  ) {
    return "runtimeUnsupportedArchitecture";
  }
  if (
    normalized.includes("docker runtime unavailable") ||
    normalized.includes("cannot connect to docker") ||
    normalized.includes("docker status unreadable")
  ) {
    return "runtimeDockerUnavailable";
  }
  if (
    normalized.includes("port is already allocated") ||
    normalized.includes("address already in use") ||
    normalized.includes("external port") && normalized.includes("already used")
  ) {
    return "runtimePortAlreadyUsed";
  }
  return undefined;
}

function errorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  return String(error || "");
}
