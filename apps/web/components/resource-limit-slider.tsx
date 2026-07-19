"use client";

import type { CSSProperties } from "react";

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
      <p className="mb-2 text-sm font-medium text-slate-300">{label}</p>
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
        <span className="text-[11px] tabular-nums text-slate-500">{formatValue(0)}</span>
        <div className="relative min-w-0 pt-7" style={rangeStyle}>
          <output
            className="pointer-events-none absolute top-0 min-w-12 -translate-x-1/2 whitespace-nowrap rounded bg-slate-800 px-2 py-0.5 text-center text-xs font-semibold tabular-nums text-slate-100"
            style={{ left: `clamp(2rem, ${valuePercent}%, calc(100% - 2rem))` }}
          >
            {formatValue(clampedValue)}
          </output>
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
        <span className="text-[11px] tabular-nums text-slate-500">{formatValue(max)}</span>
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
