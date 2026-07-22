import type { ModelBuiltInSearchProtocol, ModelProvider } from "../../types";

export type ModelBuiltInSearchUnavailableReason =
  "provider_unavailable" | "model_unsupported" | "admin_test_required";

export interface ModelBuiltInSearchAvailability {
  enabled: boolean;
  protocol?: ModelBuiltInSearchProtocol;
  reason?: ModelBuiltInSearchUnavailableReason;
}

function officialModelSupportsBuiltInSearch(
  provider: ModelProvider,
  modelId: string,
): boolean {
  const model = modelId.trim().toLowerCase();
  if (!model) return false;
  switch (provider.type) {
    case "OpenAI":
      if (/image|audio|realtime|embedding|transcri|tts/u.test(model)) {
        return false;
      }
      return /^(gpt-|o1|o3|o4|chatgpt-)/u.test(model);
    case "Gemini":
      return (
        model.startsWith("gemini-") &&
        !/image|embedding|tts|native-audio/u.test(model)
      );
    case "Anthropic":
      return model.startsWith("claude-");
    default:
      return false;
  }
}

function officialProtocol(
  provider: ModelProvider,
): ModelBuiltInSearchProtocol | undefined {
  switch (provider.type) {
    case "OpenAI":
      return "openai_responses";
    case "Gemini":
      return "gemini_google_search";
    case "Anthropic":
      return "anthropic_web_search";
    default:
      return undefined;
  }
}

export function getModelBuiltInSearchAvailability({
  provider,
  modelId,
}: {
  provider?: ModelProvider;
  modelId: string;
}): ModelBuiltInSearchAvailability {
  if (!provider?.enabled || !modelId.trim()) {
    return { enabled: false, reason: "provider_unavailable" };
  }
  const protocol = officialProtocol(provider);
  if (protocol) {
    return officialModelSupportsBuiltInSearch(provider, modelId)
      ? { enabled: true, protocol }
      : { enabled: false, protocol, reason: "model_unsupported" };
  }
  const configured = provider.modelBuiltInSearch;
  if (
    configured?.source === "custom" &&
    configured.protocol === "openai_responses" &&
    configured.connectionTestValid &&
    configured.model === modelId
  ) {
    return { enabled: true, protocol: configured.protocol };
  }
  return {
    enabled: false,
    protocol: configured?.protocol,
    reason: configured?.protocol ? "admin_test_required" : "model_unsupported",
  };
}
