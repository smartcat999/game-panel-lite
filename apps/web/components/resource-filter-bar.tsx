"use client";

import { Search, X } from "lucide-react";
import { Button, Card, Input } from "@/components/ui";
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
  resultLabel?: string;
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
  resultLabel,
  search,
  searchPlaceholder
}: ResourceFilterBarProps) {
  const { t } = useI18n();
  const hasActiveFilters = search.trim().length > 0 || activeChips.length > 0 || filters.some((filter) => filter.value !== "all");
  const searchControlClass = density === "compact"
    ? "grid w-full min-w-0 gap-1.5 sm:w-72 lg:w-80"
    : "grid w-full min-w-0 gap-1.5 sm:w-80 2xl:w-[22rem]";
  const filterControlClass = density === "compact"
    ? "grid w-full min-w-0 gap-1.5 sm:w-44 lg:w-48"
    : searchControlClass;

  return (
    <Card className="mb-4 p-3">
      <div className="flex flex-wrap items-end gap-3">
        <label className={searchControlClass}>
          <span className="text-xs font-medium text-slate-500">{t("search")}</span>
          <span className="relative block">
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
            <Input
              className="w-full pl-9"
              placeholder={searchPlaceholder}
              value={search}
              onChange={(event) => onSearchChange(event.target.value)}
            />
          </span>
        </label>
        {filters.map((filter) => (
          <label key={filter.label} className={filterControlClass}>
            <span className="text-xs font-medium text-slate-500">{filter.label}</span>
            <select
              className="h-10 w-full rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm font-medium text-slate-100 outline-none transition focus:border-panel-green focus:ring-2 focus:ring-panel-green/20"
              value={filter.value}
              onChange={(event) => filter.onChange(event.target.value)}
            >
              {filter.options.map((option) => (
                <option key={option.key} value={option.key}>
                  {option.labelKey ? t(option.labelKey) : option.label ?? option.key}
                </option>
              ))}
            </select>
          </label>
        ))}
      </div>
      <div className="mt-3 flex min-h-7 flex-wrap items-center justify-between gap-2 border-t border-panel-line pt-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          {resultLabel ? <span className="text-xs text-slate-500">{resultLabel}</span> : null}
          {activeChips.map((chip) => (
            <span key={chip} className="max-w-[14rem] truncate rounded border border-panel-line bg-slate-950/50 px-2 py-1 text-xs text-slate-300">
              {chip}
            </span>
          ))}
        </div>
        {hasActiveFilters ? (
          <Button
            type="button"
            variant="ghost"
            className="h-7 px-2 text-xs"
            onClick={onClear}
          >
            <X aria-hidden="true" className="size-3.5" />
            {clearLabel}
          </Button>
        ) : null}
      </div>
    </Card>
  );
}
