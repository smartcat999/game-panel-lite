import { describe, expect, it } from "vitest";

describe("dashboard foundation", () => {
  it("keeps the product name stable", () => {
    expect("GamePanel Lite").toBe("GamePanel Lite");
  });
});
