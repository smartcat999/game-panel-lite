"use client";

import ReactECharts from "echarts-for-react";
import type { EChartsOption } from "echarts";
import { graphic } from "echarts";
import type { MetricSeries } from "@/features/monitoring/types";

export type ServerStatusDatum = {
  color: string;
  label: string;
  value: number;
  subLabel?: string;
  icon?: React.ReactNode;
};

export function ServerStatusKpis({ data, hint }: { data: ServerStatusDatum[]; hint?: string }) {
  return (
    <div className="min-w-0">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {data.map((item) => (
          <div
            className="group relative min-w-0 overflow-hidden rounded-xl border border-slate-800/80 bg-gradient-to-b from-slate-900/80 via-slate-900/40 to-slate-950/80 p-4 transition-all duration-200 hover:border-slate-700 hover:bg-slate-900 hover:shadow-xl hover:shadow-black/60"
            key={item.label}
          >
            {/* Top glowing edge line */}
            <div
              className="absolute inset-x-0 top-0 h-[1.5px] opacity-40 transition-opacity group-hover:opacity-100"
              style={{ background: `linear-gradient(90deg, transparent, ${item.color}, transparent)` }}
            />

            <div className="flex min-w-0 items-center justify-between gap-2">
              <dt className="flex min-w-0 items-center gap-2 text-xs font-semibold text-slate-400">
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ backgroundColor: item.color, boxShadow: `0 0 10px ${item.color}` }}
                />
                <span className="truncate">{item.label}</span>
              </dt>
              {item.icon ? <div className="text-slate-500 transition-colors group-hover:text-slate-200">{item.icon}</div> : null}
            </div>
            <dd className="mt-3 flex items-baseline gap-2">
              <span className="font-mono text-2xl font-black tabular-nums tracking-tight text-white sm:text-3xl">
                {item.value.toLocaleString()}
              </span>
              {item.subLabel ? <span className="text-xs font-medium text-slate-400 truncate">{item.subLabel}</span> : null}
            </dd>
          </div>
        ))}
      </dl>
      {hint ? <p className="mt-2 text-xs text-slate-500">{hint}</p> : null}
    </div>
  );
}

export function ResourceTrendChart({
  emptyLabel,
  series,
  height = 180
}: {
  emptyLabel: string;
  series?: MetricSeries;
  height?: number;
}) {
  const points = series?.points ?? [];

  if (points.length < 2) {
    return (
      <div className="flex items-center justify-center rounded-lg border border-dashed border-panel-line bg-slate-950/20 text-sm text-slate-500" style={{ height }}>
        {emptyLabel}
      </div>
    );
  }

  const { color, gradientStart, gradientEnd } = getMetricTheme(series?.key);
  const isPercent = series?.unit === "%";

  const option: EChartsOption = {
    animationDuration: 300,
    animationEasing: "cubicOut",
    backgroundColor: "transparent",
    color: [color],
    grid: {
      bottom: 24,
      containLabel: true,
      left: 12,
      right: 16,
      top: 20
    },
    tooltip: {
      trigger: "axis",
      axisPointer: {
        type: "cross",
        crossStyle: { color: "rgba(148, 163, 184, 0.4)", width: 1, type: "dashed" },
        lineStyle: { color: "rgba(148, 163, 184, 0.3)", width: 1 }
      },
      backgroundColor: "rgba(11, 17, 26, 0.92)",
      borderColor: "rgba(51, 65, 85, 0.8)",
      borderWidth: 1,
      padding: [10, 14],
      textStyle: { color: "#e2e8f0", fontSize: 12 },
      extraCssText: "box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.6), 0 8px 10px -6px rgba(0, 0, 0, 0.6); backdrop-filter: blur(8px); border-radius: 8px;",
      confine: true,
      formatter: (params) => {
        const item = Array.isArray(params) ? params[0] : params;
        const raw = item?.value;
        const pair = Array.isArray(raw) ? raw : [];
        const timestamp = typeof pair[0] === "string" || typeof pair[0] === "number" ? new Date(pair[0]) : null;
        const value = Number(pair[1]);
        const time = timestamp && !Number.isNaN(timestamp.getTime())
          ? timestamp.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
          : "";
        return `
          <div style="min-width: 140px;">
            <div style="font-size: 11px; color: #94a3b8; margin-bottom: 4px;">${time}</div>
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px;">
              <span style="display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: #cbd5e1;">
                <span style="display: inline-block; width: 8px; height: 8px; border-radius: 50%; background-color: ${color}; box-shadow: 0 0 6px ${color};"></span>
                ${series?.title ?? "Metric"}
              </span>
              <strong style="font-family: monospace; font-size: 14px; font-weight: 700; color: #f8fafc;">
                ${formatMetricValue(value, series?.unit ?? "")}
              </strong>
            </div>
          </div>
        `;
      }
    },
    xAxis: {
      type: "time",
      boundaryGap: false,
      axisLine: { lineStyle: { color: "rgba(51, 65, 85, 0.6)" } },
      axisTick: { show: false },
      axisLabel: {
        color: "#64748b",
        fontSize: 11,
        hideOverlap: true,
        margin: 12
      },
      splitLine: {
        show: true,
        lineStyle: { color: "rgba(51, 65, 85, 0.15)", type: "dashed" }
      }
    } as unknown as EChartsOption["xAxis"],
    yAxis: {
      type: "value",
      min: 0,
      max: isPercent ? 100 : undefined,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: "#64748b",
        fontSize: 11,
        formatter: (value: number) => compactMetricValue(value, series?.unit ?? ""),
        margin: 8
      },
      splitLine: {
        lineStyle: { color: "rgba(51, 65, 85, 0.25)", type: "dashed" }
      }
    },
    series: [
      {
        name: series?.title,
        type: "line",
        smooth: 0.25,
        symbol: "none",
        lineStyle: {
          color,
          width: 2.2,
          shadowColor: color,
          shadowBlur: 8
        },
        areaStyle: {
          color: new graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: gradientStart },
            { offset: 1, color: gradientEnd }
          ])
        },
        markLine: isPercent
          ? {
              silent: true,
              symbol: "none",
              data: [
                {
                  yAxis: 80,
                  lineStyle: { color: "#f87171", type: "dashed", width: 1, opacity: 0.6 },
                  label: { show: true, position: "end", formatter: "80% Warn", color: "#f87171", fontSize: 10 }
                }
              ]
            }
          : undefined,
        data: points.map((p) => [p.timestamp, p.value])
      }
    ]
  };

  return <ReactECharts notMerge option={option} opts={{ renderer: "canvas" }} style={{ height, width: "100%" }} />;
}

