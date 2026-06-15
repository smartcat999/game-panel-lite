import { useEffect, useRef, useState } from "react";

const MAX_POINTS = 60;

export type SeriesPoint = {
  value: number;
  ts: number;
};

export function useTimeSeries(value: number | undefined, maxPoints = MAX_POINTS) {
  const [series, setSeries] = useState<SeriesPoint[]>([]);
  const lastTsRef = useRef(0);

  useEffect(() => {
    if (value === undefined) return;
    const now = Date.now();
    if (now - lastTsRef.current < 500) return;
    lastTsRef.current = now;
    setSeries((prev) => [...prev.slice(-(maxPoints - 1)), { value, ts: now }]);
  }, [value, maxPoints]);

  return series;
}

export function Sparkline({
  data,
  width = 120,
  height = 36,
  color = "#7bd978",
  max = 100,
  fill = true
}: {
  data: SeriesPoint[];
  width?: number;
  height?: number;
  color?: string;
  max?: number;
  fill?: boolean;
}) {
  if (data.length < 2) {
    return (
      <svg width={width} height={height} className="opacity-30">
        <line x1={0} y1={height / 2} x2={width} y2={height / 2} stroke={color} strokeWidth={1} strokeDasharray="3 3" />
      </svg>
    );
  }

  const points = data.map((p) => {
    const first = data[0]!.ts;
    const last = data[data.length - 1]!.ts;
    const x = (p.ts - first) / (last - first || 1) * width;
    const y = height - Math.min(1, p.value / max) * height;
    return { x, y };
  });

  const path = points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(" ");
  const areaPath = `${path} L ${width} ${height} L 0 ${height} Z`;
  const gradId = `spark-${color.replace(/[^a-z0-9]/gi, "")}`;

  return (
    <svg width={width} height={height} className="overflow-visible">
      {fill && (
        <>
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.25} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <path d={areaPath} fill={`url(#${gradId})`} />
        </>
      )}
      <path d={path} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={points[points.length - 1]!.x} cy={points[points.length - 1]!.y} r={2} fill={color} />
    </svg>
  );
}
