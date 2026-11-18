"use client";

import { Fragment } from "react";
import { Check } from "lucide-react";
import { focusRing } from "../ui";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export function Stepper({
  steps,
  current,
  maxReached,
  onSelect,
}: {
  steps: string[];
  current: number;
  /** Highest step index the user may jump to. */
  maxReached: number;
  onSelect: (index: number) => void;
}) {
  return (
    <ol className="flex items-center gap-2 sm:gap-3" aria-label="创建步骤">
      {steps.map((label, i) => {
        const done = i < current;
        const active = i === current;
        const clickable = i !== current && i <= maxReached;
        return (
          <Fragment key={label}>
            {i > 0 && (
              <li aria-hidden className={cx("h-0.5 min-w-3 flex-1 rounded-full", i <= current ? "bg-[#2B3AE8]" : "bg-[#E3E6F2]")} />
            )}
            <li className="shrink-0">
              <button
                type="button"
                disabled={!clickable}
                onClick={() => onSelect(i)}
                aria-current={active ? "step" : undefined}
                className={cx(
                  "flex items-center gap-2 rounded-full py-1 pr-2 text-sm font-medium transition-colors disabled:cursor-default",
                  clickable && "hover:text-[#2B3AE8]",
                  active ? "text-[#14161F]" : done ? "text-[#3D4258]" : "text-[#8A90A6]",
                  focusRing,
                )}
              >
                <span
                  className={cx(
                    "flex h-7 w-7 items-center justify-center rounded-full text-[13px] tabular-nums",
                    done || active ? "bg-[#2B3AE8] text-white" : "bg-[#E7EAF5] text-[#5B6075]",
                  )}
                >
                  {done ? <Check className="h-4 w-4" strokeWidth={2.5} /> : i + 1}
                </span>
                <span className={cx(active ? "inline" : "hidden sm:inline")}>{label}</span>
              </button>
            </li>
          </Fragment>
        );
      })}
    </ol>
  );
}
