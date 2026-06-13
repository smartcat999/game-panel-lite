"use client";

import { Copy, Play, RotateCcw, Square } from "lucide-react";
import { Button } from "./ui";
import type { Server } from "@/lib/types";

export function ServerActions({ server }: { server: Server }) {
  return (
    <div className="flex flex-wrap gap-2">
      {server.status === "running" ? (
        <Button variant="danger">
          <Square aria-hidden="true" />
          Stop
        </Button>
      ) : (
        <Button>
          <Play aria-hidden="true" />
          Start
        </Button>
      )}
      <Button variant="secondary">
        <RotateCcw aria-hidden="true" />
        Restart
      </Button>
      <Button variant="primary">
        <Copy aria-hidden="true" />
        Copy Invite
      </Button>
    </div>
  );
}
