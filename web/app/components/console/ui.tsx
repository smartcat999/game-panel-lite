"use client";

/*
 * GamePanel console UI kit — shares tokens with the landing page.
 *   page #F5F7FD · surface #FFFFFF · line #E3E6F2 · cobalt #2B3AE8 (only accent)
 *   ink #14161F · muted #5B6075 · success #1F9D55 · warning #B54708 · danger #D92D20
 * Shape rule: buttons / switchers / pills are fully rounded, cards are 20px, inputs are 12px.
 * Monospace only for addresses, ports, image names and file paths (use <Mono>).
 */

import Link from "next/link";
import {
  ButtonHTMLAttributes,
  forwardRef,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { Check, Copy, Eye, EyeOff, Loader2, MoreHorizontal, X } from "lucide-react";
import { phaseLabel } from "../../console/lib/format";

export const focusRing =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#2B3AE8] focus-visible:ring-offset-2 focus-visible:ring-offset-white";

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

// ------------------------------------------------------------------ buttons
type Variant = "primary" | "secondary" | "ghost" | "danger" | "danger-solid";
type Size = "sm" | "md" | "lg";

const VARIANT: Record<Variant, string> = {
  primary:
    "bg-[#2B3AE8] text-white hover:bg-[#2230D0] shadow-[0_10px_24px_-12px_rgba(43,58,232,0.8)] disabled:bg-[#EEF0F7] disabled:text-[#8A90A6] disabled:shadow-none disabled:opacity-100",
  secondary: "bg-white text-[#14161F] border border-[#D5DAEE] hover:border-[#2B3AE8] hover:text-[#2B3AE8]",
  ghost: "text-[#3D4258] hover:bg-[#EEF1FF] hover:text-[#14161F]",
  danger: "bg-white text-[#D92D20] border border-[#F4C7C3] hover:bg-[#FEF3F2]",
  "danger-solid":
    "bg-[#D92D20] text-white hover:bg-[#B42318] shadow-[0_10px_24px_-12px_rgba(217,45,32,0.72)] disabled:bg-[#EEF0F7] disabled:text-[#8A90A6] disabled:shadow-none disabled:opacity-100",
};
const SIZE: Record<Size, string> = {
  sm: "h-8 px-3.5 text-[13px] gap-1.5",
  md: "h-10 px-5 text-sm gap-2",
  lg: "h-12 px-7 text-[15px] gap-2",
};

export function buttonClass(variant: Variant = "secondary", size: Size = "md", extra?: string) {
  return cx(
    "inline-flex shrink-0 items-center justify-center rounded-full font-medium whitespace-nowrap transition-[background-color,border-color,color,transform] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100",
    VARIANT[variant],
    SIZE[size],
    focusRing,
    extra,
  );
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", icon, loading, className, children, disabled, type = "button", ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      className={buttonClass(variant, size, className)}
      {...rest}
    >
      {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : icon}
      {children}
    </button>
  );
});

export function LinkButton({
  href,
  variant = "secondary",
  size = "md",
  icon,
  className,
  children,
}: {
  href: string;
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Link href={href} className={buttonClass(variant, size, className)}>
      {icon}
      {children}
    </Link>
  );
}

export const IconButton = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { label: string }
>(function IconButton({ label, className, children, ...rest }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      aria-label={label}
      title={label}
      className={cx(
        "inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#F1F3FB] text-[#3D4258] transition-colors hover:bg-[#E3E7FA] hover:text-[#2B3AE8] disabled:cursor-not-allowed disabled:opacity-50",
        focusRing,
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  );
});

// -------------------------------------------------------------- containers
export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <div
      className={cx(
        "rounded-[20px] border border-[#E3E6F2] bg-white shadow-[0_12px_32px_-24px_rgba(43,58,232,0.35)]",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function CardHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div
      className={cx(
        "flex flex-wrap justify-between gap-3 border-b border-[#EEF0F7] px-6",
        description ? "items-start py-5" : "items-center py-4",
      )}
    >
      <div className="min-w-0">
        <h2 className="text-base font-semibold text-[#14161F]">{title}</h2>
        {description && <p className="mt-1 text-sm text-[#5B6075]">{description}</p>}
      </div>
      {actions && <div className="flex min-w-0 max-w-full flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[#14161F]">{title}</h1>
        {description && <p className="mt-1.5 text-[15px] text-[#5B6075]">{description}</p>}
      </div>
      {actions && <div className="flex max-w-full flex-wrap items-center gap-3">{actions}</div>}
    </div>
  );
}

export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cx("font-mono text-[0.92em] tabular-nums", className)}>{children}</span>;
}

