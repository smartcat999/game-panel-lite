"use client";

import { useEffect, useId, useMemo, useState } from "react";
import { ExternalLink, RotateCcw, Search, Sparkles } from "lucide-react";
import { Badge, Button, Input, Mono, Segmented, Select, Switch, focusRing } from "../ui";
import { useConsole } from "../../../console/ConsoleProvider";
import { subscribeResourceWatch } from "../../../console/lib/ws";
import type { AppConfigField, AppConfigOption, AppItem, InstanceAddon, RegistryArtifact } from "../../../console/lib/types";
import { defaultAppConfigValues, resolveRichApp } from "../../../console/lib/presets";
import { CONFIG_OPTIONS_SOURCE_ADDON_MAPS, mergeDynamicFieldOptions, useAddonMapOptions } from "./shared";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

const KLEI_TOKEN_URL = "https://accounts.klei.com/account/game/servers?game=DontStarveTogether";

export function isFloatingVersionAlias(v: string): boolean {
  const low = v.trim().toLowerCase();
  return low === "latest" || low === "stable";
}

export function compareGameVersionsDesc(a: string, b: string, _recommended?: string): number {
  void _recommended;
  const va = a.trim();
  const vb = b.trim();
  if (va === vb) return 0;
  const aAlias = isFloatingVersionAlias(va);
  const bAlias = isFloatingVersionAlias(vb);
  if (aAlias !== bAlias) return aAlias ? 1 : -1;
  return vb.replace(/^v/i, "").localeCompare(va.replace(/^v/i, ""), undefined, { numeric: true, sensitivity: "base" });
}

function parseGroupParts(groupRaw?: string): {
  isBasic: boolean;
  scope: string;
  category: string;
} {
  const g = (groupRaw || "").trim() || "基础设置";
  if (g === "基础设置") {
    return { isBasic: true, scope: "基础设置", category: "基础设置" };
  }
  const slashIdx = g.indexOf(" / ");
  if (slashIdx > 0) {
    return {
      isBasic: false,
      scope: g.slice(0, slashIdx).trim(),
      category: g.slice(slashIdx + 3).trim(),
    };
  }
  return { isBasic: false, scope: "高级游戏设置", category: g };
}

