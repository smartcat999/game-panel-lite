"use client";

import type { CSSProperties } from "react";
import { cn } from "../lib/utils";

export type ResourceLimitMarker = {
  label: string;
  tone?: "default" | "recommended";
  value: number;
};

export function ResourceLimitSlider({
  disabled = false,
  formatValue,
  label,
  markers = [],
  max,
  onChange,
  step,
  value
}: {
  disabled?: boolean;
  formatValue: (value: number) => string;
  label: string;
  markers?: ResourceLimitMarker[];
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
        <div className="relative min-w-0 pb-7 pt-7" style={rangeStyle}>
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
          <div aria-hidden="true" className="absolute inset-x-0 top-[3.15rem] h-5">
            {markers
              .filter((marker) => marker.value > 0 && marker.value < max)
              .map((marker) => {
                const left = (marker.value / max) * 100;
                return (
                  <span key={`${marker.value}-${marker.label}`} className="absolute top-0 -translate-x-1/2" style={{ left: `${left}%` }}>
                    <span className={cn("mx-auto block h-1.5 w-px", marker.tone === "recommended" ? "bg-panel-gold" : "bg-slate-600")} />
                    <span className={cn("mt-1 block whitespace-nowrap text-[10px] tabular-nums", marker.tone === "recommended" ? "text-panel-gold" : "text-slate-600")}>
                      {marker.label}
                    </span>
                  </span>
                );
              })}
          </div>
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

export function cpuResourceMarkers(max: number): ResourceLimitMarker[] {
  const values = max <= 16
    ? Array.from({ length: Math.max(0, Math.ceil(max) - 1) }, (_, index) => index + 1)
    : [1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48];
  return values
    .filter((value) => value < max)
    .map((value) => ({ label: String(value), value }));
}

export function memoryResourceMarkers(max: number, recommended = 0, recommendedLabel = ""): ResourceLimitMarker[] {
  const maxGigabytes = Math.floor(max / 1024);
  const gigabytes = maxGigabytes <= 16
    ? Array.from({ length: Math.max(0, maxGigabytes - 1) }, (_, index) => index + 1)
    : [1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64];
  const markers: ResourceLimitMarker[] = gigabytes
    .map((value) => value * 1024)
    .filter((value) => value < max && value !== recommended)
    .map((value) => ({ label: `${value / 1024}G`, value }));
  if (recommended > 0 && recommended < max) {
    markers.push({ label: `${recommendedLabel} ${recommended / 1024}G`, tone: "recommended", value: recommended });
  }
  return markers.sort((left, right) => left.value - right.value);
}
