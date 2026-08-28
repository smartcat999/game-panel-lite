"use client";

import type { CSSProperties } from "react";
import { cn } from "../lib/utils";

export function ResourceLimitSlider({
  disabled = false,
  formatValue,
  label,
  max,
  onChange,
  step,
  value
}: {
  disabled?: boolean;
  formatValue: (value: number) => string;
  label: string;
  max: number;
  onChange: (value: number) => void;
  step: number;
  value: number;
}) {
  const clampedValue = Math.max(0, Math.min(value, max));
  const valuePercent = max > 0 ? (clampedValue / max) * 100 : 0;
  const rangeStyle = { "--range-fill": `${valuePercent}%` } as CSSProperties;

  return (
    <div className="min-w-0">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-slate-200">{label}</span>
        <span className={cn(
          "rounded-md px-2 py-0.5 text-xs font-mono font-bold",
          clampedValue === 0
            ? "bg-slate-800 text-slate-300 border border-slate-700"
            : "bg-panel-green/15 text-panel-green border border-panel-green/30"
        )}>
          {formatValue(clampedValue)}
        </span>
      </div>
      <div className="grid grid-cols-[3rem_minmax(0,1fr)_3.5rem] items-center gap-2.5">
        <span className="text-[10px] tabular-nums text-slate-500">{formatValue(0)}</span>
        <div className="relative min-w-0" style={rangeStyle}>
          <input
            aria-label={label}
            className="resource-range block w-full cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
            disabled={disabled}
            max={max}
            min={0}
            step={step}
            type="range"
            value={clampedValue}
            onChange={(event) => onChange(Number(event.target.value))}
          />
        </div>
        <span className="text-right text-[10px] tabular-nums text-slate-500">{formatValue(max)}</span>
      </div>
    </div>
  );
}

export function formatCpuResourceLimit(value: number, unlimitedLabel: string, unit: string): string {
  if (value === 0) return unlimitedLabel;
  return `${Number.isInteger(value) ? value : value.toFixed(2).replace(/0+$/, "").replace(/\.$/, "")} ${unit}`;
}

export function formatMemoryResourceLimit(value: number, unlimitedLabel: string): string {
  if (value === 0) return unlimitedLabel;
  if (value >= 1024) {
    const gigabytes = value / 1024;
    return `${Number.isInteger(gigabytes) ? gigabytes : gigabytes.toFixed(2).replace(/0+$/, "").replace(/\.$/, "")} GB`;
  }
  return `${value} MB`;
}