// ------------------------------------------------------------------ status
const PHASE_STYLE: Record<string, string> = {
  Running: "bg-[#E7F6EC] text-[#1F7A45]",
  Stopped: "bg-[#EEF0F5] text-[#5B6075]",
  Failed: "bg-[#FEECEB] text-[#B42318]",
};

export function StatusPill({ phase }: { phase: string }) {
  const style = PHASE_STYLE[phase] ?? "bg-[#FFF4E5] text-[#B54708]";
  const dot =
    phase === "Running" ? "bg-[#1F9D55]" : phase === "Failed" ? "bg-[#D92D20]" : phase === "Stopped" ? "bg-[#9AA0B4]" : "bg-[#F79009] animate-pulse";
  return (
    <span className={cx("inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-1 text-xs font-medium", style)}>
      <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", dot)} />
      {phaseLabel(phase)}
    </span>
  );
}

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "accent" | "success" | "warning" | "danger";
}) {
  const tones = {
    neutral: "bg-[#EEF0F5] text-[#3D4258]",
    accent: "bg-[#EEF1FF] text-[#2B3AE8]",
    success: "bg-[#E7F6EC] text-[#1F7A45]",
    warning: "bg-[#FFF4E5] text-[#B54708]",
    danger: "bg-[#FEECEB] text-[#B42318]",
  };
  return <span className={cx("inline-flex shrink-0 items-center whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium", tones[tone])}>{children}</span>;
}