export function MetricSparkline({
  points,
  color = "#59d46f",
  height = 36,
  width = 100
}: {
  points: number[];
  color?: string;
  height?: number;
  width?: number;
}) {
  if (!points || points.length < 2) {
    return <div className="h-9 w-24 rounded bg-slate-900/40 animate-pulse" />;
  }

  const option: EChartsOption = {
    animation: false,
    backgroundColor: "transparent",
    grid: { top: 2, bottom: 2, left: 2, right: 2 },
    xAxis: { type: "category", show: false, boundaryGap: false },
    yAxis: { type: "value", show: false, min: Math.min(...points) * 0.9, max: Math.max(...points) * 1.1 },
    series: [
      {
        type: "line",
        smooth: 0.3,
        symbol: "none",
        lineStyle: { color, width: 1.5 },
        areaStyle: {
          color: new graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: `${color}40` },
            { offset: 1, color: `${color}00` }
          ])
        },
        data: points
      }
    ]
  };

  return <ReactECharts notMerge option={option} opts={{ renderer: "canvas" }} style={{ height, width }} />;
}

function getMetricTheme(key?: string) {
  if (key === "nodeMemory" || key === "memory") {
    return {
      color: "#a873ff",
      gradientStart: "rgba(168, 115, 255, 0.32)",
      gradientEnd: "rgba(168, 115, 255, 0.01)"
    };
  }
  if (key === "nodeNetwork" || key === "network") {
    return {
      color: "#38bdf8",
      gradientStart: "rgba(56, 189, 248, 0.32)",
      gradientEnd: "rgba(56, 189, 248, 0.01)"
    };
  }
  if (key === "nodeDisk" || key === "disk") {
    return {
      color: "#fbbf24",
      gradientStart: "rgba(251, 191, 36, 0.30)",
      gradientEnd: "rgba(251, 191, 36, 0.01)"
    };
  }
  return {
    color: "#59d46f",
    gradientStart: "rgba(89, 212, 111, 0.35)",
    gradientEnd: "rgba(89, 212, 111, 0.01)"
  };
}

export function formatMetricValue(value: number, unit: string) {
  if (!Number.isFinite(value)) return "—";
  if (unit === "%") return `${value.toFixed(1)}%`;
  if (unit === "MB") return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${value.toFixed(0)} MB`;
  if (unit === "MB/s") return `${value.toFixed(2)} MB/s`;
  if (unit === "KB/s") return `${value.toFixed(1)} KB/s`;
  return `${value.toFixed(1)} ${unit}`.trim();
}

export function compactMetricValue(value: number, unit: string) {
  if (unit === "%") return `${value}%`;
  if (unit === "MB" && value >= 1024) return `${(value / 1024).toFixed(1)}G`;
  return `${value}${unit ? ` ${unit}` : ""}`;
}
