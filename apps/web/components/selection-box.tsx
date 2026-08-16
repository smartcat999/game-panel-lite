"use client";

import { Check } from "lucide-react";
import { cn } from "@/lib/utils";

export function SelectionBox({
  checked,
  indeterminate = false,
  label,
  onChange
}: {
  checked: boolean;
  indeterminate?: boolean;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      aria-label={label}
      aria-pressed={checked || indeterminate}
      className={cn(
        "flex size-4 items-center justify-center rounded-[3px] border transition focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/40",
        checked || indeterminate ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-600 hover:border-slate-400"
      )}
      onClick={onChange}
      type="button"
    >
      {checked ? <Check className="size-3" strokeWidth={3} /> : indeterminate ? <span className="h-0.5 w-2 bg-slate-950" /> : null}
    </button>
  );
}
