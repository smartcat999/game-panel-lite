import type { TerrariaConfig } from "@gamepanel-lite/shared";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:4000";

export async function previewTerrariaConfig(config: TerrariaConfig): Promise<string> {
  const response = await fetch(`${API_BASE}/api/terraria/config/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ config })
  });
  const payload = (await response.json()) as { serverconfig?: string; error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to render Terraria config");
  }
  return payload.serverconfig ?? "";
}
