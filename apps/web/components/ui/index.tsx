import { AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Button({
  className,
  variant = "primary",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "gold" | "ghost" }) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-50 select-none",
        variant === "primary" && "bg-emerald-600 text-white hover:bg-emerald-700 shadow-xs font-semibold",
        variant === "secondary" && "border micro-border bg-white text-slate-700 hover:bg-slate-50 shadow-xs",
        variant === "danger" && "bg-rose-50 text-rose-700 border border-rose-200 hover:bg-rose-100",
        variant === "gold" && "bg-amber-50 text-amber-800 border border-amber-200 hover:bg-amber-100",
        variant === "ghost" && "text-slate-600 hover:text-slate-900 hover:bg-slate-100",
        className
      )}
      {...props}
    />
  );
}

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <section className={cn("rounded-xl border micro-border bg-white subtle-elevation", className)}>{children}</section>;
}

export function Badge({ className, children }: { className?: string; children: ReactNode }) {
  return <span className={cn("inline-flex items-center rounded-md border micro-border bg-slate-50 px-2 py-0.5 text-xs font-medium text-slate-700", className)}>{children}</span>;
}

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "w-full h-8 rounded-md border micro-border bg-white px-2.5 text-xs text-slate-900 outline-none placeholder:text-slate-400 focus:border-emerald-500 transition",
        className
      )}
      {...props}
    />
  );
}

export function ToastNotice({
  closeLabel = "Close notification",
  message,
  tone = "success",
  onClose
}: {
  closeLabel?: string;
  message: string;
  tone?: "success" | "warning" | "error" | "info";
  onClose?: () => void;
}) {
  if (!message) return null;
  const Icon = tone === "success" ? CheckCircle2 : tone === "info" ? Info : AlertTriangle;
  return (
    <div
      className={cn(
        "pointer-events-auto relative flex w-[min(360px,calc(100vw-32px))] items-start gap-3 overflow-hidden rounded-xl border micro-border bg-white px-3.5 py-3 text-xs text-slate-800 shadow-lg subtle-elevation",
        "before:absolute before:inset-y-0 before:left-0 before:w-1",
        tone === "success" && "border-emerald-200 before:bg-emerald-500",
        tone === "info" && "border-sky-200 before:bg-sky-500",
        tone === "warning" && "border-amber-200 before:bg-amber-500",
        tone === "error" && "border-rose-200 before:bg-rose-500"
      )}
      role={tone === "error" ? "alert" : "status"}
    >
      <span
        className={cn(
          "mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-md border",
          tone === "success" && "border-emerald-200 bg-emerald-50 text-emerald-600",
          tone === "info" && "border-sky-200 bg-sky-50 text-sky-600",
          tone === "warning" && "border-amber-200 bg-amber-50 text-amber-600",
          tone === "error" && "border-rose-200 bg-rose-50 text-rose-600"
        )}
      >
        <Icon aria-hidden="true" className="size-3.5" />
      </span>
      <p className="min-w-0 flex-1 pt-0.5 font-medium leading-relaxed">{message}</p>
      {onClose ? (
        <button
          aria-label={closeLabel}
          className="flex size-6 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-700"
          onClick={onClose}
          type="button"
        >
          <X aria-hidden="true" className="size-3.5" />
        </button>
      ) : null}
    </div>
  );
}
