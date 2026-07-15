import { describe, expect, it } from "vitest";
import { isGameUpdateJobActive, isGameUpdateStateActive, normalizeGameUpdateProgress } from "./game-update";

describe("game update display helpers", () => {
  it("keeps active jobs polling while queued or running", () => {
    expect(isGameUpdateJobActive({ status: "queued" } as never)).toBe(true);
    expect(isGameUpdateJobActive({ status: "running" } as never)).toBe(true);
    expect(isGameUpdateJobActive({ status: "succeeded" } as never)).toBe(false);
  });

  it("treats check and apply phases as active", () => {
    expect(isGameUpdateStateActive({ status: "checking" } as never)).toBe(true);
    expect(isGameUpdateStateActive({ status: "updating" } as never)).toBe(true);
    expect(isGameUpdateStateActive({ status: "up_to_date" } as never)).toBe(false);
  });

  it("clamps task progress for the progress bar", () => {
    expect(normalizeGameUpdateProgress(-20)).toBe(0);
    expect(normalizeGameUpdateProgress(47.6)).toBe(48);
    expect(normalizeGameUpdateProgress(120)).toBe(100);
    expect(normalizeGameUpdateProgress(Number.NaN)).toBe(0);
  });
});