export function Meter({
  label,
  used,
  max,
  format = (n) => String(Number.isInteger(n) ? n : n.toFixed(1)),
  unit = "",
  compact,
  headerLeft,
  headerRight,
  hideHeader,
  className,
  barClassName,
}: {
  label: string;
  used: number;
  max: number;
  format?: (n: number) => string;
  unit?: string;
  compact?: boolean;
  headerLeft?: ReactNode;
  headerRight?: ReactNode;
  hideHeader?: boolean;
  className?: string;
  barClassName?: string;
}) {
  const safeMax = max > 0 ? max : 0;
  const clampedUsed = safeMax > 0 ? Math.min(safeMax, Math.max(0, used)) : 0;
  const pct = safeMax > 0 ? Math.min(100, Math.max(0, (used / safeMax) * 100)) : 0;
  const color = pct >= 90 ? "bg-[#D92D20]" : pct >= 75 ? "bg-[#F79009]" : "bg-[#2B3AE8]";
  return (
    <div className={cx(compact && "min-w-[140px]", className)}>
      {!hideHeader && (
        <div className="flex items-baseline justify-between gap-1.5 whitespace-nowrap text-[13px]">
          {headerLeft ?? <span className="whitespace-nowrap text-[#5B6075]">{label}</span>}
          {headerRight ?? (
            <span className="whitespace-nowrap font-medium tabular-nums text-[#14161F]">
              {format(used)}
              <span className="text-[#8A90A6]">
                {" "}
                / {format(max)}
                {unit && ` ${unit}`}
              </span>
            </span>
          )}
        </div>
      )}
      <div
        className={cx(
          !/(?:^|\s)mt-/.test(barClassName || "") && (hideHeader ? "mt-0" : "mt-2"),
          "h-1.5 overflow-hidden rounded-full bg-[#E7EAF5]",
          barClassName,
        )}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={safeMax}
        aria-valuenow={clampedUsed}
      >
        <div className={cx("h-full rounded-full transition-[width] duration-500", color)} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

// ------------------------------------------------------------------- forms
const hasUtility = (re: RegExp, cls?: string) => !!cls && re.test(cls);
const HEIGHT_RE = /(?:^|\s)h-(?:\d|auto|full|fit|min|max|\[)/;
const WIDTH_RE = /(?:^|\s)w-(?:\d|auto|full|fit|min|max|\[)/;
const RADIUS_RE = /(?:^|\s)rounded(?:-|\s|$)/;
const TEXT_SIZE_RE = /(?:^|\s)text-(?:xs|sm|base|lg|xl|\d*xl|\[[\d.]+(?:px|rem|em)\])(?=\s|$|\/)/;
const BG_COLOR_RE =
  /(?:^|\s)bg-(?:white|black|transparent|\[[^\]]+\]|(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d+)/;
const BORDER_COLOR_RE =
  /(?:^|\s)border-(?:white|black|transparent|\[[^\]]+\]|(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d+)/;
const PX_RE = /(?:^|\s)px-(?:\d|\[)/;

const fieldInteraction =
  "block border text-[#14161F] placeholder:text-[#8A90A6] transition-[border-color,background-color,box-shadow] hover:border-[#B8C0E6] focus:border-[#2B3AE8] focus:outline-none focus:ring-4 focus:ring-[#2B3AE8]/15 disabled:bg-[#F5F7FD] disabled:text-[#8A90A6]";

function buildFieldClass(className?: string, extra?: string, withDefaultHeight = true): string {
  return cx(
    fieldInteraction,
    withDefaultHeight && !hasUtility(HEIGHT_RE, className) && "h-10",
    !hasUtility(WIDTH_RE, className) && "w-full",
    !hasUtility(RADIUS_RE, className) && "rounded-xl",
    !hasUtility(BORDER_COLOR_RE, className) && "border-[#D5DAEE]",
    !hasUtility(BG_COLOR_RE, className) && "bg-white",
    !hasUtility(PX_RE, className) && "px-3.5",
    !hasUtility(TEXT_SIZE_RE, className) && "text-[14px]",
    extra,
    className,
  );
}

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  variant?: "default" | "bare";
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, type, variant = "default", ...rest },
  ref,
) {
  const [revealed, setRevealed] = useState(false);
  const isPassword = type === "password";
  if (!isPassword) {
    if (variant === "bare") {
      return (
        <input
          ref={ref}
          type={type}
          className={cx(
            "min-w-0 bg-transparent text-[#14161F] placeholder:text-[#8A90A6] focus:outline-none disabled:text-[#8A90A6]",
            className,
          )}
          {...rest}
        />
      );
    }
    return <input ref={ref} type={type} className={buildFieldClass(className)} {...rest} />;
  }
  return (
    <div className="relative w-full">
      <input
        ref={ref}
        type={revealed ? "text" : "password"}
        className={buildFieldClass(className, "pr-9")}
        {...rest}
      />
      <button
        type="button"
        tabIndex={-1}
        aria-label={revealed ? "隐藏内容" : "查看内容"}
        title={revealed ? "隐藏内容" : "查看内容"}
        onClick={() => setRevealed((v) => !v)}
        className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-md p-1 text-[#8A90A6] transition hover:bg-[#F2F4FB] hover:text-[#14161F]"
      >
        {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
      </button>
    </div>
  );
});

const selectChevronBg =
  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%235B6075' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")";

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select(
  { className, children, style, ...rest },
  ref,
) {
  return (
    <select
      ref={ref}
      className={buildFieldClass(className, "appearance-none bg-no-repeat pr-9")}
      style={{
        appearance: "none",
        WebkitAppearance: "none",
        backgroundImage: selectChevronBg,
        backgroundPosition: "right 0.75rem center",
        backgroundSize: "0.95rem 0.95rem",
        ...style,
      }}
      {...rest}
    >
      {children}
    </select>
  );
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...rest }, ref) {
    return <textarea ref={ref} className={buildFieldClass(className, "py-2.5", false)} {...rest} />;
  },
);

export function Field({
  label,
  hint,
  error,
  children,
  htmlFor,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  children: ReactNode;
  htmlFor?: string;
}) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={htmlFor} className="block text-[13px] font-medium text-[#3D4258]">
        {label}
      </label>
      {children}
      {error ? (
        <p className="text-[13px] text-[#B42318]">{error}</p>
      ) : hint ? (
        <p className="text-[13px] text-[#8A90A6]">{hint}</p>
      ) : null}
    </div>
  );
}

