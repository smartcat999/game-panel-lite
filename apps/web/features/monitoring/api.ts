import type {
  MonitoringEventsResponse,
  MonitoringMetricsResponse,
  MonitoringOverviewResponse,
  PlatformResponse,
  ServerLoadResponse
} from "./types";
import { getApiBaseUrl } from "@/lib/api-base";

const API_BASE = getApiBaseUrl();

async function apiFetch<T>(path: string, fallback: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { cache: "no-store", credentials: "include" });
  const payload = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? fallback);
  }
  return payload;
}

export function getMonitoringOverview() {
  return apiFetch<MonitoringOverviewResponse>("/api/monitoring/overview", "Unable to load monitoring overview");
}

export function getMonitoringMetrics(range = "15m", step = "30s") {
  return apiFetch<MonitoringMetricsResponse>(`/api/monitoring/metrics?range=${encodeURIComponent(range)}&step=${encodeURIComponent(step)}`, "Unable to load monitoring metrics");
}

export function getServerLoad() {
  return apiFetch<ServerLoadResponse>("/api/monitoring/server-load", "Unable to load server load");
}

export function getMonitoringEvents(params: { limit?: number; severity?: string; type?: string; game?: string } = {}) {
  const query = new URLSearchParams();
  query.set("limit", String(params.limit ?? 50));
  if (params.severity && params.severity !== "all") query.set("severity", params.severity);
  if (params.type && params.type !== "all") query.set("type", params.type);
  if (params.game && params.game !== "all") query.set("game", params.game);
  return apiFetch<MonitoringEventsResponse>(`/api/monitoring/events?${query.toString()}`, "Unable to load monitoring events");
}

export function getPlatformMonitoring(range = "15m", step = "30s") {
  return apiFetch<PlatformResponse>(`/api/monitoring/platform?range=${encodeURIComponent(range)}&step=${encodeURIComponent(step)}`, "Unable to load platform monitoring");
}

export function getServerMonitoringMetrics(serverId: string, range = "1h", step = "1m") {
  return apiFetch<MonitoringMetricsResponse>(`/api/servers/${serverId}/metrics?range=${encodeURIComponent(range)}&step=${encodeURIComponent(step)}`, "Unable to load server monitoring metrics");
}

export function getServerMonitoringEvents(serverId: string, limit = 50) {
  return apiFetch<MonitoringEventsResponse>(`/api/servers/${serverId}/events?limit=${limit}`, "Unable to load server monitoring events");
}
