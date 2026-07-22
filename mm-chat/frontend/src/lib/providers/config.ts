import type {
  ModelBuiltInSearchConfig,
  ModelBuiltInSearchProtocol,
  ModelProvider,
  ProviderType,
} from "../../types";
import {
  PROVIDER_CONFIG_LIMITS,
  PROVIDER_MODEL_LIMITS,
} from "../../config/limits";
import { normalizeProviderModelId } from "./models";
import { isLocalEncryptedSecretEnvelope } from "../security/localSecrets";
import {
  OPENAI_COMPATIBLE_PROVIDER_TYPE,
  OPENAI_PROVIDER_TYPE,
  isProviderType,
} from "./providerTypes";
import { SERVER_DEFAULT_PROVIDER_ID } from "../defaultConfig/shared";

type ServerManagedProviderConfig = {
  id: string;
  name: string;
  type: string;
  baseUrl: string;
  models: string[];
  enabled: boolean;
  modelBuiltInSearch?: {
    protocol?: string;
    model?: string;
    source: "official" | "custom" | "none";
    connectionTestValid: boolean;
    connectionTestedAt?: string;
  };
};

function isModelBuiltInSearchProtocol(
  value: unknown,
): value is ModelBuiltInSearchProtocol {
  return (
    value === "openai_responses" ||
    value === "gemini_google_search" ||
    value === "anthropic_web_search"
  );
}

function normalizeModelBuiltInSearchConfig(
  value: unknown,
): ModelBuiltInSearchConfig | undefined {
  if (!value || typeof value !== "object") return undefined;
  const raw = value as Partial<ModelBuiltInSearchConfig>;
  const source =
    raw.source === "official" || raw.source === "custom" ? raw.source : "none";
  return {
    ...(isModelBuiltInSearchProtocol(raw.protocol)
      ? { protocol: raw.protocol }
      : {}),
    ...(typeof raw.model === "string" && raw.model.trim()
      ? { model: raw.model.trim().slice(0, 256) }
      : {}),
    source,
    connectionTestValid: raw.connectionTestValid === true,
    ...(typeof raw.connectionTestedAt === "string" &&
    raw.connectionTestedAt.trim()
      ? { connectionTestedAt: raw.connectionTestedAt.trim() }
      : {}),
  };
}

function trimString(value: unknown, maxChars: number): string {
  return typeof value === "string" ? value.trim().slice(0, maxChars) : "";
}

function normalizeProviderType(value: unknown): ProviderType {
  return isProviderType(value) ? value : OPENAI_PROVIDER_TYPE;
}

export function migrateCoreSettingsState<T extends { providers?: unknown }>(
  state: T,
): T & { providers?: ModelProvider[] } {
  const rawProviders = Array.isArray(state.providers) ? state.providers : [];
  const providers = normalizeModelProviders(
    rawProviders.map((provider) => {
      if (!provider || typeof provider !== "object") return provider;
      const raw = provider as Partial<ModelProvider>;
      return {
        ...raw,
        type:
          raw.type === OPENAI_PROVIDER_TYPE
            ? OPENAI_COMPATIBLE_PROVIDER_TYPE
            : raw.type,
      };
    }),
  );

  return {
    ...state,
    providers,
  };
}

function normalizeModelList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];

  const result: string[] = [];
  const seen = new Set<string>();

  for (const item of value) {
    const modelId = normalizeProviderModelId(item);
    if (!modelId || seen.has(modelId)) continue;

    result.push(modelId);
    seen.add(modelId);
    if (result.length >= PROVIDER_MODEL_LIMITS.maxModels) break;
  }

  return result;
}

export function normalizeModelProvider(
  value: unknown,
  fallback?: Partial<ModelProvider>,
): ModelProvider | null {
  if (!value || typeof value !== "object") return null;
  const raw = value as Partial<ModelProvider>;
  const fallbackId = fallback?.id || "";

  const id =
    trimString(raw.id, PROVIDER_CONFIG_LIMITS.maxProviderIdChars) ||
    trimString(fallbackId, PROVIDER_CONFIG_LIMITS.maxProviderIdChars);
  if (!id) return null;

  const type = normalizeProviderType(raw.type || fallback?.type);
  const models = normalizeModelList(raw.models);
  const modelsList = normalizeModelList(raw.modelsList || raw.models);
  const modelBuiltInSearch = normalizeModelBuiltInSearchConfig(
    raw.modelBuiltInSearch,
  );

  return {
    id,
    name:
      trimString(raw.name, PROVIDER_CONFIG_LIMITS.maxProviderNameChars) ||
      fallback?.name ||
      "Provider",
    type,
    baseUrl: trimString(raw.baseUrl, PROVIDER_CONFIG_LIMITS.maxBaseUrlChars),
    apiKey: trimString(raw.apiKey, PROVIDER_CONFIG_LIMITS.maxApiKeyChars),
    ...(isLocalEncryptedSecretEnvelope(raw.apiKeySecret)
      ? { apiKeySecret: raw.apiKeySecret }
      : {}),
    enabled: typeof raw.enabled === "boolean" ? raw.enabled : true,
    models: models.filter(
      (model) => modelsList.length === 0 || modelsList.includes(model),
    ),
    modelsList,
    ...(raw.isServerDefault ? { isServerDefault: true } : {}),
    ...(raw.isServerManaged ? { isServerManaged: true } : {}),
    ...(modelBuiltInSearch ? { modelBuiltInSearch } : {}),
  };
}

export function normalizeModelProviders(value: unknown): ModelProvider[] {
  if (!Array.isArray(value)) return [];

  const providers: ModelProvider[] = [];
  const seen = new Set<string>();

  for (const item of value) {
    const provider = normalizeModelProvider(item);
    if (!provider || seen.has(provider.id)) continue;

    providers.push(provider);
    seen.add(provider.id);
    if (providers.length >= PROVIDER_CONFIG_LIMITS.maxProviders) break;
  }

  return providers;
}

export function normalizeServerManagedProviderConfigs(
  providers: readonly ServerManagedProviderConfig[],
): ModelProvider[] {
  return normalizeModelProviders(
    providers.map((provider) => ({
      id: provider.id,
      name: provider.name,
      type: provider.type,
      baseUrl: provider.baseUrl,
      apiKey: "",
      enabled: provider.enabled,
      models: provider.models,
      modelsList: provider.models,
      isServerDefault: provider.id === SERVER_DEFAULT_PROVIDER_ID,
      isServerManaged: true,
      modelBuiltInSearch: provider.modelBuiltInSearch,
    })),
  );
}