// -------------------------------------------------------------- segmented
export function Segmented<T extends string>({
  items,
  value,
  onChange,
  label,
  size = "md",
  className,
}: {
  items: Array<{ value: T; label: ReactNode; disabled?: boolean; title?: string }>;
  value: T;
  onChange: (v: T) => void;
  label: string;
  size?: "sm" | "md";
  className?: string;
}) {
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const activeBtn = listRef.current?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
    activeBtn?.scrollIntoView({ block: "nearest", inline: "nearest", container: "nearest" } as ScrollIntoViewOptions);
  }, [value]);

  const sm = size === "sm";

  return (
    <div
      ref={listRef}
      role="tablist"
      aria-label={label}
      className={cx(
        "inline-flex max-w-full flex-nowrap overflow-x-auto rounded-full bg-[#EEF0F7] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        sm ? "gap-0.5 p-0.5" : "gap-1 p-1",
        className,
      )}
    >
      {items.map((it) => {
        const active = it.value === value;
        return (
          <button
            key={it.value}
            type="button"
            role="tab"
            aria-selected={active}
            disabled={it.disabled}
            title={it.title}
            onClick={() => {
              if (!it.disabled) onChange(it.value);
            }}
            className={cx(
              "inline-flex flex-none items-center gap-1.5 whitespace-nowrap rounded-full border border-transparent font-medium text-[#5B6075] transition-colors hover:text-[#14161F] disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:text-[#5B6075]",
              sm ? "h-7 px-2.5 text-xs" : "h-8 px-3 text-xs sm:px-4 sm:text-[13px]",
              "aria-[selected=true]:border-[#E3E6F2]/80 aria-[selected=true]:bg-white aria-[selected=true]:text-[#14161F] aria-[selected=true]:shadow-[0_2px_8px_-2px_rgba(20,22,31,0.15)]",
              focusRing,
            )}
          >
            {it.label}
          </button>
        );
      })}
    </div>
  );
}

export function Switch({
  id,
  checked,
  onChange,
  label,
  disabled,
  className,
}: {
  id?: string;
  checked: boolean;
  onChange: (next: boolean) => void;
  label?: string;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <button
      id={id}
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "bg-[#2B3AE8]" : "bg-[#D5DAEE]",
        focusRing,
        className,
      )}
    >
      <span
        aria-hidden
        className={cx(
          "pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow-[0_2px_6px_rgba(20,22,31,0.2)] transition-transform",
          checked ? "translate-x-5" : "translate-x-0",
        )}
      />
    </button>
  );
}

// ------------------------------------------------------------------- misc
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text || typeof window === "undefined") return false;
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall back to textarea execCommand below
    }
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-9999px";
    ta.style.left = "-9999px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, text.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function CopyButton({ text, label = "复制" }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      aria-label={done ? "已复制" : label}
      title={done ? "已复制" : label}
      onClick={(e) => {
        e.stopPropagation();
        e.preventDefault();
        void copyToClipboard(text).then((ok) => {
          if (ok) {
            setDone(true);
            setTimeout(() => setDone(false), 1600);
          }
        });
      }}
      className={cx(
        "inline-flex h-7 w-7 items-center justify-center rounded-full text-[#8A90A6] transition-colors hover:bg-[#EEF1FF] hover:text-[#2B3AE8]",
        focusRing,
      )}
    >
      {done ? <Check className="h-3.5 w-3.5 text-[#1F9D55]" /> : <Copy className="h-3.5 w-3.5" />}
    </button>
  );
}

