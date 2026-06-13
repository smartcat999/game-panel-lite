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
