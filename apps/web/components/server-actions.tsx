"use client";

import { Copy, Play, RotateCcw, Square } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import type { Server } from "@/lib/types";
import { serverAction } from "@/lib/api";

export function ServerActions({ server }: { server: Server }) {
  const client = useQueryClient();
  const router = useRouter();
  const { t } = useI18n();
  const actionLabel = (action: "start" | "stop" | "restart" | "delete") =>
    action === "start" ? t("actionStart") : action === "stop" ? t("actionStop") : action === "restart" ? t("actionRestart") : t("delete");
  const runAction = async (action: "start" | "stop" | "restart" | "delete") => {
    if ((action === "stop" || action === "restart" || action === "delete") && !window.confirm(t("confirmAction", { action: actionLabel(action), name: server.name }))) {
      return;
    }
    try {
      await serverAction(server.id, action);
      await client.invalidateQueries({ queryKey: ["servers"] });
      await client.invalidateQueries({ queryKey: ["server", server.id] });
      router.refresh();
    } catch (error) {
      window.alert(error instanceof Error ? error.message : t("unableAction", { action: actionLabel(action) }));
    }
  };
  return (
    <div className="flex flex-wrap gap-2">
      {server.status === "running" ? (
        <Button variant="danger" onClick={() => void runAction("stop")}>
          <Square aria-hidden="true" />
          {t("actionStop")}
        </Button>
      ) : (
        <Button onClick={() => void runAction("start")}>
          <Play aria-hidden="true" />
          {t("actionStart")}
        </Button>
      )}
      <Button variant="secondary" onClick={() => void runAction("restart")}>
        <RotateCcw aria-hidden="true" />
        {t("actionRestart")}
      </Button>
      <Button variant="primary" onClick={() => void navigator.clipboard.writeText(`Join ${server.name} at 127.0.0.1:${server.port}`)}>
        <Copy aria-hidden="true" />
        {t("actionCopyInvite")}
      </Button>
    </div>
  );
}