export function EmptyState({
  image = "/landing/features/connect.jpg",
  title,
  body,
  action,
}: {
  image?: string | null;
  title: ReactNode;
  body?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      {image && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={image}
          alt=""
          className="mb-6 h-36 w-36 rounded-full object-cover [mask-image:radial-gradient(closest-side,#000_65%,transparent)]"
        />
      )}
      <h3 className="text-lg font-semibold text-[#14161F]">{title}</h3>
      {body && <p className="mt-2 max-w-[42ch] text-[15px] text-[#5B6075]">{body}</p>}
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded-xl bg-[#E7EAF5]", className)} />;
}

export function Modal({
  open,
  onClose,
  title,
  description,
  subheader,
  children,
  footer,
  width = "max-w-lg",
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  description?: ReactNode;
  subheader?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  width?: string;
}) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    const prev = document.activeElement as HTMLElement | null;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onCloseRef.current();
    document.addEventListener("keydown", onKey);
    const first = panelRef.current?.querySelector<HTMLElement>("input, select, textarea, button:not([data-close])");
    first?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      prev?.focus?.();
    };
  }, [open]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-[#14161F]/30 backdrop-blur-[2px]" onClick={onClose} />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={cx(
          "relative flex max-h-[86dvh] w-full flex-col rounded-[24px] bg-white shadow-[0_40px_80px_-30px_rgba(20,22,31,0.5)]",
          width,
        )}
      >
        <div className="flex shrink-0 items-start justify-between gap-4 border-b border-[#EEF0F7] px-6 py-4">
          <div className="min-w-0 flex-1">
            <h2 id={titleId} className="text-base font-semibold text-[#14161F]">
              {title}
            </h2>
            {description && <p className="mt-0.5 text-xs text-[#5B6075]">{description}</p>}
          </div>
          <IconButton label="关闭" onClick={onClose} data-close className="bg-transparent">
            <X className="h-4 w-4" />
          </IconButton>
        </div>
        {subheader && (
          <div className="shrink-0 border-b border-[#EEF0F7] bg-white px-6 py-3.5">{subheader}</div>
        )}
        {children && (
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain [scrollbar-gutter:stable] px-6 py-4">
            {children}
          </div>
        )}
        {footer && <div className="flex shrink-0 justify-end gap-3 border-t border-[#EEF0F7] px-6 py-3.5">{footer}</div>}
      </div>
    </div>
  );
}

export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel,
  danger,
  loading,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: ReactNode;
  body?: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      description={body}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button variant={danger ? "danger" : "primary"} loading={loading} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </>
      }
    />
  );
}

export interface MenuItem {
  label: string;
  icon?: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  onSelect: () => void;
}

