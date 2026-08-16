"use client";

import ReactECharts from "echarts-for-react";
import type { EChartsOption } from "echarts";
import type { MetricSeries } from "@/features/monitoring/types";

export type ServerStatusDatum = {
  color: string;
  label: string;
  value: number;
};

export function ServerStatusKpis({ data, hint }: { data: ServerStatusDatum[]; hint: string }) {
  return (
    <div className="min-w-0">
      <dl className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {data.map((item) => (
          <div className="min-w-0 rounded-md border border-panel-line bg-slate-950/25 px-4 py-3" key={item.label}>
            <dt className="flex min-w-0 items-center gap-2 text-xs text-slate-500">
              <span className="size-1.5 shrink-0 rounded-full" style={{ backgroundColor: item.color }} />
              <span className="truncate">{item.label}</span>
            </dt>
            <dd className="mt-2 font-mono text-2xl font-semibold tabular-nums tracking-tight text-slate-100">{item.value.toLocaleString()}</dd>
          </div>
        ))}
      </dl>
      <p className="mt-3 text-xs leading-5 text-slate-500">{hint}</p>
    </div>
  );
}

export function ResourceTrendChart({ emptyLabel, series }: { emptyLabel: string; series?: MetricSeries }) {
  const points = series?.points ?? [];

  if (points.length < 2) {
    return <div className="flex h-[260px] items-center justify-center text-sm text-slate-500">{emptyLabel}</div>;
  }

  const color = chartColor(series?.key);
  const option: EChartsOption = {
    animationDuration: 180,
    animationEasing: "quarticOut",
    backgroundColor: "transparent",
    color: [color],
    grid: { bottom: 18, containLabel: true, left: 6, right: 12, top: 18 },
    tooltip: {
      trigger: "axis",
      axisPointer: { lineStyle: { color: "rgba(148,163,184,.35)", type: "dashed" } },
      backgroundColor: "#0b111a",
      borderColor: "#334155",
      borderWidth: 1,
      confine: true,
      formatter: (params) => {
        const item = Array.isArray(params) ? params[0] : params;
        const raw = item?.value;
        const pair = Array.isArray(raw) ? raw : [];
        const timestamp = typeof pair[0] === "string" || typeof pair[0] === "number" ? new Date(pair[0]) : null;
        const value = Number(pair[1]);
        const time = timestamp && !Number.isNaN(timestamp.getTime()) ? timestamp.toLocaleString() : "";
        return `${time}<br/><strong>${formatMetricValue(value, series?.unit ?? "")}</strong>`;
      }
    },
    xAxis: {
      axisLabel: { color: "#64748b", hideOverlap: true, margin: 12 },
      axisLine: { lineStyle: { color: "rgba(100,116,139,.35)" } },
      axisTick: { show: false },
      boundaryGap: false,
      type: "time"
    } as unknown as EChartsOption["xAxis"],
    yAxis: {
      axisLabel: { color: "#64748b", formatter: (value: number) => compactMetricValue(value, series?.unit ?? ""), margin: 10 },
      axisLine: { show: false },
      axisTick: { show: false },
      min: 0,
      splitLine: { lineStyle: { color: "rgba(148,163,184,.11)" } },
      type: "value"
    },
    series: [{
      areaStyle: { color, opacity: 0.06 },
      data: points.map((point) => [point.timestamp, point.value]),
      lineStyle: { color, width: 2 },
      name: series?.title,
      showSymbol: false,
      smooth: 0.18,
      symbol: "circle",
      type: "line"
    }]
  };

  return <ReactECharts notMerge option={option} opts={{ renderer: "canvas" }} style={{ height: 260, width: "100%" }} />;
}

function chartColor(key?: string) {
  if (key === "nodeMemory") return "#a78bfa";
  if (key === "nodeNetwork") return "#60a5fa";
  return "#59d46f";
}

function formatMetricValue(value: number, unit: string) {
  if (!Number.isFinite(value)) return "—";
  if (unit === "%") return `${value.toFixed(1)}%`;
  if (unit === "MB") return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${value.toFixed(0)} MB`;
  if (unit === "MB/s") return `${value.toFixed(2)} MB/s`;
  return `${value.toFixed(1)} ${unit}`.trim();
}

function compactMetricValue(value: number, unit: string) {
  if (unit === "%") return `${value}%`;
  if (unit === "MB" && value >= 1024) return `${(value / 1024).toFixed(1)}G`;
  return `${value}${unit ? ` ${unit}` : ""}`;
}
