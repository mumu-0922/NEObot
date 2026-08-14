import type {
  ChatConfig,
  ModelMetadata,
  ModelProvider,
  SearchProviderID,
  SearchServiceConfig,
  Session,
  Workspace,
} from "../../types";
import {
  getSearchCompatibility,
  type SearchCompatibilityResult,
} from "../settings/search";
import { buildDiagramPromptInstruction } from "./diagramPrompt";
import { buildHtmlVisualPromptInstruction } from "./htmlVisualPrompt";
import { parseModelString } from "../utils/model";

export type CapabilityStatusCode =
  | "ok"
  | "search_unavailable"
  | "attachment_unsupported"
  | "audio_unsupported"
  | "reasoning_unsupported";

export interface CapabilityStatus {
  code: CapabilityStatusCode;
  level: "info" | "warning" | "error";
  message: string;
}

export interface ModelCapabilities {
  vision: boolean;
  attachment: boolean;
  audio: boolean;
  reasoning: boolean;
}

export interface EffectiveChatContext {
  sessionId: string | null;
  systemInstruction?: string;
  workspaceFiles: Workspace["files"];
  modelCapabilities: ModelCapabilities;
  searchCompatibility: SearchCompatibilityResult;
  capabilityStatuses: CapabilityStatus[];
}

export interface ResolveEffectiveChatContextOptions {
  session?: Session | null;
  workspace?: Workspace | null;
  systemPrompt?: string;
  enableHtmlVisualPrompt?: boolean;
  now?: Date | number;
  selectedModel: string;
  provider?: Pick<ModelProvider, "type"> | null;
  modelMetadata: Record<string, ModelMetadata>;
  customModelMetadata: Record<string, ModelMetadata>;
  chatConfig: ChatConfig;
  search: {
    provider: SearchProviderID;
    configs: Record<string, SearchServiceConfig>;
  };
}

function formatCurrentDateTime(now: Date | number | undefined): string {
  const date =
    now instanceof Date
      ? now
      : typeof now === "number"
        ? new Date(now)
        : new Date();
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

  return [
    "Current date and time:",
    `- ISO: ${date.toISOString()}`,
    `- Local: ${date.toLocaleString(undefined, { timeZone })}`,
    `- Time zone: ${timeZone}`,
  ].join("\n");
}

function buildSystemInstruction({
  systemPrompt,
  workspacePrompt,
  sessionInstruction,
  enableHtmlVisualPrompt,
  now,
}: {
  systemPrompt?: string;
  workspacePrompt?: string;
  sessionInstruction?: string;
  enableHtmlVisualPrompt?: boolean;
  now?: Date | number;
}) {
  const sections: string[] = [];
  const seen = new Set<string>();
  for (const value of [systemPrompt, workspacePrompt, sessionInstruction]) {
    const trimmed = value?.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    sections.push(trimmed);
  }
  sections.push(
    buildDiagramPromptInstruction({
      enhanced: Boolean(enableHtmlVisualPrompt),
    }),
  );
  if (enableHtmlVisualPrompt) {
    sections.push(buildHtmlVisualPromptInstruction());
  }
  sections.push(formatCurrentDateTime(now));
  return sections.join("\n\n");
}

function getModelCapabilities({
  selectedModel,
  modelMetadata,
  customModelMetadata,
}: Pick<
  ResolveEffectiveChatContextOptions,
  "selectedModel" | "modelMetadata" | "customModelMetadata"
>): ModelCapabilities {
  const { modelName } = parseModelString(selectedModel);
  const meta = customModelMetadata[modelName] || modelMetadata[modelName];
  const lower = modelName.toLowerCase();
  const reasoningByName =
    lower.includes("thinking") ||
    lower.includes("reasoner") ||
    lower.includes("o1") ||
    lower.includes("r1");

  return {
    vision: meta?.modalities?.input?.includes("image") ?? false,
    attachment: meta?.attachment ?? false,
    audio: meta?.modalities?.input?.includes("audio") ?? false,
    reasoning: meta?.reasoning ?? reasoningByName,
  };
}

export function resolveEffectiveChatContext(
  options: ResolveEffectiveChatContextOptions,
): EffectiveChatContext {
  const {
    session,
    workspace,
    systemPrompt,
    enableHtmlVisualPrompt,
    now,
    selectedModel,
    modelMetadata,
    customModelMetadata,
    chatConfig,
    search,
  } = options;

  const searchCompatibility = getSearchCompatibility({
    searchProvider: search.provider,
    searchConfig: search.configs.default,
  });
  const modelCapabilities = getModelCapabilities({
    selectedModel,
    modelMetadata,
    customModelMetadata,
  });
  const statuses: CapabilityStatus[] = [];

  if (chatConfig.useSearch && !searchCompatibility.enabled) {
    statuses.push({
      code: "search_unavailable",
      level: "warning",
      message:
        "Search is enabled but the selected model or provider configuration cannot use it.",
    });
  }

  if (chatConfig.useReasoning && !modelCapabilities.reasoning) {
    statuses.push({
      code: "reasoning_unsupported",
      level: "info",
      message:
        "Reasoning is enabled but the selected model is not marked as reasoning-capable.",
    });
  }

  return {
    sessionId: session?.id || null,
    systemInstruction: buildSystemInstruction({
      systemPrompt,
      workspacePrompt: workspace?.systemPrompt,
      sessionInstruction: session?.systemInstruction,
      enableHtmlVisualPrompt,
      now,
    }),
    workspaceFiles: workspace?.files || [],
    modelCapabilities,
    searchCompatibility,
    capabilityStatuses: statuses.length
      ? statuses
      : [{ code: "ok", level: "info", message: "Ready" }],
  };
}