/** Three-dot overflow menu rendered via a portal so overflow-x-auto / overflow-hidden tables never clip it. */
export function MoreMenu({
  items,
  label = "更多操作",
  align = "right",
  disabled,
  className,
}: {
  items: MenuItem[];
  label?: string;
  align?: "left" | "right";
  disabled?: boolean;
  className?: string;
}) {
  const triggerId = useId();
  const [open, setOpen] = useState(false);
  const [coords, setCoords] = useState<{
    top?: number;
    bottom?: number;
    left?: number;
    right?: number;
  } | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const enterOnOpenRef = useRef(false);
  const hasNativeFocusGroup =
    typeof HTMLElement !== "undefined" && "focusGroup" in HTMLElement.prototype;
  const hasAnyIcon = items.some((x) => Boolean(x.icon));

  const focusItem = useCallback((target: "first" | "last" | number) => {
    const el = menuRef.current;
    if (!el) return;
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>("button")).filter(
      (btn) => !btn.disabled && btn.getAttribute("aria-disabled") !== "true",
    );
    if (buttons.length === 0) return;
    if (target === "first") {
      buttons[0]?.focus({ preventScroll: true });
    } else if (target === "last") {
      buttons[buttons.length - 1]?.focus({ preventScroll: true });
    } else {
      buttons[target]?.focus({ preventScroll: true });
    }
  }, []);

  const updatePosition = useCallback(() => {
    const el = ref.current;
    if (!el || typeof window === "undefined") return;
    const rect = el.getBoundingClientRect();
    const estimatedHeight = items.length * 40 + 24;
    const spaceBelow = window.innerHeight - rect.bottom;
    const openUpward = spaceBelow < estimatedHeight && rect.top > spaceBelow;

    setCoords({
      ...(openUpward
        ? { bottom: Math.max(8, window.innerHeight - rect.top + 8) }
        : { top: rect.bottom + 8 }),
      ...(align === "right"
        ? { right: Math.max(8, window.innerWidth - rect.right) }
        : { left: Math.max(8, rect.left) }),
    });
  }, [align, items.length]);

  useEffect(() => {
    if (!open) return;
    updatePosition();
    let rafId = 0;
    if (enterOnOpenRef.current) {
      enterOnOpenRef.current = false;
      rafId = window.requestAnimationFrame(() => focusItem("first"));
    }
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!ref.current?.contains(target) && !menuRef.current?.contains(target)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        setOpen(false);
        triggerRef.current?.focus({ preventScroll: true });
      } else if (e.key === "Tab") {
        setOpen(false);
      }
    };
    const onFocusIn = (e: FocusEvent) => {
      const target = e.target as Node | null;
      if (target && !ref.current?.contains(target) && !menuRef.current?.contains(target)) {
        setOpen(false);
      }
    };
    const onScrollOrResize = () => updatePosition();
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("focusin", onFocusIn);
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
    return () => {
      if (rafId) window.cancelAnimationFrame(rafId);
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("focusin", onFocusIn);
      window.removeEventListener("scroll", onScrollOrResize, true);
      window.removeEventListener("resize", onScrollOrResize);
    };
  }, [open, updatePosition, focusItem]);

  if (items.length === 0) return null;

  return (
    <div ref={ref} className="relative inline-flex" onClick={(e) => e.stopPropagation()}>
      <IconButton
        ref={triggerRef}
        id={triggerId}
        label={label}
        disabled={disabled}
        aria-haspopup="menu"
        aria-expanded={open}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") {
            e.preventDefault();
            if (!open) {
              enterOnOpenRef.current = true;
              updatePosition();
              setOpen(true);
            } else {
              focusItem("first");
            }
          }
        }}
        onClick={(e) => {
          if (!open) {
            enterOnOpenRef.current = e.detail === 0;
            updatePosition();
          }
          setOpen((o) => !o);
        }}
        className={cx(open && "bg-[#E3E7FA] text-[#2B3AE8]", className)}
      >
        <MoreHorizontal className="h-4 w-4" />
      </IconButton>
      {open &&
        coords &&
        typeof document !== "undefined" &&
        createPortal(
          <div
            ref={menuRef}
            role={hasNativeFocusGroup ? undefined : "menu"}
            aria-labelledby={triggerId}
            {...({ focusgroup: "menu nomemory" } as Record<string, string>)}
            style={coords}
            onClick={(e) => e.stopPropagation()}
            onKeyDownCapture={(e) => {
              if (hasNativeFocusGroup) return;
              if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
              const container = menuRef.current;
              if (!container) return;
              const enabledButtons = Array.from(
                container.querySelectorAll<HTMLButtonElement>("button"),
              ).filter((btn) => !btn.disabled && btn.getAttribute("aria-disabled") !== "true");
              if (enabledButtons.length === 0) return;
              const current = enabledButtons.indexOf(
                document.activeElement as HTMLButtonElement,
              );
              let next = 0;
              if (e.key === "Home") {
                next = 0;
              } else if (e.key === "End") {
                next = enabledButtons.length - 1;
              } else if (current < 0) {
                next = e.key === "ArrowUp" ? enabledButtons.length - 1 : 0;
              } else {
                next =
                  (current + (e.key === "ArrowUp" ? -1 : 1) + enabledButtons.length) %
                  enabledButtons.length;
              }
              e.preventDefault();
              e.stopPropagation();
              enabledButtons[next]?.focus({ preventScroll: true });
            }}
            className="fixed z-[110] min-w-[168px] rounded-2xl border border-[#E3E6F2] bg-white p-1.5 shadow-[0_20px_40px_-20px_rgba(20,22,31,0.35)]"
          >
            {items.map((it) => (
              <button
                key={it.label}
                type="button"
                role={hasNativeFocusGroup ? undefined : "menuitem"}
                disabled={it.disabled}
                onPointerMove={(e) => {
                  if (!it.disabled && document.activeElement !== e.currentTarget) {
                    e.currentTarget.focus({ preventScroll: true, focusVisible: false } as FocusOptions);
                  }
                }}
                onClick={(e) => {
                  const fromPointer = e.detail > 0;
                  setOpen(false);
                  triggerRef.current?.focus({
                    preventScroll: true,
                    ...(fromPointer ? { focusVisible: false } : {}),
                  } as FocusOptions);
                  it.onSelect();
                }}
                className={cx(
                  "flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50",
                  it.danger
                    ? "text-[#D92D20] hover:bg-[#FEF3F2] focus:bg-[#FEF3F2]"
                    : "text-[#14161F] hover:bg-[#F1F3FB] focus:bg-[#F1F3FB]",
                  focusRing,
                )}
              >
                {hasAnyIcon && (
                  <span
                    className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
                    aria-hidden="true"
                  >
                    {it.icon}
                  </span>
                )}
                {it.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </div>
  );
}
