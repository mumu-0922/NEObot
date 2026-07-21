import { SEARCH_CONFIG_LIMITS } from "../../config/limits";
import type { SearchProviderID, SearchServiceConfig } from "../../types";

const DEFAULT_SEARCH_RESULTS_LIMIT = 5;

export type SearchCompatibilityMode = "server" | "unavailable";

export type SearchCompatibilityReason = "server_search_unavailable";

export interface SearchCompatibilityResult {
  enabled: boolean;
  mode: SearchCompatibilityMode;
  provider: SearchProviderID;
  reason?: SearchCompatibilityReason;
}

const clampInteger = (
  value: unknown,
  min: number,
  max: number,
  fallback: number,
) => {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(max, Math.max(min, Math.round(number)));
};

export const isSearchProviderID = (
  provider: unknown,
): provider is SearchProviderID => provider === "default";

export const normalizeSearchProvider = (
  provider: unknown,
): SearchProviderID => {
  void provider;
  return "default";
};

export const getSearchProviderLabel = (provider: SearchProviderID): string => {
  void provider;
  return "Server";
};

export const getSearchCompatibility = ({
  searchProvider,
  searchConfig,
}: {
  searchProvider: SearchProviderID;
  searchConfig?: SearchServiceConfig;
}): SearchCompatibilityResult => {
  return searchConfig?.serverAvailable
    ? { enabled: true, mode: "server", provider: searchProvider }
    : {
        enabled: false,
        mode: "unavailable",
        provider: searchProvider,
        reason: "server_search_unavailable",
      };
};

export const getSearchCompatibilityErrorMessage = (
  result: SearchCompatibilityResult,
): string => {
  void result;
  return "Search is not configured on the server.";
};

export const normalizeSearchResultsLimit = (limit: unknown): number =>
  clampInteger(
    limit,
    SEARCH_CONFIG_LIMITS.minResultsLimit,
    SEARCH_CONFIG_LIMITS.maxResultsLimit,
    DEFAULT_SEARCH_RESULTS_LIMIT,
  );

export const normalizeSearchConfig = (
  provider: unknown,
  config: unknown,
): SearchServiceConfig | undefined => {
  if (provider !== "default") {
    return undefined;
  }
  const rawConfig =
    config && typeof config === "object"
      ? (config as Partial<SearchServiceConfig>)
      : {};
  return { serverAvailable: rawConfig.serverAvailable === true };
};

export const normalizeSearchSettings = (
  search: unknown,
): {
  provider: SearchProviderID;
  resultsLimit: number;
  configs: Record<string, SearchServiceConfig>;
} => {
  const rawSearch =
    search && typeof search === "object"
      ? (search as {
          provider?: unknown;
          resultsLimit?: unknown;
          configs?: Record<string, unknown>;
        })
      : {};
  const rawDefaultConfig =
    rawSearch.configs?.default && typeof rawSearch.configs.default === "object"
      ? (rawSearch.configs.default as Partial<SearchServiceConfig>)
      : {};

  return {
    provider: "default",
    resultsLimit: normalizeSearchResultsLimit(rawSearch.resultsLimit),
    configs: {
      default: {
        serverAvailable: rawDefaultConfig.serverAvailable === true,
      },
    },
  };
};
