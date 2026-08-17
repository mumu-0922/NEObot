import { describe, expect, it } from "vitest";
import {
  normalizeChatToolMode,
  resolveEffectiveChatToolMode,
  resolveModelToolCapability,
} from "../lib/chat/agentMode";
import type { ModelProvider } from "../types";

const provider = (overrides: Partial<ModelProvider> = {}): ModelProvider => ({
  id: "provider",
  name: "Provider",
  type: "OpenAI Compatible",
  baseUrl: "https://example.invalid/v1",
  apiKey: "",
  enabled: true,
  models: ["model-a"],
  modelsList: ["model-a"],
  toolCapabilityDefault: "auto",
  toolCapabilityModelOverrides: {},
  ...overrides,
});

describe("Chat/Agent mode policy", () => {
  it("defaults missing and invalid persisted values to Agent", () => {
    expect(normalizeChatToolMode(undefined)).toBe("agent");
    expect(normalizeChatToolMode("invalid")).toBe("agent");
    expect(normalizeChatToolMode("chat")).toBe("chat");
  });

  it("resolves Tool capability by model, provider, metadata, then unknown", () => {
    const base = {
      selectedModel: "provider:model-a",
      modelMetadata: {
        "model-a": { id: "model-a", name: "A", tool_call: false },
      },
      customModelMetadata: {},
    };

    expect(
      resolveModelToolCapability({
        ...base,
        providers: [
          provider({
            toolCapabilityDefault: "disabled",
            toolCapabilityModelOverrides: { "model-a": "enabled" },
          }),
        ],
      }),
    ).toBe("supported");
    expect(
      resolveModelToolCapability({
        ...base,
        providers: [provider({ toolCapabilityDefault: "enabled" })],
      }),
    ).toBe("supported");
    expect(
      resolveModelToolCapability({ ...base, providers: [provider()] }),
    ).toBe("unsupported");
    expect(
      resolveModelToolCapability({
        ...base,
        providers: [provider()],
        modelMetadata: {},
      }),
    ).toBe("unknown");
  });

  it("keeps the requested Agent choice while only effective mode downgrades", () => {
    expect(resolveEffectiveChatToolMode("agent", "unsupported")).toBe("chat");
    expect(resolveEffectiveChatToolMode("agent", "unknown")).toBe("agent");
    expect(resolveEffectiveChatToolMode("chat", "supported")).toBe("chat");
  });
});
