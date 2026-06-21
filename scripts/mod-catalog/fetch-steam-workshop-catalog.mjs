#!/usr/bin/env node
/* global console, fetch, process, URL */
/* eslint-disable no-useless-escape */

import { writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

const DEFAULT_OUTPUT = resolve(__dirname, "../../apps/api/internal/modcatalog/recommended_dst_mods.json");
const DST_APP_ID = "322330";

function argValue(name, fallback) {
  const index = process.argv.indexOf(name);
  if (index >= 0 && process.argv[index + 1]) return process.argv[index + 1];
  return fallback;
}

const appid = argValue("--appid", DST_APP_ID);
const output = resolve(process.cwd(), argValue("--output", DEFAULT_OUTPUT));
const limit = Number.parseInt(argValue("--limit", "60"), 10);
const pages = Number.parseInt(argValue("--pages", "3"), 10);

async function main() {
  const items = new Map();
  for (let page = 1; page <= pages && items.size < limit; page += 1) {
    const browseItems = await fetchWorkshopBrowse(appid, page);
    for (const item of browseItems) {
      if (!items.has(item.workshopId)) items.set(item.workshopId, item);
      if (items.size >= limit) break;
    }
  }
  const payload = [...items.values()].slice(0, limit).map((item, index) => ({
    rank: index + 1,
    ...item
  }));
  await writeFile(output, `${JSON.stringify(payload, null, 2)}\n`);
  console.log(`wrote ${payload.length} workshop items to ${output}`);
}

async function fetchWorkshopBrowse(appid, page) {
  const url = new URL("https://steamcommunity.com/workshop/browse/");
  url.searchParams.set("appid", appid);
  url.searchParams.set("browsesort", "totaluniquesubscribers");
  url.searchParams.set("section", "readytouseitems");
  url.searchParams.set("p", String(page));

  const html = await fetchText(url);
  try {
    const context = parseSSRContext(html);
    const queryData = JSON.parse(context.queryData);
    const browse = queryData.queries.find((query) => Array.isArray(query.queryKey) && query.queryKey[0] === "workshop_browse");
    const results = browse?.state?.data?.results ?? [];
    return results.map((item) => fromBrowseItem(item)).filter(Boolean);
  } catch {
    return fromEscapedBrowseItems(html);
  }
}

function fromBrowseItem(item) {
  if (!item?.publishedfileid || !item?.title) return null;
  return compactItem({
    workshopId: String(item.publishedfileid),
    title: item.title,
    creatorSteamId: item.creator ? String(item.creator) : "",
    previewUrl: item.preview_url,
    fileSize: numberValue(item.file_size),
    subscriptions: numberValue(item.subscriptions),
    favorited: numberValue(item.favorited),
    views: numberValue(item.views),
    timeCreated: numberValue(item.time_created),
    timeUpdated: numberValue(item.time_updated),
    tags: (item.tags ?? []).map((tag) => tag.display_name || tag.tag).filter(Boolean),
    description: truncateDescription(item.short_description ?? "")
  });
}

function fromEscapedBrowseItems(html) {
  const items = [];
  const pattern = /\\\"publishedfileid\\\":\\\"(?<workshopId>\d+)\\\"(?<chunk>[\s\S]*?)\\\"total_votes\\\":(?<totalVotes>\d+)/g;
  for (const match of html.matchAll(pattern)) {
    const chunk = match.groups?.chunk ?? "";
    const item = compactItem({
      workshopId: match.groups?.workshopId,
      title: escapedField(chunk, "title"),
      creatorSteamId: escapedField(chunk, "creator"),
      previewUrl: escapedField(chunk, "preview_url"),
      fileSize: numberValue(escapedField(chunk, "file_size")),
      subscriptions: numberValue(numberField(chunk, "subscriptions")),
      favorited: numberValue(numberField(chunk, "favorited")),
      views: numberValue(numberField(chunk, "views")),
      timeCreated: numberValue(numberField(chunk, "time_created")),
      timeUpdated: numberValue(numberField(chunk, "time_updated")),
      tags: [...chunk.matchAll(/\\\"display_name\\\":\\\"(.*?)\\\"/g)].map((tag) => unescapeSteamString(tag[1])).filter(Boolean),
      description: truncateDescription(escapedField(chunk, "short_description"))
    });
    if (item.workshopId && item.title) items.push(item);
  }
  return items;
}

function escapedField(chunk, field) {
  const match = chunk.match(new RegExp(`\\\\\\\\\\\"${field}\\\\\\\\\\\":\\\\\\\\\\\"([\\\\s\\\\S]*?)(?=\\\\\\\\\\\",\\\\\\\\\\\"|\\\\\\\\\\\"})`));
  return match ? unescapeSteamString(match[1]) : "";
}

function numberField(chunk, field) {
  const match = chunk.match(new RegExp(`\\\\\\\\\\\"${field}\\\\\\\\\\\":(\\\\d+)`));
  return match ? match[1] : "";
}

function unescapeSteamString(value) {
  return value
    .replace(/\\\\n/g, " ")
    .replace(/\\\\\\\"/g, "\"")
    .replace(/\\\\\//g, "/")
    .replace(/\\\\u([0-9a-fA-F]{4})/g, (_, hex) => String.fromCharCode(Number.parseInt(hex, 16)))
    .replace(/\\\\/g, "\\")
    .trim();
}

function compactItem(item) {
  const output = {};
  for (const [key, value] of Object.entries(item)) {
    if (value === "" || value === undefined || value === null) continue;
    if (Array.isArray(value) && value.length === 0) continue;
    output[key] = value;
  }
  return output;
}

function parseSSRContext(html) {
  const marker = "window.SSR.renderContext=JSON.parse(";
  const start = html.indexOf(marker);
  if (start < 0) throw new Error("Steam Workshop SSR context was not found");
  const valueStart = start + marker.length;
  const valueEnd = html.indexOf(");", valueStart);
  if (valueEnd < 0) throw new Error("Steam Workshop SSR context was incomplete");
  const literal = html.slice(valueStart, valueEnd).replace(/\r?\n/g, "\\n");
  return JSON.parse(Function(`return ${literal}`)());
}

function htmlToPlainText(value) {
  return htmlDecode(value)
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function htmlDecode(value) {
  return value
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, "\"")
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">");
}

function truncateDescription(value) {
  return htmlToPlainText(value).slice(0, 360);
}

function numberValue(value) {
  if (value === undefined || value === null || value === "") return 0;
  const parsed = Number.parseInt(String(value).replace(/,/g, ""), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

async function fetchText(url) {
  const response = await fetch(url, {
    headers: {
      "accept-language": "en-US,en;q=0.9",
      "user-agent": "GamePanelLiteCatalogBot/1.0"
    }
  });
  if (!response.ok) throw new Error(`Steam request failed: ${response.status} ${url}`);
  return response.text();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
