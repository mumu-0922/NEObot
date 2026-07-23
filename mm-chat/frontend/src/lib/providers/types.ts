import type { LocalEncryptedSecretEnvelope } from "../security/localSecrets";

export type ProviderType =
  "Gemini" | "OpenAI" | "OpenAI Compatible" | "Anthropic";

export type ModelBuiltInSearchProtocol =
  "openai_responses" | "gemini_google_search" | "anthropic_web_search";

export interface ModelBuiltInSearchConfig {
  protocol?: ModelBuiltInSearchProtocol;
  model?: string;
  source: "official" | "custom" | "none";
  connectionTestValid: boolean;
  connectionTestedAt?: string;
}

export type ToolCapabilityOverride = "auto" | "enabled" | "disabled";

export interface ModelProvider {
  id: string;
  name: string;
  type: ProviderType;
  baseUrl: string;
  apiKey: string;
  apiKeySecret?: LocalEncryptedSecretEnvelope;
  enabled: boolean;
  models: string[];
  modelsList?: string[];
  isServerDefault?: boolean;
  isServerManaged?: boolean;
  modelBuiltInSearch?: ModelBuiltInSearchConfig;
  toolCapabilityDefault: ToolCapabilityOverride;
  toolCapabilityModelOverrides: Record<string, ToolCapabilityOverride>;
}

export interface ModelMetadata {
  id: string;
  name: string;
  family?: string;
  attachment?: boolean;
  reasoning?: boolean;
  tool_call?: boolean;
  temperature?: boolean;
  knowledge?: string;
  release_date?: string;
  last_updated?: string;
  modalities?: {
    input?: string[];
    output?: string[];
  };
  structured_output?: boolean;
  open_weights?: boolean;
  cost?: {
    input: number;
    output: number;
    cache_read?: number;
    cache_write?: number;
    reasoning?: number;
    input_audio?: number;
    output_audio?: number;
  };
  limit?: {
    context?: number;
    output?: number;
  };
}