export function GameConfigEditor({
  app: rawApp,
  values,
  onChange,
  version,
  onVersionChange,
  addons,
  disabled,
  syncUrlParams = true,
}: {
  app: AppItem;
  values: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  version?: string;
  onVersionChange?: (v: string) => void;
  addons?: InstanceAddon[];
  disabled?: boolean;
  syncUrlParams?: boolean;
}) {
  const { apiFetch, token } = useConsole();
  const versionSelectId = useId();
  const app = useMemo(() => resolveRichApp(rawApp.metadata.name, [rawApp]) || rawApp, [rawApp]);
  const fields = useMemo(() => app.configSchema || [], [app.configSchema]);
  const needsAddonMaps = useMemo(
    () => fields.some((f) => f.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS),
    [fields],
  );
  const addonMapOptions = useAddonMapOptions(addons, needsAddonMaps);
  const presets = useMemo(() => app.configPresets || [], [app.configPresets]);
  const [registryArtifacts, setRegistryArtifacts] = useState<RegistryArtifact[]>([]);

  const registryVersions = useMemo(
    () =>
      Array.from(
        new Set(
          registryArtifacts
            .filter((a) => a.status?.phase === "Ready" && a.spec.version)
            .map((a) => a.spec.version),
        ),
      ),
    [registryArtifacts],
  );

  useEffect(() => {
    const sb = app.serverBinding;
    if (!sb?.repository || !sb?.package) {
      setRegistryArtifacts([]);
      return;
    }
    let cancelled = false;
    const selector = encodeURIComponent(`repository=${sb.repository},package=${sb.package}`);
    const loadArtifacts = () => {
      apiFetch(`/apis/registry.gamepanel.io/v1/artifacts?fieldSelector=${selector}&limit=100`)
        .then((res) => (res.ok ? res.json() : null))
        .then((data: { items?: RegistryArtifact[] } | null) => {
          if (cancelled || !data?.items) return;
          setRegistryArtifacts(data.items);
        })
        .catch(() => {
          if (!cancelled) setRegistryArtifacts([]);
        });
    };
    loadArtifacts();
    const unsub = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: loadArtifacts,
      onEvent: (ev) => {
        const obj = ev.object;
        if (!obj?.metadata?.name) return;
        if (obj.spec?.repository !== sb.repository || obj.spec?.package !== sb.package) return;
        setRegistryArtifacts((prev) => {
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
    return () => {
      cancelled = true;
      unsub();
    };
  }, [apiFetch, token, app.serverBinding]);

  const versions = useMemo(() => {
    const raw = Array.from(new Set([...(app.versions || []), ...registryVersions]));
    const hasConcrete = raw.some((v) => !isFloatingVersionAlias(v));
    const base = hasConcrete
      ? raw.filter((v) => !isFloatingVersionAlias(v) || registryVersions.includes(v))
      : raw;
    if (version && !base.includes(version) && (!isFloatingVersionAlias(version) || !hasConcrete)) {
      base.push(version);
    }
    return base.sort((a, b) => compareGameVersionsDesc(a, b));
  }, [app.versions, registryVersions, version]);
  const defaults = useMemo(() => defaultAppConfigValues(app), [app]);

  const basicFields = useMemo(
    () => fields.filter((f) => parseGroupParts(f.group).isBasic),
    [fields],
  );
  const advancedFields = useMemo(
    () => fields.filter((f) => !parseGroupParts(f.group).isBasic),
    [fields],
  );

  // Scopes inside Advanced View (e.g. ["地上世界", "洞穴世界"] for DST, ["服务器选项", "沙盒规则"] for PZ)
  const advancedScopes = useMemo(() => {
    const out: string[] = [];
    for (const f of advancedFields) {
      const { scope } = parseGroupParts(f.group);
      if (!out.includes(scope)) out.push(scope);
    }
    return out;
  }, [advancedFields]);

  function syncEditorUrl(updates: Record<string, string | null>) {
    if (!syncUrlParams || typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === undefined || v === "") sp.delete(k);
      else sp.set(k, v);
    }
    const qs = sp.toString();
    const nextUrl = qs ? `${window.location.pathname}?${qs}` : window.location.pathname;
    window.history.replaceState(window.history.state, "", nextUrl);
  }

  const [activeView, setActiveView] = useState<"basic" | "advanced">(() => {
    if (typeof window !== "undefined") {
      const v = new URLSearchParams(window.location.search).get("view");
      if (v === "advanced" && advancedFields.length > 0) return "advanced";
      if (v === "basic" && basicFields.length > 0) return "basic";
    }
    return basicFields.length > 0 ? "basic" : "advanced";
  });
  const [activeScope, setActiveScope] = useState<string>(() => {
    if (typeof window !== "undefined") {
      const s = new URLSearchParams(window.location.search).get("scope");
      if (s && advancedScopes.includes(s)) return s;
    }
    return advancedScopes[0] || "高级游戏设置";
  });
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<"all" | "modified">("all");
  const [activePresetId, setActivePresetId] = useState<string>("");

  const isDst = app.metadata.name === "dont-starve-together";
  const cavesEnabled =
    !isDst || String(values["caves.enabled"] ?? defaults["caves.enabled"] ?? "true") !== "false";

  const prevAppNameRef = useState(() => app.metadata.name)[0];
  useEffect(() => {
    if (app.metadata.name === prevAppNameRef) return;
    setActiveView(basicFields.length > 0 ? "basic" : "advanced");
    setActiveScope(advancedScopes[0] || "高级游戏设置");
    setQuery("");
    setFilter("all");
    setActivePresetId("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app.metadata.name]);

  useEffect(() => {
    if (!advancedScopes.includes(activeScope)) {
      setActiveScope(advancedScopes[0] || "高级游戏设置");
    }
  }, [advancedScopes, activeScope]);

  useEffect(() => {
    if (isDst && !cavesEnabled && activeScope === "洞穴世界") {
      setActiveScope("地上世界");
    }
  }, [isDst, cavesEnabled, activeScope]);

  const scopeFields = useMemo(
    () => advancedFields.filter((f) => parseGroupParts(f.group).scope === activeScope),
    [advancedFields, activeScope],
  );

  const categories = useMemo(() => {
    const out: string[] = [];
    for (const f of scopeFields) {
      const { category } = parseGroupParts(f.group);
      if (!out.includes(category)) out.push(category);
    }
    return out;
  }, [scopeFields]);

  const [activeCategory, setActiveCategory] = useState<string>(categories[0] || "");

  useEffect(() => {
    if (!categories.includes(activeCategory)) {
      setActiveCategory(categories[0] || "");
    }
  }, [categories, activeCategory]);

  const isModified = (f: AppConfigField) => {
    const cur = values[f.name] ?? f.default ?? "";
    const def = defaults[f.name] ?? f.default ?? "";
    return String(cur) !== String(def);
  };

  const currentScopeModifiedCount = useMemo(
    () => scopeFields.filter((f) => isModified(f)).length,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [scopeFields, values, defaults],
  );

  const normalizedQuery = query.trim().toLowerCase();

  const matchedAdvancedFields = useMemo(() => {
    return scopeFields.filter((f) => {
      if (filter === "modified" && !isModified(f)) return false;
      const { category } = parseGroupParts(f.group);
      if (!normalizedQuery) return category === activeCategory;
      const hay = `${f.label} ${f.name} ${f.help || ""} ${f.envKey || ""} ${category}`.toLowerCase();
      return hay.includes(normalizedQuery);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scopeFields, filter, normalizedQuery, activeCategory, values, defaults]);

  const visibleCategories = useMemo(() => {
    if (normalizedQuery || filter === "modified") {
      return categories.filter((cat) =>
        matchedAdvancedFields.some((f) => parseGroupParts(f.group).category === cat),
      );
    }
    return activeCategory ? [activeCategory] : [];
  }, [normalizedQuery, filter, categories, matchedAdvancedFields, activeCategory]);

  function updateField(name: string, val: string) {
    setActivePresetId("");
    const next = { ...values, [name]: val };
    // Sync DST gameMode preset overrides like game-panel-lite
    if (isDst && name === "gameplay.gameMode") {
      if (val === "survival") {
        next["world.overrides.spawnmode"] = "fixed";
        next["world.overrides.ghostenabled"] = "always";
        next["world.overrides.portalresurection"] = "none";
        next["world.overrides.ghostsanitydrain"] = "always";
        next["world.overrides.resettime"] = "default";
      } else if (val === "endless") {
        next["world.overrides.spawnmode"] = "fixed";
        next["world.overrides.ghostenabled"] = "always";
        next["world.overrides.portalresurection"] = "always";
        next["world.overrides.ghostsanitydrain"] = "none";
        next["world.overrides.resettime"] = "none";
      } else if (val === "wilderness") {
        next["world.overrides.spawnmode"] = "scatter";
        next["world.overrides.ghostenabled"] = "none";
        next["world.overrides.portalresurection"] = "none";
        next["world.overrides.ghostsanitydrain"] = "none";
        next["world.overrides.resettime"] = "none";
      }
    }
    onChange(next);
  }

  function applyPreset(presetId: string) {
    const p = presets.find((x) => x.id === presetId);
    if (!p) return;
    setActivePresetId(presetId);
    onChange({ ...defaults, ...values, ...p.values });
  }

  const basicModifiedCount = useMemo(
    () => basicFields.filter((f) => isModified(f)).length,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [basicFields, values, defaults],
  );

  function restoreBasicDefaults() {
    const next = { ...values };
    for (const f of basicFields) {
      next[f.name] = defaults[f.name] ?? "";
    }
    setActivePresetId("");
    onChange(next);
  }

  function restoreScopeDefaults() {
    const next = { ...values };
    for (const f of scopeFields) {
      next[f.name] = defaults[f.name] ?? "";
    }
    setActivePresetId("");
    onChange(next);
  }

  if (fields.length === 0) return null;

  const newestVersion = versions.find((v) => !isFloatingVersionAlias(v)) || versions[0];
  const defaultVersion =
    app.recommendedVersion && versions.includes(app.recommendedVersion) ? app.recommendedVersion : newestVersion;
  const upgradeAvailable =
    !!version &&
    !!newestVersion &&
    !isFloatingVersionAlias(newestVersion) &&
    !isFloatingVersionAlias(version) &&
    compareGameVersionsDesc(newestVersion, version) < 0;
  const basicNonBoolean = basicFields.filter((f) => f.type !== "boolean");
  const basicBooleans = basicFields.filter((f) => f.type === "boolean");

  return (
    <div className="space-y-4">
      {/* Top Bar: Playstyle Presets & Version Picker */}
      {(presets.length > 0 || (versions.length > 1 && onVersionChange)) && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl bg-[#F7F8FC] px-4 py-2.5">
          {presets.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1.5 text-xs font-medium text-[#5B6075]">
                <Sparkles className="h-3.5 w-3.5 text-[#2B3AE8]" />
                预设
              </span>
              {presets.map((p) => {
                const active = activePresetId === p.id;
                const shortName = p.name.replace(/[（(].*[）)]$/, "").trim() || p.name;
                return (
                  <button
                    key={p.id}
                    type="button"
                    disabled={disabled}
                    title={p.description ? `${p.name} — ${p.description}` : p.name}
                    onClick={() => applyPreset(p.id)}
                    className={cx(
                      "h-8 rounded-full border px-3 text-xs font-medium transition",
                      active
                        ? "border-[#2B3AE8] bg-[#EEF1FF] font-semibold text-[#2B3AE8]"
                        : "border-[#E3E6F2] bg-white text-[#5B6075] hover:border-[#B8C0E6] hover:text-[#14161F]",
                      focusRing,
                    )}
                  >
                    {shortName}
                  </button>
                );
              })}
            </div>
          ) : (
            <div />
          )}

          {versions.length > 0 && (
            <div className="flex items-center">
              {versions.length >= 1 && onVersionChange ? (
                <div className="relative inline-flex items-center gap-2">
                  <label htmlFor={versionSelectId} className="text-xs font-medium text-[#5B6075]">
                    游戏版本
                  </label>
                  <Select
                    id={versionSelectId}
                    name="gameVersion"
                    disabled={disabled}
                    title={upgradeAvailable ? `有新版本 ${newestVersion}，选择后保存即可升级` : undefined}
                    value={version && versions.includes(version) ? version : defaultVersion}
                    onChange={(e) => onVersionChange(e.target.value)}
                    className="h-8 w-auto rounded-full border-[#E3E6F2] pl-3 font-mono text-xs font-semibold"
                  >
                    {versions.map((ver) => {
                      const isNewest = ver === newestVersion && !isFloatingVersionAlias(ver);
                      const isPinnedDefault = ver === defaultVersion && defaultVersion !== newestVersion;
                      return (
                        <option key={ver} value={ver}>
                          {ver}
                          {isNewest ? " · 最新" : isPinnedDefault ? " · 默认" : ""}
                        </option>
                      );
                    })}
                  </Select>
                  {upgradeAvailable && (
                    <span
                      aria-label="有新版本可升级"
                      className="pointer-events-none absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full bg-[#2B3AE8] ring-2 ring-white"
                    />
                  )}
                </div>
              ) : (
                <span className="inline-flex h-7 items-center gap-1.5 rounded-full bg-white px-3 text-xs font-medium text-[#5B6075] ring-1 ring-[#E3E6F2]">
                  <span className="h-1.5 w-1.5 rounded-full bg-[#1F9D55]" />
                  <span>游戏版本</span>
                  <Mono className="font-semibold text-[#14161F]">
                    {version || defaultVersion || versions[0]}
                  </Mono>
                </span>
              )}
            </div>
          )}
        </div>
      )}

      {/* Level 1 View Switcher (基础设置 | 高级设置) + Right Scope/Shard Switcher (● 地上世界 | ● 洞穴世界) */}
      {advancedFields.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Segmented
            label="配置模式切换"
            value={activeView}
            onChange={(v) => {
              const next = v as "basic" | "advanced";
              setActiveView(next);
              syncEditorUrl({
                view: next === "basic" ? null : next,
                scope: next === "basic" ? null : activeScope,
              });
            }}
            items={[
              { value: "basic", label: "基础设置" },
              { value: "advanced", label: "高级设置" },
            ]}
          />

          {activeView === "basic" && basicModifiedCount > 0 && !disabled && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              icon={<RotateCcw className="h-3.5 w-3.5" />}
              onClick={restoreBasicDefaults}
            >
              恢复默认
            </Button>
          )}

          {activeView === "advanced" && advancedScopes.length > 1 && (
            <Segmented
              size="sm"
              label="世界或规则维度切换"
              value={activeScope}
              onChange={(scope) => {
                setActiveScope(scope);
                setQuery("");
                syncEditorUrl({ view: "advanced", scope });
              }}
              items={advancedScopes.map((scope) => {
                const scopeDisabled = isDst && scope === "洞穴世界" && !cavesEnabled;
                return {
                  value: scope,
                  label: scope,
                  disabled: scopeDisabled,
                  title: scopeDisabled ? "请先在「基础设置」中开启「启用洞穴世界」" : undefined,
                };
              })}
            />
          )}
        </div>
      )}

      {/* VIEW A: Basic Settings (2-Column Borderless Controls + Characteristic Toggles) */}
      {activeView === "basic" ? (
        <div className="space-y-5">
          {basicNonBoolean.length > 0 && (
            <div className="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
              {basicNonBoolean.map((f) => (
                <ConfigFieldControl
                  key={f.name}
                  field={f}
                  rawVal={values[f.name] ?? f.default ?? ""}
                  modified={isModified(f)}
                  dynamicOptions={f.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS ? addonMapOptions : undefined}
                  disabled={disabled}
                  onUpdate={(val) => updateField(f.name, val)}
                />
              ))}
            </div>
          )}

          {basicBooleans.length > 0 && (
            <div className={cx("space-y-2.5", basicNonBoolean.length > 0 && "border-t border-[#EEF0F7] pt-4")}>
              <div className="text-xs font-medium text-[#5B6075]">特性开关</div>
              <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
                {basicBooleans.map((f) => {
                  const switchId = `game-cfg-${f.name}`;
                  const raw = values[f.name] ?? f.default ?? "";
                  const checked = String(raw).toLowerCase() === "true";
                  const modified = isModified(f);
                  return (
                    <div
                      key={f.name}
                      title={f.help ? `${f.label} — ${f.help}` : f.label}
                      className="flex items-center justify-between gap-3 rounded-xl bg-[#F7F8FC] px-4 py-3"
                    >
                      <div className="flex min-w-0 items-center gap-2">
                        <label
                          htmlFor={switchId}
                          className="cursor-pointer truncate text-[13px] font-medium text-[#14161F] select-none"
                        >
                          {f.label}
                        </label>
                        {modified && (
                          <span
                            aria-label="已修改"
                            title="已修改"
                            className="h-1.5 w-1.5 shrink-0 rounded-full bg-[#2B3AE8]"
                          />
                        )}
                      </div>
                      <Switch
                        id={switchId}
                        checked={checked}
                        disabled={disabled}
                        onChange={(next) => updateField(f.name, next ? "true" : "false")}
                      />
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      ) : (
        /* VIEW B: Advanced Game Settings (Single-Row Filter Bar + 2-Column Borderless Field Grid) */
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 border-y border-[#EEF0F7] py-3">
            <div className="flex flex-wrap items-center gap-2.5">
              {categories.length > 1 &&
                (categories.length <= 4 ? (
                  <Segmented
                    size="sm"
                    label="参数分类"
                    value={normalizedQuery ? "" : activeCategory}
                    onChange={(cat) => {
                      setQuery("");
                      setActiveCategory(cat);
                    }}
                    items={categories.map((cat) => ({ value: cat, label: cat }))}
                  />
                ) : (
                  <Select
                    aria-label="参数分类"
                    value={activeCategory}
                    onChange={(e) => {
                      setQuery("");
                      setActiveCategory(e.target.value);
                    }}
                    className="h-9 w-auto min-w-[168px] text-xs"
                  >
                    {categories.map((cat) => (
                      <option key={cat} value={cat}>
                        {cat}
                      </option>
                    ))}
                  </Select>
                ))}

              {visibleCategories.length === 1 &&
                (visibleCategories[0].startsWith("生成 ·") ||
                  visibleCategories[0].includes("世界生成")) && (
                  <Badge tone="warning">仅新世界生效</Badge>
                )}

              <span className="text-xs text-[#8A90A6]">
                共 {matchedAdvancedFields.length} 项
                {currentScopeModifiedCount > 0 ? ` · 已改 ${currentScopeModifiedCount} 项` : ""}
              </span>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <div className="relative w-52">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]" />
                <Input
                  placeholder="搜索参数..."
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  className="h-9 bg-white pl-8 text-xs"
                />
              </div>

              <Segmented
                size="sm"
                label="全部或已修改过滤"
                value={filter}
                onChange={(v) => setFilter(v as "all" | "modified")}
                items={[
                  { value: "all", label: "全部" },
                  { value: "modified", label: "已改" },
                ]}
              />

              {currentScopeModifiedCount > 0 && !disabled && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  icon={<RotateCcw className="h-3.5 w-3.5" />}
                  onClick={restoreScopeDefaults}
                >
                  重置
                </Button>
              )}
            </div>
          </div>

          <div className="space-y-5 pt-1">
            {visibleCategories.map((cat) => {
              const groupFields = matchedAdvancedFields.filter(
                (f) => parseGroupParts(f.group).category === cat,
              );
              if (groupFields.length === 0) return null;
              const isWorldGen = cat.startsWith("生成 ·") || cat.includes("世界生成");

              return (
                <section
                  key={cat}
                  className={cx(
                    visibleCategories.length > 1 && "border-t border-[#EEF0F7] pt-4 first:border-t-0 first:pt-0",
                  )}
                >
                  {visibleCategories.length > 1 && (
                    <div className="mb-3 flex items-center gap-2">
                      <h4 className="text-xs font-semibold text-[#5B6075]">{cat}</h4>
                      {isWorldGen && <Badge tone="warning">仅新世界生效</Badge>}
                    </div>
                  )}

                  <div className="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
                    {groupFields.map((f) => (
                      <ConfigFieldControl
                        key={f.name}
                        field={f}
                        rawVal={values[f.name] ?? f.default ?? ""}
                        modified={isModified(f)}
                        dynamicOptions={f.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS ? addonMapOptions : undefined}
                        disabled={disabled}
                        onUpdate={(val) => updateField(f.name, val)}
                      />
                    ))}
                  </div>
                </section>
              );
            })}

            {matchedAdvancedFields.length === 0 && (
              <div className="py-10 text-center text-sm text-[#8A90A6]">未找到匹配参数</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function ConfigFieldControl({
  field: f,
  rawVal,
  modified,
  dynamicOptions,
  disabled,
  onUpdate,
}: {
  field: AppConfigField;
  rawVal: string;
  modified: boolean;
  dynamicOptions?: AppConfigOption[];
  disabled?: boolean;
  onUpdate: (val: string) => void;
}) {
  const selectOptions = useMemo(() => {
    if (f.type !== "select") return f.options || [];
    if (f.optionsSource === CONFIG_OPTIONS_SOURCE_ADDON_MAPS) {
      return mergeDynamicFieldOptions(f.options, dynamicOptions || [], rawVal);
    }
    return f.options || [];
  }, [dynamicOptions, f.options, f.optionsSource, f.type, rawVal]);
  const id = `game-cfg-${f.name}`;
  const isSlider = f.type === "number" && f.min !== undefined && f.max !== undefined;
  const numVal = Number(rawVal || f.default || f.min || 0);
  const min = f.min ?? 0;
  const max = f.max ?? 100;
  const step = f.step ?? 1;
  const pct = max > min ? Math.min(100, Math.max(0, ((numVal - min) / (max - min)) * 100)) : 0;
  const isKleiToken = f.name === "clusterToken" || f.name === "identity.clusterToken";
  const checked = String(rawVal).toLowerCase() === "true";

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <label
            htmlFor={id}
            title={f.help ? `${f.label} (${f.name}) — ${f.help}` : `${f.label} (${f.name})`}
            className="truncate text-[13px] font-medium text-[#3D4258]"
          >
            {f.label}
            {f.required && <span className="ml-1 text-[#D92D20]">*</span>}
          </label>
          {modified && <span className="shrink-0 text-[11px] font-medium text-[#2B3AE8]">已改</span>}
        </div>

        {isKleiToken ? (
          <a
            href={KLEI_TOKEN_URL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex shrink-0 items-center gap-1 text-[11px] font-medium text-[#2B3AE8] hover:underline"
          >
            获取 Klei 令牌
            <ExternalLink className="h-3 w-3" />
          </a>
        ) : isSlider ? (
          <Mono className="text-xs font-semibold text-[#2B3AE8]">{rawVal}</Mono>
        ) : null}
      </div>

      {f.type === "boolean" ? (
        <div className="flex h-10 items-center justify-between rounded-xl bg-[#F7F8FC] px-3.5 text-[13px] text-[#5B6075]">
          <span>{checked ? "已开启" : "已关闭"}</span>
          <Switch
            id={id}
            checked={checked}
            disabled={disabled}
            onChange={(next) => onUpdate(next ? "true" : "false")}
          />
        </div>
      ) : f.type === "select" ? (
        <Select
          id={id}
          disabled={disabled}
          value={String(rawVal)}
          onChange={(e) => onUpdate(e.target.value)}
          className="h-10 text-[13px]"
        >
          {selectOptions.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </Select>
      ) : isSlider ? (
        <div className="flex h-10 items-center gap-3">
          <span className="font-mono text-xs tabular-nums text-[#8A90A6]">{min}</span>
          <div className="relative flex flex-1 items-center">
            <input
              id={id}
              type="range"
              min={min}
              max={max}
              step={step}
              disabled={disabled}
              value={numVal}
              onChange={(e) => onUpdate(e.target.value)}
              className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-[#E3E6F2] accent-[#2B3AE8]"
              style={{
                background: `linear-gradient(to right, #2B3AE8 0%, #2B3AE8 ${pct}%, #E3E6F2 ${pct}%, #E3E6F2 100%)`,
              }}
            />
          </div>
          <span className="font-mono text-xs tabular-nums text-[#8A90A6]">{max}</span>
        </div>
      ) : (
        <Input
          id={id}
          type={f.type === "password" ? "password" : f.type === "number" ? "number" : "text"}
          disabled={disabled}
          value={String(rawVal)}
          placeholder={f.default || ""}
          onChange={(e) => onUpdate(e.target.value)}
          className="h-10 text-[13px]"
        />
      )}
    </div>
  );
}
