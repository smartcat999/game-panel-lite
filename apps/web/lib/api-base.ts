const configuredApiBase = process.env.NEXT_PUBLIC_API_BASE_URL?.trim();

export function getApiBaseUrl() {
  // In the browser, if the user visits via a remote IP/host (e.g. 192.168.x.x, domain)
  // while configuredApiBase is empty or points to localhost, use relative path ("")
  // so requests go through the current origin (Nginx reverse proxy) instead of failing.
  if (typeof window !== "undefined") {
    const currentHost = window.location.hostname;
    if (currentHost !== "localhost" && currentHost !== "127.0.0.1") {
      if (!configuredApiBase || configuredApiBase.includes("localhost") || configuredApiBase.includes("127.0.0.1")) {
        return "";
      }
    }
  }
  if (configuredApiBase) {
    return configuredApiBase.replace(/\/$/, "");
  }
  return "";
}
