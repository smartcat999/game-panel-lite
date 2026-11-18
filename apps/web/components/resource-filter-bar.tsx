"use client";

import { ChevronDown, Search, SlidersHorizontal, X } from "lucide-react";
import { useState } from "react";
import { Input } from "@/components/ui";
import { useI18n, type MessageKey } from "@/lib/i18n";

export type ResourceFilterOption<T extends string = string> = {
  key: T;
  label?: string;
  labelKey?: MessageKey;
};

export type ResourceFilter<T extends string = string> = {
  label: string;
  onChange: (value: T) => void;
  options: readonly ResourceFilterOption<T>[];
  value: T;
};

type ResourceFilterBarProps = {
  activeChips?: string[];
  clearLabel: string;
  density?: "default" | "compact";
  filters: readonly ResourceFilter[];
  onClear: () => void;
  onSearchChange: (value: string) => void;
  search: string;
  searchPlaceholder: string;
};

export function ResourceFilterBar({
  activeChips = [],
  clearLabel,
  density = "default",
  filters,
  onClear,
  onSearchChange,
  search,
  searchPlaceholder
}: ResourceFilterBarProps) {
  const { locale, t } = useI18n();
  const [mobileExpanded, setMobileExpanded] = useState(false);
  const hasActiveFilters = search.trim().length > 0 || activeChips.length > 0 || filters.some((filter) => filter.value !== "all");
  const searchControlClass = density === "compact"
    ? "w-full min-w-0 sm:w-56 lg:w-64"
    : "w-full min-w-0 sm:w-64 lg:w-72";
  const filterControlClass = density === "compact"
    ? "relative h-9 w-full min-w-0 sm:w-36"
    : "relative h-9 w-full min-w-0 sm:w-40";

  const optionLabel = (filter: ResourceFilter, option: ResourceFilterOption) => {
    const label = option.labelKey ? t(option.labelKey) : option.label ?? option.key;
    if (option.key !== "all") return label;
    return locale === "zh" ? `${label}${filter.label}` : `${label} ${filter.label}`;
  };

  return (
    <section className="mb-4">
      <button
        type="button"
        aria-expanded={mobileExpanded}
        className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1.5 text-sm font-medium text-slate-200 md:hidden"
        onClick={() => setMobileExpanded((value) => !value)}
      >
        <span className="flex items-center gap-2">
          <SlidersHorizontal aria-hidden="true" className="size-4 text-panel-green" />
          {t("filters")}
          {activeChips.length > 0 ? <span className="rounded bg-panel-green/15 px-1.5 py-0.5 text-xs text-panel-green">{activeChips.length}</span> : null}
        </span>
        <ChevronDown aria-hidden="true" className={`size-4 text-slate-500 transition-transform ${mobileExpanded ? "rotate-180" : ""}`} />
      </button>
      <div className={`${mobileExpanded ? "flex" : "hidden"} mt-2.5 flex-col gap-2 md:mt-0 md:flex md:flex-row md:items-center`}>
        <label className={searchControlClass}>
          <span className="sr-only">{t("search")}</span>
          <span className="relative block h-9">
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
            <Input
              className="h-9 w-full bg-slate-950/55 pl-9 pr-8"
              placeholder={searchPlaceholder}
              value={search}
              onChange={(event) => onSearchChange(event.target.value)}
            />
            {search ? (
              <button
                type="button"
                aria-label={clearLabel}
                className="absolute right-2 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded text-slate-500 hover:bg-slate-800 hover:text-white"
                onClick={() => onSearchChange("")}
              >
                <X aria-hidden="true" className="size-3.5" />
              </button>
            ) : null}
          </span>
        </label>
        {filters.map((filter) => (
          <label key={filter.label} className={filterControlClass}>
            <span className="sr-only">{filter.label}</span>
            <select
              aria-label={filter.label}
              className="h-full w-full appearance-none rounded-md border border-panel-line bg-slate-950/55 px-3 pr-8 text-left text-sm font-medium text-slate-200 outline-none transition hover:border-slate-600 focus:border-panel-green focus:ring-1 focus:ring-panel-green/30"
              value={filter.value}
              onChange={(event) => filter.onChange(event.target.value)}
            >
              {filter.options.map((option) => (
                <option key={option.key} value={option.key}>
                  {optionLabel(filter, option)}
                </option>
              ))}
            </select>
            <ChevronDown aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
          </label>
        ))}
        <span className="hidden min-w-0 flex-1 md:block" />
        <button
            type="button"
            aria-label={clearLabel}
            title={clearLabel}
            className={`flex size-9 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-800 hover:text-white focus:outline-none focus-visible:ring-1 focus-visible:ring-panel-green ${hasActiveFilters ? "visible" : "invisible"}`}
            onClick={onClear}
          >
            <X aria-hidden="true" className="size-4" />
            <span className="sr-only">{clearLabel}</span>
          </button>
      </div>
    </section>
  );
}
