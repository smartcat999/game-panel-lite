const configuredApiBase = process.env.NEXT_PUBLIC_API_BASE_URL?.trim();

export function getApiBaseUrl() {
  if (configuredApiBase) {
    return configuredApiBase.replace(/\/$/, "");
  }
  if (typeof window !== "undefined") {
    return `${window.location.protocol}//${window.location.hostname}:4000`;
  }
  return "http://localhost:4000";
}
