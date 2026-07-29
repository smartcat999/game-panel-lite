import { describe, expect, it } from "vitest";
import { parseWorkshopIds } from "./workshop-input";

describe("parseWorkshopIds", () => {
  it("accepts IDs, full Steam URLs, and mixed separators", () => {
    expect(parseWorkshopIds([
      "2824688072",
      "https://steamcommunity.com/sharedfiles/filedetails/?id=2824688266",
      "2619954303，2669644269"
    ].join("\n"))).toEqual(["2824688072", "2824688266", "2619954303", "2669644269"]);
  });

  it("deduplicates IDs and ignores unsupported URLs or invalid input", () => {
    expect(parseWorkshopIds([
      "2824688072",
      "https://steamcommunity.com/sharedfiles/filedetails/?id=2824688072",
      "https://steamcommunity.com/workshop/browse/?appid=1281930",
      "https://example.com/sharedfiles/filedetails/?id=123456",
      "not-an-id"
    ].join("\n"))).toEqual(["2824688072"]);
  });
});
