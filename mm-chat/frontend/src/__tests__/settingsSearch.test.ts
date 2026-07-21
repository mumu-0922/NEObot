import { describe, expect, it } from "vitest";
import { SEARCH_CONFIG_LIMITS } from "../config/limits";
import {
  normalizeSearchConfig,
  normalizeSearchSettings,
} from "../lib/settings/search";

describe("search settings normalization", () => {
  it("falls back invalid search providers and clamps result limits", () => {
    const search = normalizeSearchSettings({
      provider: "unknown",
      resultsLimit: 999,
      configs: {
        default: { serverAvailable: true },
        injected: { serverAvailable: true },
      },
    });

    expect(search.provider).toBe("default");
    expect(search.resultsLimit).toBe(SEARCH_CONFIG_LIMITS.maxResultsLimit);
    expect(search.configs.default.serverAvailable).toBe(true);
    expect(search.configs).not.toHaveProperty("injected");
  });

  it("falls back missing search settings to the server service", () => {
    const search = normalizeSearchSettings(undefined);

    expect(search.provider).toBe("default");
    expect(search.configs.default.serverAvailable).toBe(false);
  });

  it("accepts only server-owned search availability", () => {
    expect(normalizeSearchConfig("default", { serverAvailable: true })).toEqual(
      { serverAvailable: true },
    );
    expect(normalizeSearchConfig("tavily", {})).toBeUndefined();
  });
});
