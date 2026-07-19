"use client";

import { RotateCcw, Search } from "lucide-react";
import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { SecretInput } from "@/components/secret-input";
import { Input } from "@/components/ui";
import { dstConfigGroupLabelKey } from "@/lib/dst-config";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { isAdvancedProviderConfigField, isProviderFieldModified, providerConfigValue, type ProviderConfigPayload } from "@/lib/provider-config";
import { providerOptionLabel } from "@/lib/provider-option-label";
import type { ProviderConfigField } from "@/lib/types";
import { cn } from "@/lib/utils";

type ConfigValue = string | boolean;

export function ProviderConfigEditor({
  disabled = false,
  errors = {},
  fieldHelp,
  fieldLabel,
  fields,
  onChange,
  payload,
  providerKey
}: {
  disabled?: boolean;
  errors?: Record<string, string>;
  fieldHelp: (field: ProviderConfigField) => string;
  fieldLabel: (field: ProviderConfigField) => string;
  fields: ProviderConfigField[];
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  payload: ProviderConfigPayload;
  providerKey: string;
}) {
  const { locale, t } = useI18n();
  const [activeView, setActiveView] = useState<"basic" | "advanced">("basic");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<"all" | "modified">("all");

  const baseFields = useMemo(() => fields.filter((field) => !isAdvancedProviderConfigField(providerKey, field)), [fields, providerKey]);
  const cavesEnabled = providerConfigValue(payload, "caves.enabled") === true;
  const advancedFields = useMemo(
    () => fields.filter((field) => isAdvancedProviderConfigField(providerKey, field) && (cavesEnabled || !field.name.startsWith("caves.overrides."))),
    [cavesEnabled, fields, providerKey]
  );
  const modifiedCount = advancedFields.filter((field) => isProviderFieldModified(payload, field)).length;
  const groups = useMemo(() => Array.from(new Set(advancedFields.map((field) => field.group).filter((group): group is string => Boolean(group)))), [advancedFields]);
  const [activeGroup, setActiveGroup] = useState(groups[0] ?? "");

  useEffect(() => {
    if (!groups.includes(activeGroup)) setActiveGroup(groups[0] ?? "");
  }, [activeGroup, groups]);

  useEffect(() => {
    setActiveView("basic");
    setQuery("");
    setFilter("all");
  }, [providerKey]);

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
      {advancedFields.length > 0 ? (
        <div role="tablist" aria-label={t("gameSettingsViews")} className="inline-flex rounded-md border border-panel-line bg-slate-950/50 p-1">
          {(["basic", "advanced"] as const).map((view) => (
            <button
              key={view}
              type="button"
              role="tab"
              aria-selected={activeView === view}
              className={cn("flex h-8 items-center gap-2 rounded px-3 text-xs font-medium transition", activeView === view ? "bg-slate-800 text-slate-100" : "text-slate-500 hover:text-slate-300")}
              onClick={() => setActiveView(view)}
            >
              {t(view === "basic" ? "basicGameSettings" : "advancedGameSettings")}
              {view === "advanced" && modifiedCount > 0 ? (
                <span aria-label={t("modifiedSettingsCount", { count: modifiedCount })} className="min-w-5 rounded bg-panel-green/15 px-1.5 py-0.5 text-center text-[10px] tabular-nums text-panel-green">{modifiedCount}</span>
              ) : null}
            </button>
          ))}
        </div>
      ) : null}

      {activeView === "advanced" && advancedFields.length > 0 ? (
        <div className="min-w-0">
          <div className="flex flex-wrap items-center justify-end gap-2 pb-3">
            <label className="relative min-w-52 flex-1 sm:max-w-72">
              <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
              <Input className="w-full pl-9" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t("searchGameSettings")} />
            </label>
            <div className="flex rounded-md border border-panel-line bg-slate-950 p-0.5">
              {(["all", "modified"] as const).map((value) => (
                <button key={value} type="button" className={cn("rounded px-2.5 py-1.5 text-xs font-medium transition", filter === value ? "bg-slate-800 text-slate-100" : "text-slate-500 hover:text-slate-300")} onClick={() => setFilter(value)}>
                  {t(value === "all" ? "allSettings" : "modifiedSettings")}
                </button>
              ))}
            </div>
          </div>

          <div className="border-t border-panel-line lg:grid lg:grid-cols-[190px_minmax(0,1fr)]">
            <nav aria-label={t("settingsCategories")} className="flex gap-1 overflow-x-auto border-b border-panel-line py-2 lg:block lg:border-b-0 lg:border-r lg:pr-3">
              {groups.map((group) => {
                const count = advancedFields.filter((field) => field.group === group && isProviderFieldModified(payload, field)).length;
                return (
                  <button key={group} type="button" className={cn("flex shrink-0 items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-xs transition lg:mb-1 lg:w-full", activeGroup === group && !normalizedQuery ? "bg-slate-800 text-slate-100" : "text-slate-500 hover:bg-slate-900 hover:text-slate-300")} onClick={() => { setQuery(""); setActiveGroup(group); }}>
                    <span>{groupLabel(group, locale, t)}</span>
                    {count > 0 ? <span className="text-panel-green">{count}</span> : null}
                  </button>
                );
              })}
            </nav>
            <div className="min-w-0 py-3 lg:pl-4">
              {visibleGroups.map((group) => {
                const groupFields = matchedFields.filter((field) => field.group === group);
                if (groupFields.length === 0) return null;
                return (
                  <section key={group} className="mb-6 last:mb-0">
                    <div className="mb-3 flex items-center justify-between gap-3">
                      <h5 className="text-sm font-semibold text-slate-200">{groupLabel(group, locale, t)}</h5>
                      <span className="text-xs text-slate-500">{t("settingsCount", { count: groupFields.length })}</span>
                    </div>
                    <div className="grid gap-3 xl:grid-cols-2">
                      {groupFields.map((field) => (
                        <ConfigField key={field.name} disabled={disabled} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} resettable slider={providerKey === "palworld"} />
                      ))}
                    </div>
                  </section>
                );
              })}
              {matchedFields.length === 0 ? <p className="py-10 text-center text-sm text-slate-500">{t("noGameSettingsMatch")}</p> : null}
            </div>
          </div>
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {baseFields.map((field) => (
            <ConfigField key={field.name} disabled={disabled} error={errors[field.name]} field={field} help={fieldHelp(field)} label={fieldLabel(field)} onChange={onChange} payload={payload} slider={providerKey === "palworld"} />
          ))}
        </div>
      )}
    </div>
  );
}

