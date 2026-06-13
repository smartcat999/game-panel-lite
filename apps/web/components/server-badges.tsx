import { Badge } from "@/components/ui";
import type { ServerMode, ServerStatus } from "@/lib/types";

export function ServerStatusBadge({ status }: { status: ServerStatus }) {
  const color = status === "running" ? "bg-panel-green/15 text-panel-green" : status === "errored" ? "bg-red-500/15 text-red-200" : "bg-slate-700 text-slate-300";
  return <Badge className={color}>{status === "running" ? "Running" : status === "errored" ? "Errored" : "Stopped"}</Badge>;
}

export function ServerModeBadge({ mode }: { mode: ServerMode }) {
  return mode === "tmodloader" ? (
    <Badge className="bg-panel-purple/20 text-panel-purple">tModLoader</Badge>
  ) : (
    <Badge className="bg-panel-green/15 text-panel-green">Vanilla</Badge>
  );
}
