import { describe, expect, it } from "vitest";
import { normalizeSearchMode, searchModeEnabled } from "../lib/chat/searchMode";

describe("chat search mode", () => {
  it("normalizes the three persisted modes", () => {
    expect(normalizeSearchMode("off", true)).toBe("off");
    expect(normalizeSearchMode("model_builtin", false)).toBe("model_builtin");
    expect(normalizeSearchMode("external", false)).toBe("external");
  });

  it("maps legacy useSearch to external without enabling invalid values", () => {
    expect(normalizeSearchMode(undefined, true)).toBe("external");
    expect(normalizeSearchMode("invalid", true)).toBe("off");
    expect(normalizeSearchMode("invalid", false)).toBe("off");
    expect(searchModeEnabled("off")).toBe(false);
    expect(searchModeEnabled("external")).toBe(true);
    expect(searchModeEnabled("model_builtin")).toBe(true);
  });
});