function ConfigField({ disabled, error, field, help, label, onChange, payload, resettable = false, slider }: {
  disabled: boolean;
  error?: string;
  field: ProviderConfigField;
  help: string;
  label: string;
  onChange: (field: ProviderConfigField, value: ConfigValue) => void;
  payload: ProviderConfigPayload;
  resettable?: boolean;
  slider: boolean;
}) {
  const { t } = useI18n();
  const value = providerConfigValue(payload, field.name);
  const modified = isProviderFieldModified(payload, field);
  const checked = value === true;
  const numericValue = Number(value ?? field.default ?? 0);
  const isRangeSlider = field.type === "number" && slider && field.min !== undefined && field.max !== undefined;
  const rangeFill = field.min !== undefined && field.max !== undefined && field.max > field.min
    ? ((numericValue - field.min) / (field.max - field.min)) * 100
    : 0;
  const clampedRangeFill = Math.max(0, Math.min(100, rangeFill));
  const reset = () => onChange(field, field.type === "boolean" ? field.default === true : String(field.default ?? (field.type === "number" ? 0 : "")));
  return (
    <div className={cn("min-w-0 rounded-md border bg-slate-950/35 p-3", error ? "border-red-400/60" : "border-panel-line")}>
      <div className="mb-2 flex min-h-5 items-start justify-between gap-3">
        <label className="text-xs font-medium text-slate-400" htmlFor={`provider-field-${field.name}`}>{label}{field.required ? <span className="ml-1 text-panel-gold">*</span> : null}</label>
        {resettable && modified ? (
          <button type="button" aria-label={t("restoreDefault")} title={t("restoreDefault")} className="flex size-7 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-800 hover:text-panel-green focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/40 disabled:opacity-50" onClick={reset} disabled={disabled}>
            <RotateCcw aria-hidden="true" className="size-3.5" />
          </button>
        ) : null}
      </div>
      {field.type === "boolean" ? (
        <button id={`provider-field-${field.name}`} type="button" role="switch" aria-checked={checked} disabled={disabled} className="flex w-full items-center justify-between rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-sm text-slate-200 outline-none transition focus-visible:border-panel-green focus-visible:ring-2 focus-visible:ring-panel-green/30 disabled:opacity-50" onClick={() => onChange(field, !checked)}>
          <span>{checked ? t("enabled") : t("disabled")}</span>
          <span aria-hidden="true" className={cn("relative h-5 w-9 shrink-0 rounded-full transition-colors", checked ? "bg-panel-green" : "bg-slate-700")}>
            <span className={cn("absolute left-0.5 top-0.5 size-4 rounded-full bg-white transition-transform", checked ? "translate-x-4" : "translate-x-0")} />
          </span>
        </button>
      ) : isRangeSlider ? (
        <div className="relative space-y-1.5 pt-7" style={{ "--range-fill": `${clampedRangeFill}%` } as CSSProperties}>
          <output
            aria-hidden="true"
            className="pointer-events-none absolute top-0 min-w-8 -translate-x-1/2 rounded bg-slate-800 px-1.5 py-0.5 text-center text-xs font-semibold tabular-nums text-slate-100"
            style={{ left: `clamp(1.25rem, ${clampedRangeFill}%, calc(100% - 1.25rem))` }}
          >
            {numericValue}
          </output>
          <input id={`provider-field-${field.name}`} aria-label={label} className="resource-range w-full" type="range" min={field.min} max={field.max} step={field.step ?? 1} value={numericValue} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
          <div className="flex justify-between text-[11px] tabular-nums text-slate-600">
            <span>{field.min}</span>
            <span>{field.max}</span>
          </div>
        </div>
      ) : field.type === "select" ? (
        <select id={`provider-field-${field.name}`} className="h-10 w-full rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green" disabled={disabled} value={String(value ?? "")} onChange={(event) => onChange(field, event.target.value)}>
          {(field.options ?? []).map((option) => <option key={option.value} value={option.value}>{providerOptionLabel(field, option.value, option.label, t)}</option>)}
        </select>
      ) : field.type === "password" ? (
        <SecretInput disabled={disabled} hideLabel={t("hideSensitiveValue", { label })} showLabel={t("showSensitiveValue", { label })} value={String(value ?? "")} onChange={(event) => onChange(field, event.target.value)} />
      ) : (
        <Input id={`provider-field-${field.name}`} className="w-full" type={field.type === "number" ? "number" : "text"} min={field.min} max={field.max} step={field.step ?? 1} value={field.type === "number" ? Number(value ?? 0) : String(value ?? "")} disabled={disabled} onChange={(event) => onChange(field, event.target.value)} />
      )}
      {help ? <p className="mt-2 text-xs leading-5 text-slate-500">{help}</p> : null}
      {error ? <p className="mt-2 text-xs font-medium text-red-200">{error}</p> : null}
    </div>
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
