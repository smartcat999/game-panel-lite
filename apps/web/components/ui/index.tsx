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
        "inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-50",
        variant === "primary" && "bg-panel-green text-slate-950 hover:bg-panel-green/90",
        variant === "secondary" && "border border-panel-line bg-slate-900/70 text-slate-100 hover:bg-slate-800",
        variant === "danger" && "bg-red-500/15 text-red-200 hover:bg-red-500/25",
        variant === "gold" && "bg-panel-gold/20 text-panel-gold hover:bg-panel-gold/30",
        variant === "ghost" && "text-slate-300 hover:bg-slate-800",
        className
      )}
      {...props}
    />
  );
}

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <section className={cn("rounded-lg border border-panel-line bg-panel-card", className)}>{children}</section>;
}

export function Badge({ className, children }: { className?: string; children: ReactNode }) {
  return <span className={cn("inline-flex items-center rounded px-2 py-0.5 text-xs font-medium", className)}>{children}</span>;
}

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn("h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-panel-green", className)}
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
        "pointer-events-auto relative flex w-[min(340px,calc(100vw-32px))] items-start gap-3 overflow-hidden rounded-lg border bg-slate-950 px-3.5 py-3 text-sm text-slate-100 shadow-[0_18px_44px_rgba(0,0,0,0.48)]",
        "before:absolute before:inset-y-0 before:left-0 before:w-1",
        tone === "success" && "border-panel-green/45 before:bg-panel-green",
        tone === "info" && "border-blue-400/40 before:bg-blue-400",
        tone === "warning" && "border-panel-gold/45 before:bg-panel-gold",
        tone === "error" && "border-red-400/45 before:bg-red-400"
      )}
      role={tone === "error" ? "alert" : "status"}
    >
      <span
        className={cn(
          "mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md border",
          tone === "success" && "border-panel-green/35 bg-panel-green/12 text-panel-green",
          tone === "info" && "border-blue-400/30 bg-blue-400/10 text-blue-200",
          tone === "warning" && "border-panel-gold/35 bg-panel-gold/10 text-panel-gold",
          tone === "error" && "border-red-400/35 bg-red-400/10 text-red-200"
        )}
      >
        <Icon aria-hidden="true" className="size-4" />
      </span>
      <p className="min-w-0 flex-1 pt-0.5 font-medium leading-5">{message}</p>
      {onClose ? (
        <button
          aria-label={closeLabel}
          className="flex size-7 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
          onClick={onClose}
          type="button"
        >
          <X aria-hidden="true" className="size-4" />
        </button>
      ) : null}
    </div>
  );
}
