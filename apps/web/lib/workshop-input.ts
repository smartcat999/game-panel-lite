export function parseWorkshopIds(value: string) {
  const seen = new Set<string>();
  const ids: string[] = [];

  for (const token of value.split(/[\s,，]+/)) {
    const id = workshopIdFromToken(token);
    if (!id || seen.has(id)) continue;
    seen.add(id);
    ids.push(id);
  }

  return ids;
}

function workshopIdFromToken(token: string) {
  const value = token.trim();
  if (/^\d{5,20}$/.test(value)) return value;

  try {
    const url = new URL(value);
    if (url.protocol !== "https:" || !["steamcommunity.com", "www.steamcommunity.com"].includes(url.hostname.toLowerCase())) {
      return "";
    }
    if (url.pathname !== "/sharedfiles/filedetails/") return "";
    const id = url.searchParams.get("id") ?? "";
    return /^\d{5,20}$/.test(id) ? id : "";
  } catch {
    return "";
  }
}
