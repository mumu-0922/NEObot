import { describe, expect, it } from "vitest";
import { PROVIDER_MODEL_LIMITS } from "../config/limits";
import {
  extractProviderModelIds,
  mergeDiscoveredProviderModels,
  providerModelIdsEqual,
} from "../lib/providers/models";

describe("provider model extraction", () => {
  it("retains selected hidden models and enables verified discoveries across refreshes", () => {
    const first = mergeDiscoveredProviderModels(
      ["gpt-5.6-sol", "custom-hidden"],
      ["gpt-5.6-sol", "gpt-6-astra"],
      ["gpt-6-astra", "unverified"],
    );
    expect(first.models).toEqual([
      "gpt-5.6-sol",
      "custom-hidden",
      "gpt-6-astra",
    ]);
    expect(first.modelsList).toEqual([
      "gpt-5.6-sol",
      "gpt-6-astra",
      "custom-hidden",
    ]);
    expect(
      mergeDiscoveredProviderModels(first.models, ["gpt-5.6-sol"]).models,
    ).toEqual(first.models);
  });

  it("keeps unselected listed models unselected and deduplicates first-time discovery", () => {
    expect(
      mergeDiscoveredProviderModels(["selected"], ["selected", "unselected"])
        .models,
    ).toEqual(["selected"]);
    expect(mergeDiscoveredProviderModels([], ["new", "new"], ["new"])).toEqual({
      models: ["new"],
      modelsList: ["new"],
    });
  });
  it("compares ordered model selections by value rather than length alone", () => {
    expect(providerModelIdsEqual(["gpt-a"], ["gpt-a"])).toBe(true);
    expect(providerModelIdsEqual(["gpt-a"], ["gpt-b"])).toBe(false);
    expect(providerModelIdsEqual(["gpt-a"], ["gpt-a", "gpt-b"])).toBe(false);
  });

  it("filters Gemini models to generateContent-capable ids", () => {
    expect(
      extractProviderModelIds("Gemini", {
        models: [
          {
            name: "models/gemini-2.5-pro",
            supportedGenerationMethods: ["generateContent"],
          },
          {
            name: "models/embed-only",
            supportedGenerationMethods: ["embedContent"],
          },
          {
            name: "models/gemini-2.5-pro",
            supportedGenerationMethods: ["generateContent"],
          },
        ],
      }),
    ).toEqual(["gemini-2.5-pro"]);
  });

  it("deduplicates and trims OpenAI-compatible model ids", () => {
    expect(
      extractProviderModelIds("OpenAI", {
        data: [
          { id: " gpt-5 " },
          { id: "gpt-5" },
          { id: "" },
          { id: 42 },
          { id: "o4-mini" },
        ],
      }),
    ).toEqual(["gpt-5", "o4-mini"]);
  });

  it("treats OpenAI Compatible model lists like OpenAI model lists", () => {
    expect(
      extractProviderModelIds("OpenAI Compatible", {
        data: [{ id: "chat-model" }, { id: "chat-model" }],
      }),
    ).toEqual(["chat-model"]);
  });

  it("caps model id length and total model count", () => {
    const models = Array.from(
      { length: PROVIDER_MODEL_LIMITS.maxModels + 5 },
      (_, index) => ({
        id:
          index === 0
            ? "m".repeat(PROVIDER_MODEL_LIMITS.maxModelIdChars + 10)
            : `model-${index}`,
      }),
    );

    const result = extractProviderModelIds("OpenAI", { data: models });

    expect(result).toHaveLength(PROVIDER_MODEL_LIMITS.maxModels);
    expect(result[0]).toHaveLength(PROVIDER_MODEL_LIMITS.maxModelIdChars);
  });
});
