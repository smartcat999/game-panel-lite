import { AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Button({
  className,
  variant = "primary",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger" | "ghost";
}) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition cursor-pointer select-none disabled:cursor-not-allowed disabled:opacity-50",
        variant === "primary" && "bg-slate-900 text-white hover:bg-slate-800 shadow-xs font-semibold",
        variant === "secondary" && "border border-slate-200/80 bg-white text-slate-700 hover:bg-slate-50 shadow-2xs",
        variant === "danger" && "bg-rose-50 text-rose-700 border border-rose-200 hover:bg-rose-100",
        variant === "ghost" && "text-slate-600 hover:text-slate-900 hover:bg-slate-100",
        className
      )}
      {...props}
    />
  );
}

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <section className={cn("rounded-xl border border-slate-200/80 bg-white shadow-2xs", className)}>
      {children}
    </section>
  );
}

export function Badge({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <span className={cn("inline-flex items-center rounded-md border border-slate-200/80 bg-slate-50 px-2 py-0.5 text-xs font-medium text-slate-700", className)}>
      {children}
    </span>
  );
}

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "w-full h-8 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-900 placeholder:text-slate-400 focus:border-emerald-500 focus:outline-none transition shadow-2xs",
        className
      )}
      {...props}
    />
  );
}

export function ToastNotice({
  message,
  tone = "success",
  onClose
}: {
  message: string;
  tone?: "success" | "warning" | "error" | "info";
  onClose?: () => void;
}) {
  if (!message) return null;
  const Icon = tone === "success" ? CheckCircle2 : tone === "info" ? Info : AlertTriangle;
  return (
    <div
      className={cn(
        "pointer-events-auto relative flex w-[min(360px,calc(100vw-32px))] items-start gap-2.5 overflow-hidden rounded-xl border bg-white px-3 py-2.5 text-xs text-slate-800 shadow-lg subtle-elevation",
        tone === "success" && "border-emerald-200",
        tone === "info" && "border-sky-200",
        tone === "warning" && "border-amber-200",
        tone === "error" && "border-rose-200"
      )}
      role={tone === "error" ? "alert" : "status"}
    >
      <span
        className={cn(
          "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded",
          tone === "success" && "text-emerald-600",
          tone === "info" && "text-sky-600",
          tone === "warning" && "text-amber-600",
          tone === "error" && "text-rose-600"
        )}
      >
        <Icon aria-hidden="true" className="size-3.5" />
      </span>
      <p className="min-w-0 flex-1 font-medium leading-tight">{message}</p>
      {onClose && (
        <button
          aria-label="Close"
          className="flex size-4 shrink-0 items-center justify-center text-slate-400 hover:text-slate-700"
          onClick={onClose}
          type="button"
        >
          <X aria-hidden="true" className="size-3" />
        </button>
      )}
    </div>
  );
}
