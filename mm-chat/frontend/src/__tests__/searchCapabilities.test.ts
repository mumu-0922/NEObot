import { describe, expect, it } from "vitest";
import { getModelBuiltInSearchAvailability } from "../lib/chat/searchCapabilities";
import type { ModelProvider } from "../types";

function provider(
  type: ModelProvider["type"],
  overrides: Partial<ModelProvider> = {},
): ModelProvider {
  return {
    id: "PROVIDER",
    name: "Provider",
    type,
    baseUrl: "https://provider.example/v1",
    apiKey: "",
    enabled: true,
    models: [],
    ...overrides,
  };
}

describe("model built-in Search capability", () => {
  it.each([
    ["OpenAI", "gpt-5", "openai_responses"],
    ["Gemini", "gemini-2.5-pro", "gemini_google_search"],
    ["Anthropic", "claude-sonnet-4-5", "anthropic_web_search"],
  ] as const)(
    "enables official %s chat models with the native protocol",
    (type, modelId, protocol) => {
      expect(
        getModelBuiltInSearchAvailability({
          provider: provider(type),
          modelId,
        }),
      ).toEqual({ enabled: true, protocol });
    },
  );

  it.each([
    ["OpenAI", "gpt-image-2"],
    ["OpenAI", "text-embedding-3-large"],
    ["Gemini", "gemini-2.5-flash-image"],
    ["Gemini", "gemini-embedding-001"],
  ] as const)("disables non-chat %s models", (type, modelId) => {
    expect(
      getModelBuiltInSearchAvailability({
        provider: provider(type),
        modelId,
      }),
    ).toMatchObject({ enabled: false, reason: "model_unsupported" });
  });

  it("requires an exact successful administrator test for compatible models", () => {
    const custom = provider("OpenAI Compatible", {
      modelBuiltInSearch: {
        protocol: "openai_responses",
        model: "relay-chat",
        source: "custom",
        connectionTestValid: true,
      },
    });

    expect(
      getModelBuiltInSearchAvailability({
        provider: custom,
        modelId: "relay-chat",
      }),
    ).toEqual({ enabled: true, protocol: "openai_responses" });
    expect(
      getModelBuiltInSearchAvailability({
        provider: custom,
        modelId: "another-model",
      }),
    ).toMatchObject({ enabled: false, reason: "admin_test_required" });
    expect(
      getModelBuiltInSearchAvailability({
        provider: {
          ...custom,
          modelBuiltInSearch: {
            ...custom.modelBuiltInSearch!,
            connectionTestValid: false,
          },
        },
        modelId: "relay-chat",
      }),
    ).toMatchObject({ enabled: false, reason: "admin_test_required" });
  });

  it("disables built-in Search when the selected provider is unavailable", () => {
    expect(
      getModelBuiltInSearchAvailability({
        provider: provider("OpenAI", { enabled: false }),
        modelId: "gpt-5",
      }),
    ).toEqual({ enabled: false, reason: "provider_unavailable" });
  });
});
