"use client";

import { X } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { Button } from "@/components/ui";

export function ConfirmDialog({
  open,
  eyebrow,
  title,
  description,
  detail,
  cancelLabel,
  confirmLabel,
  confirmVariant = "danger",
  busy,
  onCancel,
  onConfirm
}: {
  open: boolean;
  eyebrow: string;
  title: string;
  description: string;
  detail?: ReactNode;
  cancelLabel: string;
  confirmLabel: string;
  confirmVariant?: "danger" | "gold";
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        onCancel();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [busy, onCancel, open]);

  if (!open) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 px-4 backdrop-blur-sm"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !busy) onCancel();
      }}
    >
      <div
        aria-describedby="confirm-dialog-description"
        aria-labelledby="confirm-dialog-title"
        aria-modal="true"
        className="w-full max-w-md rounded-lg border border-panel-line bg-panel-card p-5 shadow-[0_12px_40px_rgba(0,0,0,0.35)]"
        role="dialog"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-panel-gold">{eyebrow}</p>
            <h2 className="mt-2 text-lg font-semibold text-white" id="confirm-dialog-title">{title}</h2>
          </div>
          <button
            aria-label={cancelLabel}
            className="flex size-8 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            disabled={Boolean(busy)}
            onClick={onCancel}
            type="button"
          >
            <X aria-hidden="true" className="size-4" />
          </button>
        </div>
        <p className="mt-3 text-sm leading-6 text-slate-400" id="confirm-dialog-description">{description}</p>
        {detail && <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-sm">{detail}</div>}
        <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="secondary" onClick={onCancel} disabled={Boolean(busy)}>{cancelLabel}</Button>
          <Button variant={confirmVariant} onClick={onConfirm} disabled={Boolean(busy)}>{confirmLabel}</Button>
        </div>
      </div>
    </div>
  );
}
