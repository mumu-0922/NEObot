import type {
  AgentPermissionMode,
  ChatToolMode,
  ModelMetadata,
  ModelProvider,
} from "../../types";
import { parseModelString } from "../utils/model";

export type ModelToolCapability = "supported" | "unsupported" | "unknown";

export interface ResolveModelToolCapabilityOptions {
  selectedModel: string;
  providers: readonly ModelProvider[];
  modelMetadata: Record<string, ModelMetadata>;
  customModelMetadata: Record<string, ModelMetadata>;
}

export function isChatToolMode(value: unknown): value is ChatToolMode {
  return value === "chat" || value === "agent";
}

export function isAgentPermissionMode(
  value: unknown,
): value is AgentPermissionMode {
  return (
    value === "read-only" ||
    value === "workspace-write" ||
    value === "danger-full-access"
  );
}

export function normalizeChatToolMode(
  value: unknown,
  fallback: ChatToolMode = "agent",
): ChatToolMode {
  return isChatToolMode(value) ? value : fallback;
}

export function resolveModelToolCapability({
  selectedModel,
  providers,
  modelMetadata,
  customModelMetadata,
}: ResolveModelToolCapabilityOptions): ModelToolCapability {
  const { providerId, modelName } = parseModelString(selectedModel);
  const provider = providerId
    ? providers.find((candidate) => candidate.id === providerId)
    : providers.find((candidate) => candidate.enabled);
  const modelOverride = provider?.toolCapabilityModelOverrides[modelName];

  if (modelOverride === "enabled") return "supported";
  if (modelOverride === "disabled") return "unsupported";
  if (provider?.toolCapabilityDefault === "enabled") return "supported";
  if (provider?.toolCapabilityDefault === "disabled") return "unsupported";

  const metadata = customModelMetadata[modelName] || modelMetadata[modelName];
  if (metadata?.tool_call === true) return "supported";
  if (metadata?.tool_call === false) return "unsupported";
  return "unknown";
}

export function resolveEffectiveChatToolMode(
  requestedMode: ChatToolMode,
  capability: ModelToolCapability,
): ChatToolMode {
  return requestedMode === "agent" && capability === "unsupported"
    ? "chat"
    : requestedMode;
}
