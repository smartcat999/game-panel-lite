"use client";

import { Copy, Play, RotateCcw, Square } from "lucide-react";
import { Button } from "@/components/ui";
import type { Server } from "@/lib/types";
import { serverAction } from "@/lib/api";

export function ServerActions({ server }: { server: Server }) {
  const runAction = async (action: "start" | "stop" | "restart" | "delete") => {
    if ((action === "stop" || action === "restart" || action === "delete") && !window.confirm(`Confirm ${action} for ${server.name}?`)) {
      return;
    }
    try {
      await serverAction(server.id, action);
    } catch (error) {
      window.alert(error instanceof Error ? error.message : `Unable to ${action} server`);
    }
  };
  return (
    <div className="flex flex-wrap gap-2">
      {server.status === "running" ? (
        <Button variant="danger" onClick={() => void runAction("stop")}>
          <Square aria-hidden="true" />
          Stop
        </Button>
      ) : (
        <Button onClick={() => void runAction("start")}>
          <Play aria-hidden="true" />
          Start
        </Button>
      )}
      <Button variant="secondary" onClick={() => void runAction("restart")}>
        <RotateCcw aria-hidden="true" />
        Restart
      </Button>
      <Button variant="primary" onClick={() => void navigator.clipboard.writeText(`Join ${server.name} at 127.0.0.1:${server.port}`)}>
        <Copy aria-hidden="true" />
        Copy Invite
      </Button>
    </div>
  );
}
