"use client";

import { createContext, useContext, useState, useCallback, type ReactNode } from "react";
import { CheckCircle2, AlertTriangle, Info, X } from "lucide-react";
import { cn } from "@/lib/utils";

export interface ToastItem {
  id: string;
  type: "success" | "error" | "info" | "warning";
  title: string;
  description?: string;
  duration?: number;
}

interface ToastContextValue {
  showToast: (toast: Omit<ToastItem, "id">) => void;
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const showToast = useCallback(
    ({ type, title, description, duration = 3000 }: Omit<ToastItem, "id">) => {
      const id = Math.random().toString(36).substring(2, 9);
      setToasts((prev) => [...prev, { id, type, title, description, duration }]);
      setTimeout(() => {
        removeToast(id);
      }, duration);
    },
    [removeToast]
  );

  const success = useCallback((title: string, description?: string) => {
    showToast({ type: "success", title, description });
  }, [showToast]);

  const error = useCallback((title: string, description?: string) => {
    showToast({ type: "error", title, description });
  }, [showToast]);

  const info = useCallback((title: string, description?: string) => {
    showToast({ type: "info", title, description });
  }, [showToast]);

  return (
    <ToastContext.Provider value={{ showToast, success, error, info }}>
      {children}
      {/* Fixed Toast Container */}
      <div className="fixed bottom-5 right-5 z-[999] flex flex-col gap-2.5 max-w-sm pointer-events-none">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={cn(
              "pointer-events-auto flex items-start gap-3 rounded-xl border p-3.5 shadow-2xl backdrop-blur-xl transition-all animate-in slide-in-from-bottom-5 duration-200",
              toast.type === "success"
                ? "border-panel-green/40 bg-slate-900/95 text-slate-100 shadow-[0_10px_30px_rgba(34,197,94,0.15)]"
                : toast.type === "error"
                ? "border-rose-500/40 bg-slate-900/95 text-slate-100 shadow-[0_10px_30px_rgba(244,63,94,0.15)]"
                : "border-slate-700 bg-slate-900/95 text-slate-100 shadow-xl"
            )}
          >
            <div className="shrink-0 mt-0.5">
              {toast.type === "success" ? (
                <CheckCircle2 className="size-4 text-panel-green" />
              ) : toast.type === "error" ? (
                <AlertTriangle className="size-4 text-rose-400" />
              ) : (
                <Info className="size-4 text-sky-400" />
              )}
            </div>

            <div className="min-w-0 flex-1">
              <p className="text-xs font-bold text-white tracking-tight">{toast.title}</p>
              {toast.description && (
                <p className="mt-0.5 text-[11px] text-slate-400 leading-relaxed">{toast.description}</p>
              )}
            </div>

            <button
              type="button"
              onClick={() => removeToast(toast.id)}
              className="shrink-0 rounded p-0.5 text-slate-500 hover:text-slate-300 transition"
            >
              <X className="size-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    return {
      showToast: () => {},
      success: () => {},
      error: () => {},
      info: () => {}
    };
  }
  return context;
}
