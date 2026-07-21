import { describe, expect, it } from "vitest";
import {
  getSearchCompatibility,
  getSearchCompatibilityErrorMessage,
  getSearchProviderLabel,
} from "../lib/settings/search";

describe("search compatibility", () => {
  it("uses only server-published search availability", () => {
    expect(
      getSearchCompatibility({
        searchProvider: "default",
        searchConfig: { serverAvailable: true },
      }),
    ).toEqual({ enabled: true, mode: "server", provider: "default" });

    const unavailable = getSearchCompatibility({
      searchProvider: "default",
      searchConfig: { serverAvailable: false },
    });
    expect(unavailable).toEqual({
      enabled: false,
      mode: "unavailable",
      provider: "default",
      reason: "server_search_unavailable",
    });
    expect(getSearchCompatibilityErrorMessage(unavailable)).toContain("server");
    expect(getSearchProviderLabel("default")).toBe("Server");
  });
});
