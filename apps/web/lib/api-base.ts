const configuredApiBase = process.env.NEXT_PUBLIC_API_BASE_URL?.trim();

export function getApiBaseUrl() {
  if (configuredApiBase) {
    return configuredApiBase.replace(/\/$/, "");
  }
  return "";
}
