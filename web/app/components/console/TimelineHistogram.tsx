"use client";

import { useMemo } from "react";
import { focusRing } from "./ui";

export interface HistogramDatum {
  timestamp?: string;
  isError?: boolean;
}

interface HistogramBounds {
  startMs: number;
  endMs: number;
  stepMs: number;
}

function resolveHistogramBounds(timestampsMs: number[], bucketCount: number): HistogramBounds | null {
  if (timestampsMs.length === 0) return null;
  let minTs = timestampsMs[0];
  let maxTs = timestampsMs[0];
  for (let i = 1; i < timestampsMs.length; i += 1) {
    if (timestampsMs[i] < minTs) minTs = timestampsMs[i];
    if (timestampsMs[i] > maxTs) maxTs = timestampsMs[i];
  }
  const endMs = maxTs;
  const startMs = Math.min(minTs, endMs - 60 * 1000);
  const spanMs = Math.max(60 * 1000, endMs - startMs);
  return {
    startMs,
    endMs,
    stepMs: spanMs / bucketCount,
  };
}

function bucketIndexFor(tsMs: number, bounds: HistogramBounds, bucketCount: number): number {
  const raw = Math.floor((tsMs - bounds.startMs) / bounds.stepMs);
  if (raw < 0) return 0;
  if (raw >= bucketCount) return bucketCount - 1;
  return raw;
}

