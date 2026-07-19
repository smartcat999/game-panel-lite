"use client";

import { Eye, EyeOff } from "lucide-react";
import { useEffect, useState, type InputHTMLAttributes } from "react";
import { Input } from "@/components/ui";
import { cn } from "@/lib/utils";

export function SecretInput({
  className,
  disabled,
  hideLabel,
  showLabel,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { hideLabel: string; showLabel: string }) {
  const [revealed, setRevealed] = useState(false);

  useEffect(() => {
    if (disabled) setRevealed(false);
  }, [disabled]);

  const toggleLabel = revealed ? hideLabel : showLabel;

  return (
    <div className="relative">
      <Input
        {...props}
        autoComplete={props.autoComplete ?? "off"}
        className={cn("w-full pr-10", className)}
        disabled={disabled}
        spellCheck={false}
        type={revealed ? "text" : "password"}
      />
      <button
        aria-label={toggleLabel}
        aria-pressed={revealed}
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center rounded-r-md text-slate-500 transition hover:text-slate-200 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-50"
        disabled={disabled}
        onClick={() => setRevealed((current) => !current)}
        title={toggleLabel}
        type="button"
      >
        {revealed ? <EyeOff aria-hidden="true" className="size-4" /> : <Eye aria-hidden="true" className="size-4" />}
      </button>
    </div>
  );
}
