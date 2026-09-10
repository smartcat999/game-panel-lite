import type { DeploymentState } from "@/lib/control-plane";
import type { MessageKey } from "@/lib/messages";
import { cn } from "@/lib/utils";

const labels: Record<DeploymentState | "stale", MessageKey> = {
  pending_payment: "instances.status.pending_payment",
  waiting_region: "instances.status.waiting_region",
  running: "instances.status.running",
  stopped: "instances.status.stopped",
  failed: "instances.status.failed",
  stale: "instances.status.stale",
};

export function InstanceStatus({ state, stale, t }: { state: DeploymentState; stale: boolean; t: (key: MessageKey) => string }) {
  const displayState = stale ? "stale" : state;
  return <span className={cn("status-badge", `status-${displayState}`)}>{t(labels[displayState])}</span>;
}