function formatBucketClock(ms: number, longSpan: boolean, showSeconds = false): string {
  const d = new Date(ms);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  if (longSpan) {
    const mon = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${mon}-${day} ${hh}:${mm}`;
  }
  if (!showSeconds) return `${hh}:${mm}`;
  const ss = String(d.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}

export function filterByHistogramBucket<T>(
  items: T[],
  getTimestamp: (item: T) => string | undefined,
  windowKey: string,
  bucketIndex: number | null,
  bucketCount = 24,
): T[] {
  void windowKey;
  if (bucketIndex === null || bucketIndex < 0 || bucketIndex >= bucketCount) {
    return items;
  }
  const validMs: number[] = [];
  for (const item of items) {
    const raw = getTimestamp(item);
    if (!raw) continue;
    const ms = Date.parse(raw);
    if (!Number.isNaN(ms)) validMs.push(ms);
  }
  const bounds = resolveHistogramBounds(validMs, bucketCount);
  if (!bounds) return items;
  return items.filter((item) => {
    const raw = getTimestamp(item);
    if (!raw) return false;
    const ms = Date.parse(raw);
    if (Number.isNaN(ms)) return false;
    return bucketIndexFor(ms, bounds, bucketCount) === bucketIndex;
  });
}

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export function TimelineHistogram({
  title,
  windowKey,
  items,
  selectedBucket,
  onSelectBucket,
  onlyErrors = false,
  onToggleOnlyErrors,
  bucketCount = 24,
  className,
}: {
  title: string;
  windowKey: string;
  items: HistogramDatum[];
  selectedBucket: number | null;
  onSelectBucket: (idx: number | null) => void;
  onlyErrors?: boolean;
  onToggleOnlyErrors?: () => void;
  bucketCount?: number;
  className?: string;
}) {
  void windowKey;
  const model = useMemo(() => {
    const parsed: Array<{ ms: number; isError: boolean }> = [];
    const tsOnly: number[] = [];
    for (const item of items) {
      if (!item.timestamp) continue;
      const ms = Date.parse(item.timestamp);
      if (Number.isNaN(ms)) continue;
      parsed.push({ ms, isError: Boolean(item.isError) });
      tsOnly.push(ms);
    }
    const bounds = resolveHistogramBounds(tsOnly, bucketCount);
    const buckets = Array.from({ length: bucketCount }, () => ({ total: 0, errors: 0 }));
    if (bounds) {
      for (const p of parsed) {
        const idx = bucketIndexFor(p.ms, bounds, bucketCount);
        buckets[idx].total += 1;
        if (p.isError) buckets[idx].errors += 1;
      }
    }
    const maxCount = Math.max(...buckets.map((b) => b.total), 1);
    const errorTotal = parsed.filter((p) => p.isError).length;
    return { bounds, buckets, maxCount, total: parsed.length, errorTotal };
  }, [items, bucketCount]);

  if (!model.bounds || model.total === 0) {
    return null;
  }

  const spanMs = model.bounds.endMs - model.bounds.startMs;
  const longSpan = spanMs >= 48 * 60 * 60 * 1000;
  const showSeconds = !longSpan && spanMs <= 6 * 60 * 60 * 1000;
  const activeStartMs =
    selectedBucket !== null ? model.bounds.startMs + selectedBucket * model.bounds.stepMs : 0;
  const activeEndMs =
    selectedBucket !== null ? model.bounds.startMs + (selectedBucket + 1) * model.bounds.stepMs : 0;

  return (
    <div className={cx("rounded-2xl bg-[#F7F8FC] px-4 py-3", className)}>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-xs">
        <div className="flex flex-wrap items-center gap-2 text-[#5B6075]">
          <span className="font-medium text-[#14161F]">{title}</span>
          {model.errorTotal > 0 &&
            (onToggleOnlyErrors ? (
              <button
                type="button"
                aria-pressed={onlyErrors}
                onClick={onToggleOnlyErrors}
                className={cx(
                  "rounded-full px-2 py-0.5 font-mono text-[11px] font-medium transition-colors",
                  onlyErrors
                    ? "bg-[#DC2626] text-white hover:bg-[#B91C1C]"
                    : "bg-[#FEE4E2] text-[#B42318] hover:bg-[#FECDCA]",
                  focusRing,
                )}
              >
                {onlyErrors ? `仅看异常 ${model.errorTotal} 条 ×` : `异常 ${model.errorTotal} 条`}
              </button>
            ) : (
              <span className="font-mono text-[#B42318]">异常 {model.errorTotal} 条</span>
            ))}
        </div>
        {selectedBucket !== null ? (
          <button
            type="button"
            onClick={() => onSelectBucket(null)}
            className={cx(
              "inline-flex items-center gap-1.5 rounded-full bg-[#EEF1FF] px-2.5 py-0.5 font-mono text-xs font-medium text-[#2B3AE8] hover:bg-[#E0E6FF]",
              focusRing,
            )}
          >
            <span>
              {formatBucketClock(activeStartMs, longSpan, showSeconds)} - {formatBucketClock(activeEndMs, longSpan, showSeconds)}
            </span>
            <span>清除筛选 ×</span>
          </button>
        ) : (
          <div className="flex items-center gap-3 text-xs text-[#8A90A6]">
            <span className="inline-flex items-center gap-1">
              <span className="h-2 w-2 rounded-xs bg-[#2B3AE8]" aria-hidden />
              常规
            </span>
            <span className="inline-flex items-center gap-1">
              <span className="h-2 w-2 rounded-xs bg-[#DC2626]" aria-hidden />
              含异常
            </span>
          </div>
        )}
      </div>

      <div className="flex h-12 items-end gap-1" role="group" aria-label={title}>
        {model.buckets.map((bucket, idx) => {
          const isSel = selectedBucket === idx;
          const dimmed = selectedBucket !== null && !isSel;
          const startLabel = formatBucketClock(model.bounds!.startMs + idx * model.bounds!.stepMs, longSpan, showSeconds);
          const endLabel = formatBucketClock(model.bounds!.startMs + (idx + 1) * model.bounds!.stepMs, longSpan, showSeconds);
          if (bucket.total === 0) {
            return (
              <div
                key={idx}
                className="flex h-full flex-1 flex-col justify-end"
                title={`${startLabel} - ${endLabel}：0 条`}
              >
                <div className="h-[3px] w-full rounded-full bg-[#E6E9F4]" />
              </div>
            );
          }
          const heightPct = Math.max(18, Math.round((bucket.total / model.maxCount) * 100));
          const hasErrors = bucket.errors > 0;
          const errShare =
            bucket.errors === bucket.total
              ? 100
              : Math.max(35, Math.round((bucket.errors / bucket.total) * 100));
          return (
            <button
              key={idx}
              type="button"
              onClick={() => onSelectBucket(isSel ? null : idx)}
              title={`${startLabel} - ${endLabel}：共 ${bucket.total} 条${hasErrors ? `（含异常 ${bucket.errors} 条）` : ""}`}
              aria-label={`时段 ${startLabel} 到 ${endLabel}，共 ${bucket.total} 条`}
              aria-pressed={isSel}
              className={cx(
                "flex h-full flex-1 cursor-pointer flex-col justify-end rounded-xs px-[1px] transition-opacity",
                dimmed ? "opacity-35 hover:opacity-80" : "opacity-90 hover:opacity-100",
                isSel && "bg-[#EEF1FF]/70 ring-1 ring-[#2B3AE8]",
                focusRing,
              )}
            >
              <div
                style={{ height: `${heightPct}%` }}
                className={cx(
                  "flex w-full flex-col overflow-hidden rounded-t-[2px]",
                  isSel && "ring-1 ring-[#14161F]",
                )}
              >
                {hasErrors ? (
                  <>
                    <div style={{ height: `${errShare}%` }} className="w-full shrink-0 bg-[#DC2626]" />
                    {errShare < 100 && <div className="w-full flex-1 bg-[#2B3AE8]" />}
                  </>
                ) : (
                  <div className="h-full w-full bg-[#2B3AE8]" />
                )}
              </div>
            </button>
          );
        })}
      </div>

      <div className="mt-1.5 flex items-center justify-between font-mono text-[11px] text-[#8A90A6]">
        <span>{formatBucketClock(model.bounds.startMs, longSpan)}</span>
        <span>{formatBucketClock(model.bounds.endMs, longSpan)}</span>
      </div>
    </div>
  );
}
