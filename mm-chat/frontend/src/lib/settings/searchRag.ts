import { RAG_LIMITS, SEARCH_CONFIG_LIMITS } from "../../config/limits";
import type {
  DocumentParseProvider,
  RAGConfig,
  SearchProviderID,
  SearchServiceConfig,
} from "../../types";
import { isLocalEncryptedSecretEnvelope } from "../security/localSecrets";

const DEFAULT_SEARCH_RESULTS_LIMIT = 5;
const DEFAULT_RAG_TOP_K = 10;
const DEFAULT_RAG_CHUNK_SIZE = 512;
const DEFAULT_DOCUMENT_PARSE_PROVIDER: DocumentParseProvider = "mineru";

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

const trimToLimit = (value: unknown, maxLength: number): string => {
  if (typeof value !== "string") return "";
  return value.trim().slice(0, maxLength);
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

export const isDocumentParseProvider = (
  provider: unknown,
): provider is DocumentParseProvider =>
  provider === "mineru" || provider === "llamaParse";

export const normalizeDocumentParseProvider = (
  provider: unknown,
): DocumentParseProvider =>
  isDocumentParseProvider(provider)
    ? provider
    : DEFAULT_DOCUMENT_PARSE_PROVIDER;

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

export const normalizeRAGConfig = (config: unknown): RAGConfig => {
  const rawConfig =
    config && typeof config === "object" ? (config as Partial<RAGConfig>) : {};

  const namespace = trimToLimit(
    rawConfig.namespace,
    RAG_LIMITS.maxNamespaceChars,
  );

  return {
    enabled: rawConfig.enabled === true,
    url: trimToLimit(rawConfig.url, RAG_LIMITS.maxBaseUrlChars),
    token: trimToLimit(rawConfig.token, RAG_LIMITS.maxTokenChars),
    ...(isLocalEncryptedSecretEnvelope(rawConfig.tokenSecret)
      ? { tokenSecret: rawConfig.tokenSecret }
      : {}),
    topK: clampInteger(
      rawConfig.topK,
      RAG_LIMITS.minTopK,
      RAG_LIMITS.maxTopK,
      DEFAULT_RAG_TOP_K,
    ),
    chunkSize: clampInteger(
      rawConfig.chunkSize,
      RAG_LIMITS.minChunkSize,
      RAG_LIMITS.maxChunkSize,
      DEFAULT_RAG_CHUNK_SIZE,
    ),
    documentParseProvider: normalizeDocumentParseProvider(
      rawConfig.documentParseProvider,
    ),
    mineruApiToken: trimToLimit(
      rawConfig.mineruApiToken,
      RAG_LIMITS.maxMineruApiTokenChars,
    ),
    ...(isLocalEncryptedSecretEnvelope(rawConfig.mineruApiTokenSecret)
      ? { mineruApiTokenSecret: rawConfig.mineruApiTokenSecret }
      : {}),
    llamaParseApiKey: trimToLimit(
      rawConfig.llamaParseApiKey,
      RAG_LIMITS.maxLlamaParseApiKeyChars,
    ),
    ...(isLocalEncryptedSecretEnvelope(rawConfig.llamaParseApiKeySecret)
      ? { llamaParseApiKeySecret: rawConfig.llamaParseApiKeySecret }
      : {}),
    ...(namespace ? { namespace } : {}),
    ...(rawConfig.useDefaultVectorStore !== undefined
      ? { useDefaultVectorStore: rawConfig.useDefaultVectorStore === true }
      : {}),
    ...(rawConfig.useDefaultDocumentProcessing !== undefined
      ? {
          useDefaultDocumentProcessing:
            rawConfig.useDefaultDocumentProcessing === true,
        }
      : {}),
    ...(rawConfig.serverVectorStoreAvailable !== undefined
      ? {
          serverVectorStoreAvailable:
            rawConfig.serverVectorStoreAvailable === true,
        }
      : {}),
    ...(rawConfig.serverDocumentProcessingAvailable !== undefined
      ? {
          serverDocumentProcessingAvailable:
            rawConfig.serverDocumentProcessingAvailable === true,
        }
      : {}),
  };
};
