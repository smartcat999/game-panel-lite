import type { GameUpdateJob, GameUpdateState } from "./types";

export function isGameUpdateJobActive(job?: GameUpdateJob) {
  return job?.status === "queued" || job?.status === "running";
}

export function isGameUpdateStateActive(state?: GameUpdateState) {
  return state?.status === "checking" || state?.status === "updating" || isGameUpdateJobActive(state?.job);
}

export function normalizeGameUpdateProgress(progress?: number) {
  if (!Number.isFinite(progress)) return 0;
  return Math.min(100, Math.max(0, Math.round(progress ?? 0)));
}
