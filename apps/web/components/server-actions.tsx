"use client";

import { Copy, Ellipsis, Globe2, Play, RotateCcw, Settings, Square, Trash2, X } from "lucide-react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { useEffect, useRef, useState } from "react";
import { Button, ToastNotice } from "@/components/ui";
import { serverActionRedirectPath } from "@/lib/server-action-flow";
import { copyText } from "@/lib/clipboard";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { formatServerDetailError } from "@/lib/server-detail-actions";
import { serverInviteText } from "@/lib/server-join";
import type { GameServerResource } from "@/lib/types";
import { gameServerAction } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

export function ServerActions({
  server,
  showInvite = true,
  showDelete = true,
  compact = false,
  rowMode = false,
  disabled = false,
  regenerationBusy = false,
  onRegenerateWorld,
  className
}: {
  server: GameServerResource;
  showInvite?: boolean;
  showDelete?: boolean;
  compact?: boolean;
  rowMode?: boolean;
  disabled?: boolean;
  regenerationBusy?: boolean;
  onRegenerateWorld?: () => void;
  className?: string;
}) {
  const client = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const { t, locale } = useI18n();
  const isZh = locale === "zh";
  const { isViewer, canDeleteServer } = usePermissions();
  const [pendingAction, setPendingAction] = useState<"stop" | "restart" | "delete" | null>(null);
  const [busyAction, setBusyAction] = useState<"start" | "stop" | "restart" | "delete" | null>(null);
  const [copiedInvite, setCopiedInvite] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [moreOpen, setMoreOpen] = useState(false);
  const [morePosition, setMorePosition] = useState({ left: 0, top: 0 });
  const noticeTimerRef = useRef<number | null>(null);
  const moreButtonRef = useRef<HTMLButtonElement>(null);
  const moreMenuRef = useRef<HTMLDivElement>(null);
  const status = gameServerStatus(server);
  const lifecycleBusy = status === "creating" || status === "starting" || status === "stopping" || status === "restarting" || status === "deleting";
  const controlsDisabled = disabled || Boolean(busyAction) || lifecycleBusy || isViewer;
  const canDelete = (status === "stopped" || status === "errored") && canDeleteServer;
  const canShowDelete = showDelete && canDeleteServer;
  // Restart is a refresh-style launch operation: a running server is recreated,
  // while a stopped or failed server is started through the same refresh path.
  const showRowRestart = rowMode;
  const actionLabel = (action: "start" | "stop" | "restart" | "delete") =>
    action === "start" ? t("actionStart") : action === "stop" ? t("actionStop") : action === "restart" ? t("actionRestart") : t("delete");
  const successLabel = (action: "start" | "stop" | "restart" | "delete") =>
    action === "start" ? t("serverStartQueued") : action === "stop" ? t("serverStopQueued") : action === "restart" ? t("serverRestartQueued") : t("serverDeleteQueued");
  const startLabel = busyAction === "start" || status === "starting" || status === "creating" ? t("actionStarting") : t("actionStart");
  const stopLabel = busyAction === "stop" || status === "stopping" ? t("actionStopping") : t("actionStop");
  const restartLabel = busyAction === "restart" || status === "restarting" ? t("actionRestarting") : t("actionRestart");
  const deleteLabel = busyAction === "delete" || status === "deleting" ? t("actionDeleting") : t("delete");

  useEffect(() => {
    if (!pendingAction) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busyAction) {
        setPendingAction(null);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [busyAction, pendingAction]);

  useEffect(() => {
    if (!moreOpen) return;
    const closeMenu = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!moreButtonRef.current?.contains(target) && !moreMenuRef.current?.contains(target)) setMoreOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMoreOpen(false);
    };
    const closeOnViewportChange = () => setMoreOpen(false);
    document.addEventListener("pointerdown", closeMenu);
    window.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", closeOnViewportChange);
    window.addEventListener("scroll", closeOnViewportChange, true);
    return () => {
      document.removeEventListener("pointerdown", closeMenu);
      window.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", closeOnViewportChange);
      window.removeEventListener("scroll", closeOnViewportChange, true);
    };
  }, [moreOpen]);

  const toggleMoreMenu = () => {
    if (controlsDisabled) return;
    if (moreOpen) {
      setMoreOpen(false);
      return;
    }
    const rect = moreButtonRef.current?.getBoundingClientRect();
    if (!rect) return;
    const menuWidth = 144;
    const visibleActionCount = Number(showRowRestart) + Number(Boolean(onRegenerateWorld)) + Number(canShowDelete);
    const hasSeparatedDelete = canShowDelete && (showRowRestart || Boolean(onRegenerateWorld));
    const estimatedHeight = visibleActionCount * 32 + 8 + (hasSeparatedDelete ? 9 : 0);
    setMorePosition({
      left: Math.max(8, Math.min(window.innerWidth - menuWidth - 8, rect.right - menuWidth)),
      top: rect.bottom + estimatedHeight + 6 <= window.innerHeight ? rect.bottom + 6 : Math.max(8, rect.top - estimatedHeight - 6)
    });
    setMoreOpen(true);
  };

  useEffect(() => {
    return () => {
      if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
    };
  }, []);

  const showNotice = (tone: "success" | "error", message: string) => {
    if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
    setErrorMessage(tone === "error" ? message : "");
    setSuccessMessage(tone === "success" ? message : "");
    noticeTimerRef.current = window.setTimeout(() => {
      setErrorMessage("");
      setSuccessMessage("");
    }, tone === "success" ? 3000 : 6000);
  };

  const executeAction = async (action: "start" | "stop" | "restart" | "delete") => {
    if (isViewer) {
      showNotice("error", "当前账号为只读访客，无权执行操作");
      return;
    }
    setBusyAction(action);
    setErrorMessage("");
    setSuccessMessage("");
    try {
      const updatedServer = await gameServerAction(server.id, action);
      setPendingAction(null);
      if (updatedServer) {
        client.setQueryData(["game-server", server.id], updatedServer);
      }
      await client.invalidateQueries({ queryKey: ["game-server", server.id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
      showNotice("success", successLabel(action));
      const redirectPath = serverActionRedirectPath(action, pathname, server.id);
      if (redirectPath) {
        router.push(redirectPath);
      }
    } catch (error) {
      const message = formatServerDetailError(error, {
        dockerUnavailable: t("detailDockerUnavailable"),
        containerUnavailable: t("detailContainerUnavailable"),
        portAlreadyAllocated: (port) => t("detailPortAlreadyAllocated", { port })
      });
      showNotice("error", message || t("unableAction", { action: actionLabel(action) }));
    } finally {
      setBusyAction(null);
    }
  };

  const runAction = (action: "start" | "stop" | "restart" | "delete") => {
    if (action === "delete" && !canDelete) {
      showNotice("error", t("deleteRequiresStopped"));
      return;
    }
    if (action === "stop" || action === "restart" || action === "delete") {
      setErrorMessage("");
      setSuccessMessage("");
      setPendingAction(action);
      return;
    }
    void executeAction(action);
  };

  const copyInvite = async () => {
    setErrorMessage("");
    setSuccessMessage("");
    try {
      await copyText(serverInviteText(server));
      setCopiedInvite(true);
      window.setTimeout(() => setCopiedInvite(false), 1500);
    } catch (error) {
      setCopiedInvite(false);
      showNotice("error", error instanceof Error ? error.message : t("copyInviteFailed"));
    }
  };

  const pendingLabel = pendingAction ? actionLabel(pendingAction) : "";
  const buttonClassName = rowMode
    ? "h-8 min-w-0 whitespace-nowrap px-2.5 text-xs"
    : compact
      ? "h-10 w-full min-w-0 whitespace-nowrap px-2 text-sm"
      : undefined;

  if (isViewer) return null;

  return (
    <>
      <div className={cn(rowMode ? "flex flex-nowrap items-center justify-end gap-1" : compact ? "grid grid-cols-2 gap-2 md:grid-cols-4" : "flex flex-wrap gap-2", className)}>
        {rowMode ? (
          <>
            {/* Restart icon-only button */}
            <button
              type="button"
              title={restartLabel}
              aria-label={restartLabel}
              onClick={() => runAction("restart")}
              disabled={controlsDisabled}
              className="flex size-7 items-center justify-center rounded hover:bg-slate-100 text-slate-500 hover:text-slate-900 transition disabled:opacity-30"
            >
              <RotateCcw className="size-3.5" />
            </button>

            {/* Start / Stop icon-only button */}
            {status === "running" || status === "stopping" ? (
              <button
                type="button"
                title={stopLabel}
                aria-label={stopLabel}
                onClick={() => runAction("stop")}
                disabled={controlsDisabled}
                className="flex size-7 items-center justify-center rounded hover:bg-amber-50 text-slate-500 hover:text-amber-600 transition disabled:opacity-30"
              >
                <Square className="size-3.5" />
              </button>
            ) : (
              <button
                type="button"
                title={startLabel}
                aria-label={startLabel}
                onClick={() => runAction("start")}
                disabled={controlsDisabled}
                className="flex size-7 items-center justify-center rounded hover:bg-emerald-50 text-emerald-600 transition disabled:opacity-30"
              >
                <Play className="size-3.5 fill-current" />
              </button>
            )}

            {/* Config & Detail Link icon-only button */}
            <Link
              href={`/servers/${server.id}`}
              title={isZh ? "参数配置与详情" : "Configuration & Details"}
              aria-label={isZh ? "参数配置与详情" : "Configuration & Details"}
              className="flex size-7 items-center justify-center rounded hover:bg-slate-100 text-slate-600 hover:text-slate-900 transition"
            >
              <Settings className="size-3.5" />
            </Link>
          </>
        ) : (
          <>
            {status === "running" || status === "stopping" ? (
              <Button className={buttonClassName} variant="danger" onClick={() => runAction("stop")} disabled={controlsDisabled}>
                <Square aria-hidden="true" />
                {stopLabel}
              </Button>
            ) : (
              <Button
                className={cn(
                  "border border-emerald-500/30 bg-emerald-50 text-emerald-700 hover:bg-emerald-100 disabled:opacity-40",
                  buttonClassName
                )}
                variant="ghost"
                onClick={() => runAction("start")}
                disabled={controlsDisabled}
              >
                <Play aria-hidden="true" />
                {startLabel}
              </Button>
            )}
            <Button className={buttonClassName} variant="secondary" onClick={() => runAction("restart")} disabled={controlsDisabled}>
              <RotateCcw aria-hidden="true" />
              {restartLabel}
            </Button>
            {showInvite && (
              <Button className={buttonClassName} variant="secondary" onClick={() => void copyInvite()} disabled={status === "deleting"}>
                <Copy aria-hidden="true" />
                {copiedInvite ? t("copied") : t("actionCopyInvite")}
              </Button>
            )}
          </>
        )}

        {rowMode || onRegenerateWorld || canShowDelete ? (
          <button
            aria-expanded={moreOpen}
            aria-haspopup="menu"
            aria-label={t("serverMoreActions")}
            className={cn(
              "flex items-center justify-center rounded text-slate-400 transition hover:bg-slate-100 hover:text-slate-700 focus:outline-none",
              moreOpen && "bg-slate-100 text-slate-900",
              rowMode ? "size-7 px-0" : "h-10 px-3 border micro-border",
              compact && !rowMode && "col-span-2 w-full md:col-span-1",
              controlsDisabled && "cursor-not-allowed opacity-40"
            )}
            disabled={controlsDisabled}
            onClick={toggleMoreMenu}
            ref={moreButtonRef}
            title={t("serverMoreActions")}
            type="button"
          >
            <Ellipsis aria-hidden="true" className="size-3.5" />
          </button>
        ) : null}
      </div>

      {moreOpen && typeof document !== "undefined" ? createPortal(
        <div
          className="fixed z-[70] w-36 rounded-xl border micro-border bg-white p-1 shadow-xl text-slate-700 animate-in fade-in zoom-in-95 duration-100"
          ref={moreMenuRef}
          role="menu"
          style={{ left: morePosition.left, top: morePosition.top }}
        >
          {showRowRestart && !rowMode ? (
            <button
              className="flex h-7 w-full items-center gap-2 rounded-lg px-2 text-left text-xs text-slate-700 transition hover:bg-slate-50 focus:outline-none disabled:opacity-40"
              disabled={controlsDisabled}
              onClick={() => {
                setMoreOpen(false);
                runAction("restart");
              }}
              role="menuitem"
              type="button"
            >
              <RotateCcw aria-hidden="true" className="size-3.5 text-slate-400" />
              {restartLabel}
            </button>
          ) : null}
          {onRegenerateWorld ? (
            <button
              className="flex h-7 w-full items-center gap-2 rounded-lg px-2 text-left text-xs text-amber-700 transition hover:bg-amber-50 focus:outline-none disabled:opacity-40"
              disabled={controlsDisabled || regenerationBusy}
              onClick={() => {
                setMoreOpen(false);
                onRegenerateWorld();
              }}
              role="menuitem"
              type="button"
            >
              <Globe2 aria-hidden="true" className="size-3.5" />
              {regenerationBusy ? t("worldRegenerationProgress") : t("worldRegenerateAction")}
            </button>
          ) : null}
          {canShowDelete && (onRegenerateWorld || (showRowRestart && !rowMode)) ? <div className="mx-1 my-1 border-t border-slate-100" /> : null}
          {canShowDelete ? (
            <button
              className="flex h-7 w-full items-center gap-2 rounded-lg px-2 text-left text-xs text-rose-600 transition hover:bg-rose-50 focus:outline-none disabled:opacity-40"
              disabled={controlsDisabled || !canDelete}
              onClick={() => {
                setMoreOpen(false);
                runAction("delete");
              }}
              role="menuitem"
              title={!canDelete ? t("deleteRequiresStopped") : undefined}
              type="button"
            >
              <Trash2 aria-hidden="true" className="size-3.5" />
              {deleteLabel}
            </button>
          ) : null}
        </div>,
        document.body
      ) : null}

      {(errorMessage || successMessage) && (
        <div className="pointer-events-none fixed inset-x-4 bottom-4 z-[60] flex justify-end md:inset-x-auto md:bottom-auto md:right-6 md:top-24">
          <ToastNotice
            closeLabel={t("cancel")}
            message={errorMessage || successMessage}
            tone={errorMessage ? "error" : "success"}
            onClose={() => {
              if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
              setErrorMessage("");
              setSuccessMessage("");
            }}
          />
        </div>
      )}

      {pendingAction && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 px-4 backdrop-blur-xs animate-in fade-in duration-150"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget && !busyAction) setPendingAction(null);
          }}
        >
          <div
            aria-describedby="server-action-confirm-description"
            aria-labelledby="server-action-confirm-title"
            aria-modal="true"
            className="w-full max-w-md rounded-2xl border micro-border bg-white p-5 shadow-2xl space-y-4 animate-in zoom-in-95 duration-150"
            role="dialog"
          >
            <div className="flex items-start justify-between gap-4 border-b border-slate-100 pb-3">
              <div>
                <p className="text-xs font-bold text-amber-600">{t("destructiveAction")}</p>
                <h2 className="mt-1 text-base font-bold text-slate-900" id="server-action-confirm-title">
                  {t("confirmServerActionTitle", { action: pendingLabel })}
                </h2>
              </div>
              <button
                aria-label={t("cancel")}
                className="flex size-7 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 hover:text-slate-700 transition"
                disabled={Boolean(busyAction)}
                onClick={() => setPendingAction(null)}
                type="button"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            </div>
            <p className="text-xs leading-relaxed text-slate-600" id="server-action-confirm-description">
              {pendingAction === "delete"
                ? t("confirmServerDeleteDescription", { name: server.name })
                : t("confirmServerActionDescription", { action: pendingLabel, name: server.name })}
            </p>
            <div className="rounded-lg border micro-border bg-slate-50/70 px-3 py-2 text-xs">
              <span className="text-slate-400">{t("server")}: </span>
              <span className="font-mono font-bold text-slate-800">{server.name}</span>
            </div>
            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end pt-2 border-t border-slate-100">
              <Button variant="secondary" onClick={() => setPendingAction(null)} disabled={Boolean(busyAction)}>
                {t("cancel")}
              </Button>
              <Button
                variant={pendingAction === "restart" ? "gold" : "danger"}
                onClick={() => void executeAction(pendingAction)}
                disabled={Boolean(busyAction)}
              >
                {busyAction ? t("actionWorking") : t("confirmServerActionButton", { action: pendingLabel })}
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
