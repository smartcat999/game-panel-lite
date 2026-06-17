import { describe, expect, it } from "vitest";
import { createReviewInvitePreview } from "./create-server-review";

describe("createReviewInvitePreview", () => {
  it("creates Terraria invite preview text", () => {
    expect(createReviewInvitePreview({ gameKey: "terraria", hostPortLabel: "7777", password: "secret", serverName: "Friends" })).toBe(
      "Join Friends in Terraria at 127.0.0.1:7777 password: secret"
    );
  });

  it("creates Palworld invite preview text", () => {
    expect(createReviewInvitePreview({ gameKey: "palworld", hostPortLabel: "18211", serverName: "Pal Friends" })).toBe(
      "Join Pal Friends in Palworld at 127.0.0.1:18211"
    );
  });

  it("creates Don't Starve Together invite preview text", () => {
    expect(createReviewInvitePreview({ gameKey: "dont-starve-together", hostPortLabel: "11099", password: "secret", serverName: "DST Friends" })).toBe(
      "Join DST Friends in Don't Starve Together at 127.0.0.1:11099 password: secret"
    );
  });
});
