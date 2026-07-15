"use client";

import { X } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";
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
  confirmDisabled,
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
  confirmDisabled?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const onCancelRef = useRef(onCancel);
  const busyRef = useRef(Boolean(busy));

  useEffect(() => {
    onCancelRef.current = onCancel;
    busyRef.current = Boolean(busy);
  }, [busy, onCancel]);

  useEffect(() => {
    if (!open) return;
    const previouslyFocused = document.activeElement;
    const previouslyOverflow = document.body.style.overflow;
    const inerted = new Map<HTMLElement, boolean>();
    let branch: HTMLElement | null = overlayRef.current;
    while (branch?.parentElement) {
      const parent = branch.parentElement;
      for (const sibling of Array.from(parent.children)) {
        if (sibling !== branch && sibling instanceof HTMLElement) {
          inerted.set(sibling, sibling.inert);
          sibling.inert = true;
        }
      }
      if (parent === document.body) break;
      branch = parent;
    }
    document.body.style.overflow = "hidden";

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busyRef.current) {
        onCancelRef.current();
        return;
      }
      if (event.key !== "Tab") return;
      const dialog = dialogRef.current;
      if (!dialog) return;
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      ));
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0]!;
      const last = focusable[focusable.length - 1]!;
      if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    const focusFrame = window.requestAnimationFrame(() => dialogRef.current?.focus());
    return () => {
      window.cancelAnimationFrame(focusFrame);
      window.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = previouslyOverflow;
      for (const [element, wasInert] of inerted) element.inert = wasInert;
      if (previouslyFocused instanceof HTMLElement && previouslyFocused.isConnected) previouslyFocused.focus();
    };
  }, [open]);

  if (!open) {
    return null;
  }

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 px-4 backdrop-blur-sm"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !busy) onCancel();
      }}
    >
      <div
        ref={dialogRef}
        aria-describedby="confirm-dialog-description"
        aria-labelledby="confirm-dialog-title"
        aria-modal="true"
        className="w-full max-w-md rounded-lg border border-panel-line bg-panel-card p-5 shadow-[0_12px_40px_rgba(0,0,0,0.35)]"
        role="dialog"
        tabIndex={-1}
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
          <Button variant={confirmVariant} onClick={onConfirm} disabled={Boolean(busy || confirmDisabled)}>{confirmLabel}</Button>
        </div>
      </div>
    </div>
  );
}
