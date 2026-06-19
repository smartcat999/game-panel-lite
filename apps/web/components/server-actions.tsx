"use client";

import { Copy, Play, RotateCcw, Square, Trash2, X } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui";
import { serverActionRedirectPath } from "@/lib/server-action-flow";
import { useI18n } from "@/lib/i18n";
import { formatServerDetailError } from "@/lib/server-detail-actions";
import { serverInviteText } from "@/lib/server-join";
import type { Server } from "@/lib/types";
import { serverAction } from "@/lib/api";
import { cn } from "@/lib/utils";

export function ServerActions({
  server,
  showInvite = true,
  showDelete = true,
  compact = false,
  className
}: {
  server: Server;
  showInvite?: boolean;
  showDelete?: boolean;
  compact?: boolean;
  className?: string;
}) {
  const client = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const { t } = useI18n();
  const [pendingAction, setPendingAction] = useState<"stop" | "restart" | "delete" | null>(null);
  const [busyAction, setBusyAction] = useState<"start" | "stop" | "restart" | "delete" | null>(null);
  const [copiedInvite, setCopiedInvite] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const lifecycleBusy = server.status === "creating" || server.status === "starting" || server.status === "stopping" || server.status === "restarting" || server.status === "deleting";
  const controlsDisabled = Boolean(busyAction) || lifecycleBusy;
  const actionLabel = (action: "start" | "stop" | "restart" | "delete") =>
    action === "start" ? t("actionStart") : action === "stop" ? t("actionStop") : action === "restart" ? t("actionRestart") : t("delete");
  const successLabel = (action: "start" | "stop" | "restart" | "delete") =>
    action === "start" ? t("serverStartQueued") : action === "stop" ? t("serverStopQueued") : action === "restart" ? t("serverRestartQueued") : t("serverDeleteQueued");
  const startLabel = busyAction === "start" || server.status === "starting" || server.status === "creating" ? t("actionStarting") : t("actionStart");
  const stopLabel = busyAction === "stop" || server.status === "stopping" ? t("actionStopping") : t("actionStop");
  const restartLabel = busyAction === "restart" || server.status === "restarting" ? t("actionRestarting") : t("actionRestart");
  const deleteLabel = busyAction === "delete" || server.status === "deleting" ? t("actionDeleting") : t("delete");

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

  const executeAction = async (action: "start" | "stop" | "restart" | "delete") => {
    setBusyAction(action);
    setErrorMessage("");
    setSuccessMessage("");
    try {
      const updatedServer = await serverAction(server.id, action);
      setPendingAction(null);
      if (updatedServer) {
        client.setQueryData(["server", server.id], updatedServer);
      }
      await client.invalidateQueries({ queryKey: ["servers"] });
      await client.invalidateQueries({ queryKey: ["server", server.id] });
      setSuccessMessage(successLabel(action));
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
      setErrorMessage(message || t("unableAction", { action: actionLabel(action) }));
    } finally {
      setBusyAction(null);
    }
  };

  const runAction = (action: "start" | "stop" | "restart" | "delete") => {
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
      await navigator.clipboard.writeText(serverInviteText(server));
      setCopiedInvite(true);
      window.setTimeout(() => setCopiedInvite(false), 1500);
    } catch (error) {
      setCopiedInvite(false);
      setErrorMessage(error instanceof Error ? error.message : t("copyInviteFailed"));
    }
  };

  const pendingLabel = pendingAction ? actionLabel(pendingAction) : "";
  const buttonClassName = compact ? "h-10 w-full min-w-0 whitespace-nowrap px-2 text-sm" : undefined;

  return (
    <>
      <div className={cn(compact ? "grid grid-cols-2 gap-2 md:grid-cols-4" : "flex flex-wrap gap-2", className)}>
        {server.status === "running" || server.status === "stopping" ? (
          <Button className={buttonClassName} variant="danger" onClick={() => runAction("stop")} disabled={controlsDisabled}>
            <Square aria-hidden="true" />
            {stopLabel}
          </Button>
        ) : (
          <Button
            className={cn(
              "border border-panel-green/30 bg-panel-green/10 text-panel-green hover:border-panel-green/50 hover:bg-panel-green/15 disabled:border-panel-line disabled:bg-slate-900/70 disabled:text-slate-500",
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
          <Button className={buttonClassName} variant="secondary" onClick={() => void copyInvite()} disabled={server.status === "deleting"}>
            <Copy aria-hidden="true" />
            {copiedInvite ? t("copied") : t("actionCopyInvite")}
          </Button>
        )}
        {showDelete && (
          <Button className={buttonClassName} variant="danger" onClick={() => runAction("delete")} disabled={controlsDisabled}>
            <Trash2 aria-hidden="true" />
            {deleteLabel}
          </Button>
        )}
      </div>
      {errorMessage && <p className="mt-2 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mt-2 text-sm text-panel-green">{successMessage}</p>}
      {pendingAction && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 px-4 backdrop-blur-sm"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget && !busyAction) setPendingAction(null);
          }}
        >
          <div
            aria-describedby="server-action-confirm-description"
            aria-labelledby="server-action-confirm-title"
            aria-modal="true"
            className="w-full max-w-md rounded-lg border border-panel-line bg-panel-card p-5 shadow-[0_12px_40px_rgba(0,0,0,0.35)]"
            role="dialog"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-panel-gold">{t("destructiveAction")}</p>
                <h2 className="mt-2 text-lg font-semibold text-white" id="server-action-confirm-title">
                  {t("confirmServerActionTitle", { action: pendingLabel })}
                </h2>
              </div>
              <button
                aria-label={t("cancel")}
                className="flex size-8 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                disabled={Boolean(busyAction)}
                onClick={() => setPendingAction(null)}
                type="button"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            </div>
            <p className="mt-3 text-sm leading-6 text-slate-400" id="server-action-confirm-description">
              {t("confirmServerActionDescription", { action: pendingLabel, name: server.name })}
            </p>
            <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-sm">
              <span className="text-slate-500">{t("server")}: </span>
              <span className="font-medium text-white">{server.name}</span>
            </div>
            <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
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
