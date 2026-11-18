"use client";

import ReactECharts from "echarts-for-react";
import type { EChartsOption } from "echarts";
import type { ReactNode } from "react";
import { Card } from "@/components/ui";
import type { MonitoringTimeSeriesPoint } from "@/lib/monitoring";
import { cn } from "@/lib/utils";

type MonitoringChartCardProps = {
  chartType?: "line" | "bar";
  color: string;
  currentValue: string;
  data: MonitoringTimeSeriesPoint[];
  emptyLabel: string;
  icon: ReactNode;
  limitLabel: string;
  subtitle: string;
  summary: {
    average: number;
    peak: number;
    samples: number;
  };
  threshold?: number;
  title: string;
  unit: "" | "%" | "MB";
};

export function MonitoringChartCard({
  chartType = "line",
  color,
  currentValue,
  data,
  emptyLabel,
  icon,
  limitLabel,
  subtitle,
  summary,
  threshold,
  title,
  unit
}: MonitoringChartCardProps) {
  const hasData = data.length > 0;
  const option = createChartOption({ chartType, color, data, threshold, unit });
  return (
    <Card className="h-[300px] p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-semibold text-slate-100">
            <span style={{ color }}>{icon}</span>
            <span className="truncate">{title}</span>
          </div>
          <p className="mt-1 truncate text-xs text-slate-500">{subtitle}</p>
        </div>
        <p className="font-mono text-xl font-semibold text-slate-100">{currentValue}</p>
      </div>

      <div className="h-[190px] rounded-md border border-panel-line bg-slate-950/35">
        {hasData ? (
          <ReactECharts
            className="h-full w-full"
            notMerge
            option={option}
            opts={{ renderer: "canvas" }}
            style={{ height: "100%", width: "100%" }}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-slate-500">{emptyLabel}</div>
        )}
      </div>

      <div className="mt-2 grid grid-cols-4 gap-2 text-[11px] text-slate-500">
        <ChartStat label="avg" value={formatValue(summary.average, unit)} />
        <ChartStat label="peak" value={formatValue(summary.peak, unit)} />
        <ChartStat label="samples" value={String(summary.samples)} />
        <ChartStat className="text-right" label="limit" value={limitLabel} />
      </div>
    </Card>
  );
}

function ChartStat({ className, label, value }: { className?: string; label: string; value: string }) {
  return (
    <div className={cn("min-w-0", className)}>
      <p className="truncate text-slate-600">{label}</p>
      <p className="truncate font-mono text-slate-400">{value}</p>
    </div>
  );
}

function createChartOption({
  chartType,
  color,
  data,
  threshold,
  unit
}: {
  chartType: "line" | "bar";
  color: string;
  data: MonitoringTimeSeriesPoint[];
  threshold?: number;
  unit: "" | "%" | "MB";
}): EChartsOption {
  return {
    animation: false,
    backgroundColor: "transparent",
    grid: { bottom: 34, containLabel: true, left: 10, right: 14, top: 18 },
    tooltip: {
      trigger: "axis",
      axisPointer: {
        label: { backgroundColor: "#111827" },
        lineStyle: { color: "rgba(148,163,184,0.45)", type: "dashed" },
        type: "cross"
      },
      backgroundColor: "#0b111a",
      borderColor: "rgba(71,85,105,0.8)",
      borderWidth: 1,
      className: "gamepanel-chart-tooltip",
      confine: true,
      formatter: (params: unknown) => {
        const item = (Array.isArray(params) ? params[0] : params) as { axisValue?: string | number; value?: unknown } | undefined;
        const rawTime = Array.isArray(item?.value) ? item.value[0] : item?.axisValue;
        const rawValue = Array.isArray(item?.value) ? item.value[1] : item?.value;
        const value = Number(rawValue ?? 0);
        return `<div style="font-size:12px;color:#cbd5e1;">${formatTimeLabel(rawTime)}</div><div style="margin-top:4px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;color:#f8fafc;">${formatValue(value, unit)}</div>`;
      },
      padding: [8, 10],
      textStyle: { color: "#e2e8f0", fontFamily: "Inter, ui-sans-serif, system-ui" }
    },
    xAxis: {
      axisLabel: {
        color: "#64748b",
        formatter: (value: string | number) => formatTimeLabel(value),
        hideOverlap: true,
        margin: 12
      },
      axisLine: { lineStyle: { color: "rgba(100,116,139,0.35)" } },
      axisPointer: { snap: true },
      axisTick: { show: false },
      boundaryGap: chartType === "bar",
      splitLine: { lineStyle: { color: "rgba(148,163,184,0.08)" }, show: true },
      type: "time"
    } as unknown as EChartsOption["xAxis"],
    yAxis: {
      axisLabel: {
        color: "#64748b",
        formatter: (value: number) => formatValue(value, unit),
        margin: 12
      },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: "rgba(148,163,184,0.14)" }, show: true },
      type: "value"
    },
    series: [
      {
        areaStyle: chartType === "line" ? { color, opacity: 0.08 } : undefined,
        barMaxWidth: 18,
        data: data.map((point) => [point.timestamp, point.value]),
        emphasis: { focus: "series" },
        itemStyle: { color },
        lineStyle: { color, width: 2 },
        markLine: threshold === undefined ? undefined : {
          data: [{ yAxis: threshold }],
          label: {
            color: "#f4c95d",
            formatter: "limit",
            position: "insideEndTop"
          },
          lineStyle: { color: "#f4c95d", type: "dashed", width: 1.2 },
          symbol: "none"
        },
        name: "value",
        showSymbol: false,
        smooth: chartType === "line" ? 0.25 : false,
        symbol: "circle",
        symbolSize: 7,
        type: chartType
      }
    ]
  };
}

function formatTimeLabel(value: unknown) {
  if (value === undefined || value === null || value === "") return "-";
  const date = new Date(typeof value === "number" || typeof value === "string" ? value : String(value));
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatValue(value: number, unit: "" | "%" | "MB") {
  if (unit === "%") return `${value.toFixed(1)}%`;
  if (unit === "MB") return `${Math.round(value)} MB`;
  return String(Math.round(value));
}
