"use client";

import { ExternalLink, RotateCcw, Search } from "lucide-react";
import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import { SecretInput } from "@/components/secret-input";
import { Button, Input } from "@/components/ui";
import { dstConfigGroupLabelKey } from "@/lib/dst-config";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { isAdvancedProviderConfigField, isProviderFieldModified, providerConfigValue, type ProviderConfigPayload } from "@/lib/provider-config";
import { providerOptionLabel } from "@/lib/provider-option-label";
import type { ProviderConfigField } from "@/lib/types";
import { cn } from "@/lib/utils";

type ConfigValue = string | boolean;
const kleiServerTokenURL = "https://accounts.klei.com/account/game/servers?game=DontStarveTogether";

export function ProviderConfigEditor({
  disabled = false,
  errors = {},
  fieldHelp,
  fieldLabel,
  fields,
  hasUnsavedWorldGenerationChanges = false,
  onChange,
  onRegenerateWorld,
  onRestoreDefaults,
  payload,
  providerKey,
  surface = "create"
}: {
  disabled?: boolean;
  errors?: Record<string, string>;
  fieldHelp: (field: ProviderConfigField) => string;
  fieldLabel: (field: ProviderConfigField) => string;
  fields: ProviderConfigField[];
  hasUnsavedWorldGenerationChanges?: boolean;
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  onRegenerateWorld?: () => void;
  onRestoreDefaults?: (fields: ProviderConfigField[]) => void;
  payload: ProviderConfigPayload;
  providerKey: string;
  surface?: "create" | "server";
}) {
  const { locale, t } = useI18n();
  const [activeView, setActiveView] = useState<"basic" | "advanced">("basic");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<"all" | "modified">("all");

  const isDst = providerKey === "dont-starve-together";
  const [activeShard, setActiveShard] = useState<"master" | "caves">("master");

  const baseFields = useMemo(() => fields.filter((field) => !isAdvancedProviderConfigField(providerKey, field)), [fields, providerKey]);
  const cavesEnabled = providerConfigValue(payload, "caves.enabled") === true;

  // Filter advanced fields according to provider and selected shard
  const advancedFields = useMemo(() => {
    return fields.filter((field) => {
      if (!isAdvancedProviderConfigField(providerKey, field)) return false;
      if (!isDst) return true;
      if (activeShard === "caves") {
        return cavesEnabled && (field.name.startsWith("caves.overrides.") || field.group?.includes(".caves."));
      }
      return !field.name.startsWith("caves.overrides.") && !field.group?.includes(".caves.");
    });
  }, [activeShard, cavesEnabled, fields, isDst, providerKey]);

  const allAdvancedFields = useMemo(
    () => fields.filter((field) => isAdvancedProviderConfigField(providerKey, field) && (cavesEnabled || !field.name.startsWith("caves.overrides."))),
    [cavesEnabled, fields, providerKey]
  );
  const modifiedCount = allAdvancedFields.filter((field) => isProviderFieldModified(payload, field)).length;
  const currentShardModifiedCount = advancedFields.filter((field) => isProviderFieldModified(payload, field)).length;

  const groups = useMemo(() => Array.from(new Set(advancedFields.map((field) => field.group).filter((group): group is string => Boolean(group)))), [advancedFields]);
  const [activeGroup, setActiveGroup] = useState(groups[0] ?? "");

  useEffect(() => {
    if (!groups.includes(activeGroup)) setActiveGroup(groups[0] ?? "");
  }, [activeGroup, groups]);

  useEffect(() => {
    setActiveView("basic");
    setQuery("");
    setFilter("all");
    setActiveShard("master");
  }, [providerKey]);

  useEffect(() => {
    if (!cavesEnabled && activeShard === "caves") {
      setActiveShard("master");
    }
  }, [cavesEnabled, activeShard]);

  const normalizedQuery = query.trim().toLocaleLowerCase(locale === "zh" ? "zh-CN" : "en-US");
  const matchedFields = advancedFields.filter((field) => {
    if (filter === "modified" && !isProviderFieldModified(payload, field)) return false;
    if (!normalizedQuery) return field.group === activeGroup;
    return `${fieldLabel(field)} ${field.help ?? ""}`.toLocaleLowerCase(locale === "zh" ? "zh-CN" : "en-US").includes(normalizedQuery);
  });
  const visibleGroups = normalizedQuery ? groups.filter((group) => matchedFields.some((field) => field.group === group)) : activeGroup ? [activeGroup] : [];

  if (fields.length === 0) return <p className="mt-4 text-sm text-slate-500">{t("none")}</p>;

  return (
    <div className="mt-4 space-y-4">
      {allAdvancedFields.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2.5 pb-1">
          {/* Left: View Switcher (Basic / Advanced) */}
          <div role="tablist" aria-label={t("gameSettingsViews")} className="inline-flex rounded-lg border border-panel-line bg-slate-950/80 p-0.5 shadow-sm">
            {(["basic", "advanced"] as const).map((view) => (
              <button
                key={view}
                type="button"
                role="tab"
                aria-selected={activeView === view}
                className={cn(
                  "flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition",
                  activeView === view ? "bg-slate-800 text-slate-100 shadow-sm" : "text-slate-400 hover:text-slate-200"
                )}
                onClick={() => setActiveView(view)}
              >
                <span>{t(view === "basic" ? "basicGameSettings" : "advancedGameSettings")}</span>
                {view === "advanced" && modifiedCount > 0 ? (
                  <span aria-label={t("modifiedSettingsCount", { count: modifiedCount })} className="min-w-4 rounded-full bg-panel-green/20 px-1.5 py-0.5 text-center text-[10px] font-bold tabular-nums text-panel-green">
                    {modifiedCount}
                  </span>
                ) : null}
              </button>
            ))}
          </div>

          {/* Right: Shard Switcher (For DST in Advanced View) */}
          {activeView === "advanced" && isDst && (
            <div className="inline-flex items-center rounded-lg border border-panel-line bg-slate-950/80 p-0.5 shadow-sm">
              <button
                type="button"
                onClick={() => {
                  setActiveShard("master");
                  setQuery("");
                }}
                className={cn(
                  "flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition",
                  activeShard === "master"
                    ? "bg-panel-green/15 text-panel-green font-semibold shadow-sm"
                    : "text-slate-400 hover:text-slate-200"
                )}
              >
                <span className={cn("size-1.5 rounded-full", activeShard === "master" ? "bg-panel-green" : "bg-slate-600")} />
                <span>{t("dstShardMaster")}</span>
              </button>
              <button
                type="button"
                disabled={!cavesEnabled}
                onClick={() => {
                  if (cavesEnabled) {
                    setActiveShard("caves");
                    setQuery("");
                  }
                }}
                title={!cavesEnabled ? t("dstShardCavesDisabled") : undefined}
                className={cn(
                  "flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition disabled:opacity-30 disabled:cursor-not-allowed",
                  activeShard === "caves"
                    ? "bg-panel-green/15 text-panel-green font-semibold shadow-sm"
                    : "text-slate-400 hover:text-slate-200"
                )}
              >
                <span className={cn("size-1.5 rounded-full", activeShard === "caves" ? "bg-panel-green" : "bg-slate-600")} />
                <span>{t("dstShardCaves")}</span>
              </button>
            </div>
          )}
        </div>
      ) : null}

      {activeView === "advanced" && advancedFields.length > 0 ? (
        <div className="min-w-0 rounded-lg border border-panel-line bg-slate-950/40 p-3">
          {/* Top-right action toolbar (Unified, compact, gamer-aesthetic) */}
          <div className="flex flex-wrap items-center justify-between gap-2 pb-3 border-b border-panel-line/70">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-slate-300">
                {isDst
                  ? activeShard === "master"
                    ? t("dstShardMaster")
                    : t("dstShardCaves")
                  : t("advancedGameSettings")}
              </span>
              {currentShardModifiedCount > 0 && (
                <span className="rounded-full bg-panel-green/15 border border-panel-green/30 px-2 py-0.5 text-[10px] font-semibold text-panel-green font-mono">
                  {currentShardModifiedCount} {locale.startsWith("zh") ? "项已更改" : "modified"}
                </span>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2">
              {/* Search */}
              <div className="relative w-44 sm:w-56">
                <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-slate-500" />
                <Input
                  className="h-7 w-full pl-8 text-xs bg-slate-900 border-panel-line"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("searchGameSettings")}
                />
              </div>

              {/* All / Modified Pill */}
              <div className="inline-flex rounded-md border border-panel-line bg-slate-900 p-0.5">
                {(["all", "modified"] as const).map((value) => (
                  <button
                    key={value}
                    type="button"
                    className={cn(
                      "h-6 rounded px-2 text-xs font-medium transition",
                      filter === value ? "bg-slate-800 text-slate-100 font-semibold" : "text-slate-400 hover:text-slate-200"
                    )}
                    onClick={() => setFilter(value)}
                  >
                    {t(value === "all" ? "allSettings" : "modifiedSettings")}
                  </button>
                ))}
              </div>

              {/* Restore Defaults */}
              {currentShardModifiedCount > 0 && onRestoreDefaults && (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-7 px-2 text-xs text-slate-400 hover:text-white"
                  disabled={disabled}
                  onClick={() => onRestoreDefaults(advancedFields)}
                >
                  <RotateCcw aria-hidden="true" className="size-3 text-slate-400" />
                  <span>{t("restoreDefault")}</span>
                </Button>
              )}
            </div>
          </div>

          <div className="pt-3 lg:grid lg:grid-cols-[160px_minmax(0,1fr)]">
            <nav aria-label={t("settingsCategories")} className="flex gap-1 overflow-x-auto border-b border-panel-line py-1.5 lg:block lg:border-b-0 lg:border-r lg:pr-2.5">
              {groups.map((group) => {
                const count = advancedFields.filter((field) => field.group === group && isProviderFieldModified(payload, field)).length;
                return (
                  <button
                    key={group}
                    type="button"
                    className={cn(
                      "flex shrink-0 items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition lg:mb-1 lg:w-full font-medium",
                      activeGroup === group && !normalizedQuery
                        ? "bg-slate-800 text-white font-semibold"
                        : "text-slate-400 hover:bg-slate-900 hover:text-slate-200"
                    )}
                    onClick={() => {
                      setQuery("");
                      setActiveGroup(group);
                    }}
                  >
                    <span className="truncate">{groupLabel(group, locale, t)}</span>
                    {count > 0 ? (
                      <span className="text-[10px] font-bold text-panel-green bg-panel-green/15 px-1.5 py-0.2 rounded">
                        {count}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </nav>
            <div className="min-w-0 py-2 lg:pl-3.5">
              {visibleGroups.map((group) => {
                const groupFields = matchedFields.filter((field) => field.group === group);
                const effect = dstConfigGroupEffect(providerKey, group);
                if (groupFields.length === 0) return null;
                return (
                  <section key={group} className="mb-4 last:mb-0">
                    <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                      <div className="flex min-w-0 flex-wrap items-center gap-2">
                        <h5 className="text-xs font-semibold text-slate-200">{groupLabel(group, locale, t)}</h5>
                        {surface === "server" && effect ? (
                          <span className={cn(
                            "rounded px-1.5 py-0.2 text-[10px] font-medium",
                            effect === "worldgen" ? "bg-panel-gold/15 text-panel-gold" : "bg-slate-800 text-slate-400"
                          )}>
                            {t(effect === "worldgen" ? "worldGenerationAppliesOnRegenerate" : "worldSettingsApplyAfterRestart")}
                          </span>
                        ) : null}
                      </div>
                      <span className="text-[10px] text-slate-500 font-mono">{t("settingsCount", { count: groupFields.length })}</span>
                    </div>
                    {surface === "server" && effect === "worldgen" && onRegenerateWorld && !normalizedQuery ? (
                      <div className="mb-2.5 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-panel-gold/25 bg-panel-gold/5 px-3 py-1.5">
                        <p className="text-[11px] text-slate-400">
                          {hasUnsavedWorldGenerationChanges ? t("saveWorldGenerationBeforeRegenerate") : t("worldGenerationConfigHint")}
                        </p>
                        <Button type="button" variant="gold" className="h-7 shrink-0 px-2 text-[11px]" disabled={disabled || hasUnsavedWorldGenerationChanges} onClick={onRegenerateWorld}>
                          <RotateCcw aria-hidden="true" className="size-3" />
                          {t("regenerateWithCurrentSettings")}
                        </Button>
                      </div>
                    ) : null}
                    <div className="grid gap-2 grid-cols-1 sm:grid-cols-2 xl:grid-cols-3">
                      {groupFields.map((field) => (
                        <ConfigField key={field.name} disabled={disabled} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} />
                      ))}
                    </div>
                  </section>
                );
              })}
              {matchedFields.length === 0 ? <p className="py-8 text-center text-xs text-slate-500">{t("noGameSettingsMatch")}</p> : null}
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {/* Text and Select Controls (2-column compact grid) */}
          <div className="grid gap-2.5 sm:grid-cols-2">
            {baseFields.filter((f) => f.type !== "boolean").map((field) => (
              <ConfigField key={field.name} disabled={disabled} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} />
            ))}
          </div>

          {/* Boolean Feature Toggles (Compact 2-3 column switch pills) */}
          {baseFields.filter((f) => f.type === "boolean").length > 0 && (
            <div className="pt-1">
              <p className="text-[10px] font-bold uppercase tracking-wider text-slate-500 mb-1.5">
                {locale.startsWith("zh") ? "功能特性开关" : "Feature Toggles"}
              </p>
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {baseFields.filter((f) => f.type === "boolean").map((field) => (
                  <ConfigField key={field.name} disabled={disabled} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} />
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ConfigField({ disabled, error, field, help, label, onChange, payload, slider }: {
  disabled: boolean;
  error?: string;
  field: ProviderConfigField;
  help: string;
  label: string;
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  payload: ProviderConfigPayload;
  slider: boolean;
}) {
  const { t } = useI18n();
  const value = providerConfigValue(payload, field.name);
  const checked = value === true;
  const numericValue = Number(value ?? field.default ?? 0);
  const isRangeSlider = field.type === "number" && slider && field.min !== undefined && field.max !== undefined;
  const rangeFill = field.min !== undefined && field.max !== undefined && field.max > field.min
    ? ((numericValue - field.min) / (field.max - field.min)) * 100
    : 0;
  const clampedRangeFill = Math.max(0, Math.min(100, rangeFill));
  return (
    <div className={cn("min-w-0 rounded-lg border bg-slate-950/50 px-2.5 py-1.5 transition hover:border-slate-700", error ? "border-red-400/60" : "border-slate-800")}>
      {field.type === "boolean" ? (
        <button id={`provider-field-${field.name}`} type="button" role="switch" aria-checked={checked} aria-label={`${label}: ${checked ? t("enabled") : t("disabled")}`} disabled={disabled} className="flex min-h-7 w-full items-center justify-between gap-2.5 text-left outline-none transition disabled:opacity-50" onClick={() => onChange(field, !checked)}>
          <span className="text-xs font-semibold text-slate-200">{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</span>
          <span aria-hidden="true" className={cn("relative h-4 w-7 shrink-0 rounded-full transition-colors", checked ? "bg-panel-green" : "bg-slate-700")}>
            <span className={cn("absolute left-0.5 top-0.5 size-3 rounded-full bg-white transition-transform", checked ? "translate-x-3" : "translate-x-0")} />
          </span>
        </button>
      ) : isRangeSlider ? (
        <>
          <div className="mb-1 flex min-h-4 items-center">
            <label className="text-[11px] font-semibold text-slate-300" htmlFor={`provider-field-${field.name}`}>{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</label>
          </div>
          <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-end gap-2.5 text-[10px] tabular-nums text-slate-500">
          <span className="pb-0.5">{field.min}</span>
          <div className="relative min-w-0 pt-5" style={{ "--range-fill": `${clampedRangeFill}%` } as CSSProperties}>
            <output
              aria-hidden="true"
              className="pointer-events-none absolute top-0 min-w-6 -translate-x-1/2 rounded bg-slate-800 px-1 py-0.2 text-center text-[10px] font-bold tabular-nums text-slate-100"
              style={{ left: `clamp(1.25rem, ${clampedRangeFill}%, calc(100% - 1.25rem))` }}
            >
              {numericValue}
            </output>
            <input id={`provider-field-${field.name}`} aria-label={label} className="resource-range block w-full" type="range" min={field.min} max={field.max} step={field.step ?? 1} value={numericValue} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
          </div>
          <span className="pb-0.5">{field.max}</span>
          </div>
        </>
      ) : field.type === "select" ? (
        <LabeledControl field={field} label={label}>
          <select
            id={`provider-field-${field.name}`}
            className="h-8.5 w-full cursor-pointer rounded-lg border border-slate-800 bg-slate-900 px-2.5 text-xs text-slate-100 outline-none focus:border-panel-green"
            disabled={disabled}
            value={String(value ?? "")}
            onChange={(event) => onChange(field, event.target.value)}
          >
            {(field.options ?? []).map((option) => (
              <option key={option.value} value={option.value}>
                {providerOptionLabel(field, option.value, option.label, t)}
              </option>
            ))}
          </select>
        </LabeledControl>
      ) : field.type === "password" ? (
        <LabeledControl field={field} label={label}>
          <SecretInput id={`provider-field-${field.name}`} disabled={disabled} hideLabel={t("hideSensitiveValue", { label })} showLabel={t("showSensitiveValue", { label })} value={String(value ?? "")} onChange={(event) => onChange(field, event.target.value)} />
        </LabeledControl>
      ) : (
        <LabeledControl field={field} label={label}>
          <Input id={`provider-field-${field.name}`} className="h-8.5 w-full bg-slate-900 border-slate-800 text-xs px-2.5 focus:border-panel-green" type={field.type === "number" ? "number" : "text"} min={field.min} max={field.max} step={field.step ?? 1} value={field.type === "number" ? Number(value ?? 0) : String(value ?? "")} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
        </LabeledControl>
      )}
      {help ? <p className="mt-1 text-[10px] leading-tight text-slate-500">{help}</p> : null}
      {error ? <p className="mt-1 text-[10px] font-medium text-red-300">{error}</p> : null}
    </div>
  );
}

function LabeledControl({ children, field, label }: { children: ReactNode; field: ProviderConfigField; label: string }) {
  const { t } = useI18n();
  const isKleiServerToken = field.name === "clusterToken" || field.name === "identity.clusterToken";
  return (
    <>
      <div className="mb-1.5 flex min-h-4 items-center justify-between gap-2">
        <label className="text-[11px] font-semibold text-slate-300" htmlFor={`provider-field-${field.name}`}>{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</label>
        {isKleiServerToken ? (
          <a
            className="inline-flex shrink-0 items-center gap-1 text-[10px] text-sky-400 hover:underline"
            href={kleiServerTokenURL}
            rel="noreferrer"
            target="_blank"
          >
            {t("getKleiServerToken")}
            <ExternalLink aria-hidden="true" className="size-2.5" />
          </a>
        ) : null}
      </div>
      {children}
    </>
  );
}

function groupLabel(group: string, locale: "zh" | "en", t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  const dstKey = dstConfigGroupLabelKey(group);
  if (dstKey) return t(dstKey);
  if (locale === "en") {
    const labels: Record<string, string> = { "基础设置": "Basics", "世界倍率": "World rates", "据点与公会": "Bases and guilds", "生存与战斗": "Survival and combat", "功能开关": "Features" };
    return labels[group] ?? group;
  }
  return group;
}

function dstConfigGroupEffect(providerKey: string, group: string): "worldgen" | "worldsettings" | undefined {
  if (providerKey !== "dont-starve-together") return undefined;
  if (group.includes(".worldgen.")) return "worldgen";
  if (group.includes(".worldsettings.")) return "worldsettings";
  return undefined;
}
