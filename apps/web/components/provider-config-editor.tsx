"use client";

import { ExternalLink, RotateCcw, Search, Shuffle } from "lucide-react";
import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import { createSevenDaysWorldSeed, isSevenDaysFieldVisible, sevenDaysConfigSection, type SevenDaysConfigSection } from "@/lib/seven-days-config";
import { SevenDaysSandboxEditor } from "@/components/seven-days-sandbox-editor";
import { ConfirmDialog } from "@/components/confirm-dialog";
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
  const worldIdentityFields = new Set(["worldName", "gameWorld", "worldGenSeed", "worldGenSize"]);
  const fieldDisabled = (field: ProviderConfigField) => disabled || (surface === "server" && providerKey === "seven-days-to-die" && worldIdentityFields.has(field.name));
  const [activeShard, setActiveShard] = useState<"master" | "caves">("master");

  const baseFields = useMemo(() => fields.filter((field) => !isAdvancedProviderConfigField(providerKey, field) && (providerKey !== "seven-days-to-die" || isSevenDaysFieldVisible(field, payload))), [fields, providerKey, payload]);
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

  if (providerKey === "seven-days-to-die") {
    return <SevenDaysConfigEditor disabled={disabled} errors={errors} fields={fields} onChange={onChange} onRestoreDefaults={onRestoreDefaults} payload={payload} surface={surface} />;
  }


  return (
    <div className="mt-4 space-y-4 min-w-0 w-full">
      {allAdvancedFields.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2.5 pb-1 min-w-0 w-full">
          {/* Left: View Switcher (Basic / Advanced) */}
          <div role="tablist" aria-label={t("gameSettingsViews")} className="inline-flex rounded-lg border border-panel-line bg-slate-950/80 p-0.5 shadow-sm shrink-0">
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
            <div className="inline-flex items-center rounded-lg border border-panel-line bg-slate-950/80 p-0.5 shadow-sm shrink-0">
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
        <div className="min-w-0 rounded-lg border border-panel-line bg-slate-950/40 p-2.5 sm:p-3 w-full overflow-hidden">
          {/* Top-right action toolbar (Unified, compact, gamer-aesthetic) */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 pb-3 border-b border-panel-line/70 min-w-0 w-full">
            <div className="flex items-center gap-2 min-w-0">
              <span className="text-xs font-medium text-slate-300 truncate">
                {isDst
                  ? activeShard === "master"
                    ? t("dstShardMaster")
                    : t("dstShardCaves")
                  : t("advancedGameSettings")}
              </span>
              {currentShardModifiedCount > 0 && (
                <span className="rounded-full bg-panel-green/15 border border-panel-green/30 px-2 py-0.5 text-[10px] font-semibold text-panel-green font-mono shrink-0">
                  {currentShardModifiedCount} {locale.startsWith("zh") ? "项已更改" : "modified"}
                </span>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2 min-w-0 w-full sm:w-auto">
              {/* Search */}
              <div className="relative flex-1 min-w-[130px] sm:w-56 sm:flex-initial">
                <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-slate-500" />
                <Input
                  className="h-7 w-full pl-8 text-xs bg-slate-900 border-panel-line"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("searchGameSettings")}
                />
              </div>

              {/* All / Modified Pill */}
              <div className="inline-flex rounded-md border border-panel-line bg-slate-900 p-0.5 shrink-0">
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
                  className="h-7 px-2 text-xs text-slate-400 hover:text-white shrink-0"
                  disabled={disabled}
                  onClick={() => onRestoreDefaults(advancedFields)}
                >
                  <RotateCcw aria-hidden="true" className="size-3 text-slate-400" />
                  <span>{t("restoreDefault")}</span>
                </Button>
              )}
            </div>
          </div>

          <div className="pt-3 lg:grid lg:grid-cols-[160px_minmax(0,1fr)] min-w-0 w-full">
            <nav aria-label={t("settingsCategories")} className="flex gap-1 overflow-x-auto border-b border-panel-line py-1.5 lg:block lg:border-b-0 lg:border-r lg:pr-2.5 min-w-0 w-full no-scrollbar">
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
                      <span className="text-[10px] font-bold text-panel-green bg-panel-green/15 px-1.5 py-0.2 rounded shrink-0">
                        {count}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </nav>
            <div className="min-w-0 py-2 lg:pl-3.5 w-full">
              {visibleGroups.map((group) => {
                const groupFields = matchedFields.filter((field) => field.group === group);
                const effect = dstConfigGroupEffect(providerKey, group);
                if (groupFields.length === 0) return null;
                return (
                  <section key={group} className="mb-4 last:mb-0 min-w-0 w-full">
                    <div className="mb-2 flex items-center justify-between gap-2 min-w-0">
                      <div className="flex min-w-0 flex-wrap items-center gap-2">
                        <h5 className="text-xs font-semibold text-slate-200 truncate">{groupLabel(group, locale, t)}</h5>
                        {surface === "server" && effect ? (
                          <span className={cn(
                            "rounded px-1.5 py-0.2 text-[10px] font-medium shrink-0",
                            effect === "worldgen" ? "bg-panel-gold/15 text-panel-gold" : "bg-slate-800 text-slate-400"
                          )}>
                            {t(effect === "worldgen" ? "worldGenerationAppliesOnRegenerate" : "worldSettingsApplyAfterRestart")}
                          </span>
                        ) : null}
                      </div>
                      <span className="text-[10px] text-slate-500 font-mono shrink-0">{t("settingsCount", { count: groupFields.length })}</span>
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
                        <ConfigField key={field.name} disabled={fieldDisabled(field)} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} sevenDays={providerKey === "seven-days-to-die"} />
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
              <ConfigField key={field.name} disabled={fieldDisabled(field)} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} sevenDays={providerKey === "seven-days-to-die"} />
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
                  <ConfigField key={field.name} disabled={fieldDisabled(field)} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} sevenDays={providerKey === "seven-days-to-die"} />
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function SevenDaysConfigEditor({ disabled, errors, fields, onChange, onRestoreDefaults, payload, surface }: {
  disabled: boolean;
  errors: Record<string, string>;
  fields: ProviderConfigField[];
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  onRestoreDefaults?: (fields: ProviderConfigField[]) => void;
  payload: ProviderConfigPayload;
  surface: "create" | "server";
}) {
  const { locale } = useI18n();
  const [section, setSection] = useState<SevenDaysConfigSection>("basic");
  const [resetChange, setResetChange] = useState<{ field: ProviderConfigField; value: ConfigValue } | null>(null);
  const isZh = locale === "zh";
  const sections: { key: SevenDaysConfigSection; label: string }[] = [
    { key: "basic", label: isZh ? "基础" : "Basic" },
    { key: "server", label: isZh ? "服务器" : "Server" },
    { key: "performance", label: isZh ? "高级" : "Advanced" },
    { key: "landClaims", label: isZh ? "多人" : "Multiplayer" }
  ];
  const sectionFields = fields.filter((field) => sevenDaysConfigSection(field) === section)
    .sort((a, b) => Number(a.group === "sevenDays.performance") - Number(b.group === "sevenDays.performance"));
  const visibleFields = sectionFields.filter((field) => isSevenDaysFieldVisible(field, payload) || Boolean(errors[field.name]));
  const hasChanges = sectionFields.some((field) => isProviderFieldModified(payload, field));
  const changeField = (field: ProviderConfigField, value: ConfigValue) => {
    if (field.name === "maxChunkAge" && Number(value) >= 0) {
      setResetChange({ field, value });
      return;
    }
    onChange(field, value);
  };
  return (
    <div className="mt-3 min-w-0 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-panel-line pb-2">
        <nav aria-label={isZh ? "七日杀设置分类" : "Seven Days settings categories"} className="flex min-w-0 flex-wrap gap-1">
          {sections.map(({ key, label }) => {
            const hasErrors = fields.some((field) => sevenDaysConfigSection(field) === key && Boolean(errors[field.name]));
            return <button key={key} type="button" aria-pressed={section === key} onClick={() => setSection(key)} className={cn("inline-flex min-h-9 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-panel-green", section === key ? "bg-slate-800 text-slate-100" : "text-slate-400 hover:text-white")}>
              {label}
              <span aria-hidden="true" className="text-[10px] tabular-nums text-slate-500">{fields.filter((field) => sevenDaysConfigSection(field) === key && (isSevenDaysFieldVisible(field, payload) || Boolean(errors[field.name]))).length}</span>
              {hasErrors ? <span className="size-1.5 rounded-full bg-red-400" aria-label={isZh ? "有待修正的参数" : "Contains invalid settings"} /> : null}
            </button>;
          })}
        </nav>
        {section !== "basic" && onRestoreDefaults && hasChanges ? <button type="button" disabled={disabled} className="text-xs text-slate-400 hover:text-white disabled:opacity-40" onClick={() => onRestoreDefaults(sectionFields)}>{isZh ? "恢复本组默认" : "Reset section"}</button> : null}
      </div>
      <div className={cn("grid grid-cols-1 gap-2 sm:grid-cols-2", section !== "basic" && "xl:grid-cols-3")}>
        {visibleFields.map((field) => <ConfigField key={field.name} disabled={disabled || (surface === "server" && ["worldName", "gameWorld", "worldGenSeed", "worldGenSize"].includes(field.name))} error={errors[field.name]} field={field} help={(isZh ? field.help : field.helpEn) ?? ""} label={(isZh ? field.label : field.labelEn) ?? field.label} onChange={changeField} payload={payload} slider={false} sevenDays />)}
      </div>
      {section === "basic" && (surface === "server" || payload.gameWorld !== "RWG") ? <p className="text-xs text-slate-400">{isZh
        ? `${surface === "server" ? "存档与地图标识只读，避免切换已有世界。" : ""}${payload.gameWorld !== "RWG" ? "随机种子和尺寸仅在随机生成地图时显示。" : ""}`
        : `${surface === "server" ? "Save and world identity are read-only to preserve the existing world. " : ""}${payload.gameWorld !== "RWG" ? "Seed and size are shown only for random worlds." : ""}`}</p> : null}
      <ConfirmDialog eyebrow={isZh ? "世界数据" : "World data"} open={Boolean(resetChange)} title={isZh ? "启用区块重置？" : "Enable chunk reset?"} description={isZh ? "长时间未访问且未受保护的区块会重置，建造内容可能丢失。确认后仍需保存配置并重启服务器。" : "Unvisited, unprotected chunks will reset and builds may be lost. Save settings and restart to apply."} confirmLabel={isZh ? "确认设置" : "Confirm setting"} cancelLabel={isZh ? "取消" : "Cancel"} confirmVariant="danger" onCancel={() => setResetChange(null)} onConfirm={() => { if (resetChange) onChange(resetChange.field, resetChange.value); setResetChange(null); }} />
      {section === "landClaims" ? <p className="text-xs text-slate-400">{isZh ? "耐久倍率针对非盟友玩家的破坏；0 表示免疫此类伤害。" : "Durability multipliers protect against non-allied player damage; 0 prevents that damage."}</p> : null}
    </div>
  );
}

function ConfigField({ disabled, error, field, help, label, onChange, payload, slider, sevenDays = false }: {
  disabled: boolean;
  error?: string;
  field: ProviderConfigField;
  help: string;
  label: string;
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  payload: ProviderConfigPayload;
  slider: boolean;
  sevenDays?: boolean;
}) {
  const { locale, t } = useI18n();
  if (sevenDays) label = field.label;
  if (locale === "en") {
    label = field.labelEn ?? label;
    help = field.helpEn ?? help;
  }
  const value = providerConfigValue(payload, field.name) ?? (sevenDays ? field.default : undefined);
  const checked = value === true;
  const numericValue = Number(value ?? field.default ?? 0);
  const isRangeSlider = field.type === "number" && slider && field.min !== undefined && field.max !== undefined;
  const rangeFill = field.min !== undefined && field.max !== undefined && field.max > field.min
    ? ((numericValue - field.min) / (field.max - field.min)) * 100
    : 0;
  const clampedRangeFill = Math.max(0, Math.min(100, rangeFill));
  return (
    <div className={cn("min-w-0 w-full rounded-lg border bg-slate-950/50 px-2.5 py-1.5 transition hover:border-slate-700 overflow-hidden", sevenDays && field.name === "sandboxCode" && "col-span-full", error ? "border-red-400/60" : "border-slate-800")}>
      {field.type === "boolean" ? (
        <button id={`provider-field-${field.name}`} type="button" role="switch" aria-checked={checked} aria-label={`${label}: ${checked ? t("enabled") : t("disabled")}`} disabled={disabled} className={cn("flex w-full items-center justify-between gap-2.5 text-left outline-none transition disabled:opacity-50", sevenDays ? "min-h-[62px]" : "min-h-7")} onClick={() => onChange(field, !checked)}>
          <span className="text-xs font-semibold text-slate-200 truncate">{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</span>
          <span aria-hidden="true" className={cn("relative h-4 w-7 shrink-0 rounded-full transition-colors", checked ? "bg-panel-green" : "bg-slate-700")}>
            <span className={cn("absolute left-0.5 top-0.5 size-3 rounded-full bg-white transition-transform", checked ? "translate-x-3" : "translate-x-0")} />
          </span>
        </button>
      ) : isRangeSlider ? (
        <>
          <div className="mb-1 flex min-h-4 items-center">
            <label className="text-[11px] font-semibold text-slate-300 truncate" htmlFor={`provider-field-${field.name}`}>{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</label>
          </div>
          <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-end gap-2 text-[10px] tabular-nums text-slate-500 min-w-0 w-full">
            <span className="pb-0.5 shrink-0">{field.min}</span>
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
            <span className="pb-0.5 shrink-0">{field.max}</span>
          </div>
        </>
      ) : field.type === "select" || (sevenDays && Boolean(field.options?.length)) ? (
        <LabeledControl field={field} label={label}>
          <select
            id={`provider-field-${field.name}`}
            className={cn("w-full cursor-pointer rounded-lg border border-slate-800 bg-slate-900 px-2.5 text-xs text-slate-100 outline-none focus:border-panel-green", sevenDays ? "h-10" : "h-8.5")}
            disabled={disabled}
            value={String(value ?? "")}
            onChange={(event) => onChange(field, event.target.value)}
          >
            {(disabled || sevenDays) && value !== undefined && !field.options?.some((option) => option.value === String(value)) ? <option value={String(value)}>{String(value)}</option> : null}
            {(field.options ?? []).map((option) => (
              <option key={option.value} value={option.value}>
                {locale === "en" && option.labelEn ? option.labelEn : providerOptionLabel(field, option.value, option.label, t)}
              </option>
            ))}
          </select>
        </LabeledControl>
      ) : sevenDays && field.name === "worldGenSeed" ? (
        <LabeledControl field={field} label={label}>
          <div className="flex items-center gap-1.5">
            <Input id={`provider-field-${field.name}`} className="h-8.5 min-w-0 bg-slate-900 px-2.5 text-xs" value={String(value ?? "")} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
            <button type="button" disabled={disabled} onClick={() => onChange(field, createSevenDaysWorldSeed())} title={locale === "zh" ? "重新随机种子" : "Randomize seed"} aria-label={locale === "zh" ? "重新随机种子" : "Randomize seed"} className="flex size-10 shrink-0 items-center justify-center rounded-md border border-slate-700 text-slate-300 hover:text-white disabled:opacity-40">
              <Shuffle aria-hidden="true" className="size-3.5" />
            </button>
          </div>
        </LabeledControl>
      ) : sevenDays && field.name === "sandboxCode" ? (
        <LabeledControl field={field} label={label}>
          <div className="flex flex-wrap items-center gap-2">
            <SevenDaysSandboxEditor code={String(value ?? "")} disabled={disabled} onChange={(code) => onChange(field, code)} />
            <Input id={`provider-field-${field.name}`} className="h-8.5 min-w-0 flex-1 basis-40 bg-slate-900 px-2.5 font-mono text-xs" value={String(value ?? "")} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
            <button type="button" disabled={disabled || value === field.default} onClick={() => onChange(field, String(field.default ?? ""))} className="shrink-0 text-xs text-slate-300 hover:text-white disabled:opacity-40">
              {locale === "zh" ? "恢复默认" : "Reset"}
            </button>
          </div>
          <a className="mt-1.5 inline-flex items-center gap-1 text-xs text-sky-400 hover:underline" href="https://7d2d.net/7-days-to-die-sandbox-code-generator" target="_blank" rel="noreferrer" title={locale === "zh" ? "第三方工具，请使用与服务器相同的游戏版本生成代码" : "Third-party tool. Generate codes for the same game version as your server."}>
            {locale === "zh" ? "网页生成工具（第三方）" : "Web generator (third-party)"}
            <ExternalLink aria-hidden="true" className="size-3" />
          </a>
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
      {help ? <p className="mt-1 text-[10px] leading-tight text-slate-500 break-words">{help}</p> : null}
      {error ? <p className="mt-1 text-[10px] font-medium text-red-300 break-words">{error}</p> : null}
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
