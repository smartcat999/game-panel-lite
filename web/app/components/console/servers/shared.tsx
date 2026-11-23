"use client";

import Link from "next/link";
import { ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { ConfirmDialog, focusRing, MoreMenu, type MenuItem } from "../ui";
import { useConsole } from "../../../console/ConsoleProvider";
import { DEFAULT_GAME_COVER, gameCover, resolveRichApp } from "../../../console/lib/presets";
import type {
  AppAddonBinding,
  AppConfigOption,
  AppItem,
  InstanceAddon,
  InstanceItem,
  RegistryArtifact,
} from "../../../console/lib/types";
import { subscribeResourceWatch } from "../../../console/lib/ws";

export interface AddonBindingMeta {
  isSteam: boolean;
  isPlugin: boolean;
  noun: "插件" | "模组" | "地图";
  idLabel: string;
  marketLabel: string;
  browseActionLabel: string;
}

/** Flatten primary and any additional AppAddonBinding entries into a uniform array. */
export function allAddonBindings(binding?: AppAddonBinding): AppAddonBinding[] {
  if (!binding) return [];
  return [binding, ...(binding.additional || [])].filter((b) => Boolean(b.repository?.trim()));
}

/** Derive UI labels uniformly from an App's addonBinding metadata rather than hardcoding app names. */
export function addonBindingMeta(binding?: AppAddonBinding): AddonBindingMeta {
  const repo = (binding?.repository || "").trim().toLowerCase();
  const mode = (binding?.installMode || "").trim().toLowerCase();
  const targetParts = (binding?.targetDir || "").trim().toLowerCase().split("/").filter(Boolean);
  const targetLeaf = targetParts[targetParts.length - 1] || "";
  const isSteam =
    repo.includes("workshop") ||
    mode === "tmod_latest" ||
    mode === "dst_local_mods" ||
    mode === "workshop_tree" ||
    mode === "cs2_workshop";
  const isPlugin =
    !isSteam &&
    (mode === "" || mode === "flat_dir" || mode === "oxide" || mode === "cs2_plugin") &&
    targetLeaf !== "mods";
  const noun: "插件" | "模组" | "地图" =
    mode === "cs2_workshop" ? "地图" : isPlugin ? "插件" : "模组";
  const marketLabel = isSteam ? "创意工坊" : isPlugin ? "插件市场" : "模组市场";
  return {
    isSteam,
    isPlugin,
    noun,
    idLabel: isSteam ? "工坊编号" : `${noun}标识`,
    marketLabel,
    browseActionLabel: isSteam ? "浏览创意工坊" : `添加${noun}`,
  };
}

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export const CONFIG_OPTIONS_SOURCE_ADDON_MAPS = "addon_maps";

export const INSTANCE_COVER_ANNOTATION = "core.gamepanel.io/cover-image";
export const INSTANCE_APP_CONFIG_ANNOTATION = "core.gamepanel.io/app-config";
export const INSTANCE_APP_VERSION_ANNOTATION = "core.gamepanel.io/app-version";
export const INSTANCE_RESTART_REQUESTED_AT_ANNOTATION = "core.gamepanel.io/restart-requested-at";

/** Game id, display title and cover image for a server, derived from its custom annotation, app/preset label, or image. */
export function gameOf(inst: InstanceItem, apps?: AppItem[]) {
  const customCover = inst.metadata.annotations?.[INSTANCE_COVER_ANNOTATION]?.trim() || "";
  const presetLabel =
    inst.metadata.labels?.["gamepanel.io/preset"]?.trim() ||
    inst.metadata.labels?.["gamepanel.io/app"]?.trim() ||
    "";
  const repo = inst.spec.image?.split(":")[0] || "";
  const matchedApp =
    resolveRichApp(presetLabel, apps) ||
    (!presetLabel && repo && repo !== "gamepanel/runtime"
      ? (apps || []).find((a) => a.metadata.name !== "custom-container" && a.image?.split(":")[0] === repo)
      : undefined);

  if (matchedApp) {
    const id = matchedApp.metadata.name;
    const defaultCover = gameCover(id, matchedApp.coverImage);
    return {
      id,
      title: matchedApp.displayName || id,
      cover: customCover || defaultCover,
      defaultCover,
      customCover,
    };
  }

  if (presetLabel) {
    const defaultCover = gameCover(presetLabel);
    return {
      id: presetLabel,
      title: presetLabel,
      cover: customCover || defaultCover,
      defaultCover,
      customCover,
    };
  }

  return {
    id: "custom-container",
    title: "自定义镜像",
    cover: customCover || DEFAULT_GAME_COVER,
    defaultCover: DEFAULT_GAME_COVER,
    customCover,
  };
}

/** Link styled like the kit's IconButton (for navigation actions). */
export function IconLink({ href, label, children }: { href: string; label: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      aria-label={label}
      title={label}
      className={cx(
        "inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#F1F3FB] text-[#3D4258] transition-colors hover:bg-[#E3E7FA] hover:text-[#2B3AE8]",
        focusRing,
      )}
    >
      {children}
    </Link>
  );
}

export { MoreMenu, type MenuItem };

/** Confirm + delete a server. */
export function DeleteServerDialog({
  inst,
  loading,
  onConfirm,
  onClose,
}: {
  inst: InstanceItem | null;
  loading?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <ConfirmDialog
      open={!!inst}
      title={`删除服务器 ${inst?.metadata.name ?? ""}？`}
      body="服务器上的世界存档、配置和文件都会被清除，且无法恢复。需要保留存档的话，请先在「备份」里做一次备份。"
      confirmLabel="删除服务器"
      danger
      loading={loading}
      onConfirm={onConfirm}
      onClose={onClose}
    />
  );
}

/** Merge static options, dynamic options (e.g. workshop addon_maps), and an existing saved value cleanly. */
export function mergeDynamicFieldOptions(
  staticOptions: AppConfigOption[] | undefined,
  dynamicOptions: AppConfigOption[],
  currentValue?: string,
): AppConfigOption[] {
  const out: AppConfigOption[] = [];
  const seen = new Set<string>();
  const push = (opt: AppConfigOption) => {
    const val = (opt.value || "").trim();
    if (!val || seen.has(val)) return;
    seen.add(val);
    out.push({
      value: val,
      label: opt.label || val,
      ...(opt.labelEn ? { labelEn: opt.labelEn } : {}),
    });
  };
  for (const opt of staticOptions || []) push(opt);
  for (const opt of dynamicOptions) push(opt);
  const cur = (currentValue || "").trim();
  if (cur && !seen.has(cur)) {
    push({ value: cur, label: cur });
  }
  return out;
}

/** Resolve selectable map options from Ready artifacts backing an instance's enabled addons. */
export function useAddonMapOptions(
  addons: InstanceAddon[] | undefined,
  enabled: boolean,
): AppConfigOption[] {
  const { apiFetch, token } = useConsole();
  const activeAddons = useMemo(
    () =>
      (addons || []).filter(
        (a) => !a.disabled && a.repository?.trim() && a.package?.trim() && a.version?.trim(),
      ),
    [addons],
  );
  const reposKey = useMemo(
    () => Array.from(new Set(activeAddons.map((a) => a.repository.trim()))).sort().join(","),
    [activeAddons],
  );
  const [artifacts, setArtifacts] = useState<RegistryArtifact[]>([]);

  const loadRepoArtifacts = useCallback(async () => {
    if (!enabled || !reposKey) {
      setArtifacts([]);
      return;
    }
    const repos = reposKey.split(",").filter(Boolean);
    const lists = await Promise.all(
      repos.map(async (repo) => {
        try {
          const params = new URLSearchParams({
            fieldSelector: `repository=${repo}`,
            limit: "200",
          });
          const res = await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts?${params.toString()}`);
          if (!res.ok) return [] as RegistryArtifact[];
          const data = (await res.json()) as { items?: RegistryArtifact[] };
          return data.items || [];
        } catch {
          return [] as RegistryArtifact[];
        }
      }),
    );
    setArtifacts(lists.flat());
  }, [apiFetch, enabled, reposKey]);

  useEffect(() => {
    if (!enabled || !reposKey) {
      setArtifacts([]);
      return;
    }
    void loadRepoArtifacts();
    const repoSet = new Set(reposKey.split(",").filter(Boolean));
    const unsub = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: loadRepoArtifacts,
      onEvent: (ev) => {
        const obj = ev.object;
        if (!obj?.metadata?.name || !repoSet.has(obj.spec?.repository || "")) return;
        setArtifacts((prev) => {
          if (ev.type === "DELETED") {
            return prev.filter((item) => item.metadata.name !== obj.metadata.name);
          }
          const idx = prev.findIndex((item) => item.metadata.name === obj.metadata.name);
          if (idx >= 0) {
            const next = prev.slice();
            next[idx] = obj;
            return next;
          }
          return [...prev, obj];
        });
      },
    });
    return () => unsub();
  }, [enabled, loadRepoArtifacts, reposKey, token]);

  return useMemo(() => {
    if (!enabled || activeAddons.length === 0 || artifacts.length === 0) return [];
    const byKey = new Map<string, RegistryArtifact>();
    for (const art of artifacts) {
      byKey.set(`${art.spec.repository}::${art.spec.package}::${art.spec.version}`, art);
    }
    const seen = new Set<string>();
    const options: AppConfigOption[] = [];
    for (const addon of activeAddons) {
      const art = byKey.get(`${addon.repository.trim()}::${addon.package.trim()}::${addon.version.trim()}`);
      if (art?.status?.phase !== "Ready" || !art.status.mapFolders?.length) continue;
      const addonTitle = (addon.title || "").trim();
      for (const rawMap of art.status.mapFolders) {
        const mapCode = (rawMap || "").trim();
        if (!mapCode || seen.has(mapCode)) continue;
        seen.add(mapCode);
        options.push({
          value: mapCode,
          label: addonTitle && addonTitle !== mapCode ? `${mapCode} · ${addonTitle}` : mapCode,
        });
      }
    }
    return options;
  }, [activeAddons, artifacts, enabled]);
}
