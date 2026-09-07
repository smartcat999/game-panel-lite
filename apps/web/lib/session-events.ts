export const sessionExpiredEvent = "gamepanel:session-expired";

export function notifySessionExpired() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(sessionExpiredEvent));
}
