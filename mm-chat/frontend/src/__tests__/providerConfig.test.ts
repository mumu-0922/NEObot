import { describe, expect, it } from "vitest";
import {
  PROVIDER_CONFIG_LIMITS,
  PROVIDER_MODEL_LIMITS,
} from "../config/limits";
import {
  migrateCoreSettingsState,
  normalizeModelProvider,
  normalizeModelProviders,
  normalizeServerManagedProviderConfigs,
} from "../lib/providers/config";

describe("provider config normalization", () => {
  it("trims provider fields and normalizes models", () => {
    const provider = normalizeModelProvider({
      id: " PROVIDER ",
      name: "x".repeat(PROVIDER_CONFIG_LIMITS.maxProviderNameChars + 10),
      type: "Other",
      baseUrl: ` https://example.com/${"b".repeat(
        PROVIDER_CONFIG_LIMITS.maxBaseUrlChars,
      )}`,
      apiKey: "k".repeat(PROVIDER_CONFIG_LIMITS.maxApiKeyChars + 10),
      enabled: "yes",
      models: [" models/gemini-pro ", "gemini-pro", "", 42, "custom-model"],
      modelsList: ["gemini-pro"],
    });

    expect(provider).toMatchObject({
      id: "PROVIDER",
      type: "OpenAI",
      enabled: true,
      models: ["gemini-pro"],
      modelsList: ["gemini-pro"],
    });
    expect(provider?.name).toHaveLength(
      PROVIDER_CONFIG_LIMITS.maxProviderNameChars,
    );
    expect(provider?.baseUrl).toHaveLength(
      PROVIDER_CONFIG_LIMITS.maxBaseUrlChars,
    );
    expect(provider?.apiKey).toHaveLength(
      PROVIDER_CONFIG_LIMITS.maxApiKeyChars,
    );
  });

  it("keeps selected models when no fetched model list exists", () => {
    const provider = normalizeModelProvider({
      id: "A",
      type: "Gemini",
      models: ["model-a"],
      modelsList: [],
    });

    expect(provider?.models).toEqual(["model-a"]);
    expect(provider?.modelsList).toEqual([]);
  });

  it("accepts OpenAI Compatible as a provider type", () => {
    expect(
      normalizeModelProvider({
        id: "COMPAT",
        type: "OpenAI Compatible",
      })?.type,
    ).toBe("OpenAI Compatible");
  });

  it("accepts Anthropic as a provider protocol without a vendor preset", () => {
    expect(
      normalizeModelProvider({
        id: "CLAUDE",
        type: "Anthropic",
        baseUrl: "https://api.anthropic.com",
      })?.type,
    ).toBe("Anthropic");
  });

  it("preserves backend-managed provider identity through normalization", () => {
    const [provider] = normalizeServerManagedProviderConfigs([
      {
        id: "CUSTOM",
        name: "Backend Custom",
        type: "OpenAI Compatible",
        baseUrl: "https://custom.example/v1",
        models: ["custom-model"],
        enabled: true,
      },
    ]);

    expect(provider).toMatchObject({
      id: "CUSTOM",
      isServerManaged: true,
    });
    expect(provider?.isServerDefault).toBeUndefined();
    expect(normalizeModelProvider(provider)?.isServerManaged).toBe(true);
  });

  it("preserves exact built-in Search attestation on backend reload", () => {
    const [provider] = normalizeServerManagedProviderConfigs([
      {
        id: "CUSTOM",
        name: "Backend Custom",
        type: "OpenAI Compatible",
        baseUrl: "https://custom.example/v1",
        models: ["custom-model"],
        enabled: true,
        modelBuiltInSearch: {
          protocol: "openai_responses",
          model: "custom-model",
          source: "custom",
          connectionTestValid: true,
          connectionTestedAt: "2026-07-22T10:00:00Z",
        },
      },
    ]);

    expect(provider?.modelBuiltInSearch).toEqual({
      protocol: "openai_responses",
      model: "custom-model",
      source: "custom",
      connectionTestValid: true,
      connectionTestedAt: "2026-07-22T10:00:00Z",
    });
  });

  it("round-trips backend Tool capability defaults and model overrides", () => {
    const [provider] = normalizeServerManagedProviderConfigs([
      {
        id: "CUSTOM",
        name: "Backend Custom",
        type: "OpenAI Compatible",
        baseUrl: "https://custom.example/v1",
        models: ["model-a", "model-b"],
        enabled: true,
        toolCapability: {
          default: "disabled",
          modelOverrides: {
            "model-a": "enabled",
            "model-b": "disabled",
          },
        },
      },
    ]);

    expect(provider).toMatchObject({
      toolCapabilityDefault: "disabled",
      toolCapabilityModelOverrides: {
        "model-a": "enabled",
        "model-b": "disabled",
      },
    });
    expect(normalizeModelProvider(provider)).toMatchObject({
      toolCapabilityDefault: "disabled",
      toolCapabilityModelOverrides: {
        "model-a": "enabled",
        "model-b": "disabled",
      },
    });
  });

  it("defaults unknown Tool capability values to Auto and drops invalid overrides", () => {
    const [provider] = normalizeServerManagedProviderConfigs([
      {
        id: "CUSTOM",
        name: "Backend Custom",
        type: "OpenAI Compatible",
        baseUrl: "https://custom.example/v1",
        models: ["selected-model"],
        enabled: true,
        toolCapability: {
          default: "future-value",
          modelOverrides: {
            "selected-model": "auto",
            "missing-model": "enabled",
            invalid: "future-value",
          },
        },
      },
    ]);

    expect(provider?.toolCapabilityDefault).toBe("auto");
    expect(provider?.toolCapabilityModelOverrides).toEqual({});
  });

  it("migrates persisted OpenAI providers to OpenAI Compatible", async () => {
    const migrated = await migrateCoreSettingsState({
      providers: [
        {
          id: "OLD",
          type: "OpenAI",
          models: ["gpt-4o-mini"],
          modelsList: ["gpt-4o-mini"],
        },
      ],
    });

    expect(migrated.providers?.[0]?.type).toBe("OpenAI Compatible");
    expect(migrated.providers?.[0]?.toolCapabilityDefault).toBe("auto");
    expect(migrated.providers?.[0]?.toolCapabilityModelOverrides).toEqual({});
  });

  it("filters invalid providers and caps provider/model counts", () => {
    const providers = Array.from(
      { length: PROVIDER_CONFIG_LIMITS.maxProviders + 5 },
      (_, index) => ({
        id: `P${index}`,
        type: "OpenAI",
        models: Array.from(
          { length: PROVIDER_MODEL_LIMITS.maxModels + 5 },
          (__, modelIndex) => `model-${modelIndex}`,
        ),
      }),
    );

    const normalized = normalizeModelProviders([
      null,
      { id: "" },
      ...providers,
      { id: "P1", type: "OpenAI" },
    ]);

    expect(normalized).toHaveLength(PROVIDER_CONFIG_LIMITS.maxProviders);
    expect(normalized[0]?.models).toHaveLength(PROVIDER_MODEL_LIMITS.maxModels);
    expect(normalized[1]?.id).toBe("P1");
  });
});
