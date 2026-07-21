import type { ModelProvider, VoiceSettings } from "../../types";
import {
  decryptLocalSecret,
  hasLocalSecret,
  LOCAL_SECRET_CONTEXTS,
} from "./localSecrets";

function trimSecret(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

export function hasProviderApiKey(
  provider: Pick<ModelProvider, "apiKey" | "apiKeySecret">,
): boolean {
  return Boolean(
    trimSecret(provider.apiKey) || hasLocalSecret(provider.apiKeySecret),
  );
}

export async function resolveProviderApiKey(
  provider: Pick<ModelProvider, "id" | "apiKey" | "apiKeySecret">,
): Promise<string | undefined> {
  const plain = trimSecret(provider.apiKey);
  if (plain) return plain;

  return decryptLocalSecret(
    provider.apiKeySecret,
    LOCAL_SECRET_CONTEXTS.providerApiKey(provider.id),
  );
}

export function hasElevenLabsApiKey(
  voice: Pick<
    VoiceSettings,
    | "elevenLabsApiKey"
    | "elevenLabsApiKeySecret"
    | "sttProvider"
    | "ttsProvider"
  > & {
    serverElevenLabsAvailable?: boolean;
  },
): boolean {
  return Boolean(
    trimSecret(voice.elevenLabsApiKey) ||
    hasLocalSecret(voice.elevenLabsApiKeySecret) ||
    ((voice.sttProvider === "default" || voice.ttsProvider === "default") &&
      voice.serverElevenLabsAvailable),
  );
}

export async function resolveElevenLabsApiKey(
  voice: Pick<VoiceSettings, "elevenLabsApiKey" | "elevenLabsApiKeySecret">,
): Promise<string | undefined> {
  const plain = trimSecret(voice.elevenLabsApiKey);
  if (plain) return plain;

  return decryptLocalSecret(
    voice.elevenLabsApiKeySecret,
    LOCAL_SECRET_CONTEXTS.elevenLabsApiKey,
  );
}

export function hasMimoApiKey(
  voice: Pick<
    VoiceSettings,
    "mimoApiKey" | "mimoApiKeySecret" | "sttProvider" | "ttsProvider"
  > & {
    serverMimoAvailable?: boolean;
  },
): boolean {
  return Boolean(
    trimSecret(voice.mimoApiKey) ||
    hasLocalSecret(voice.mimoApiKeySecret) ||
    ((voice.sttProvider === "default" || voice.ttsProvider === "default") &&
      voice.serverMimoAvailable),
  );
}

export async function resolveMimoApiKey(
  voice: Pick<VoiceSettings, "mimoApiKey" | "mimoApiKeySecret">,
): Promise<string | undefined> {
  const plain = trimSecret(voice.mimoApiKey);
  if (plain) return plain;

  return decryptLocalSecret(
    voice.mimoApiKeySecret,
    LOCAL_SECRET_CONTEXTS.mimoApiKey,
  );
}

type LocalAuthSecretInput = {
  value?: string;
  localValueSecret?: unknown;
};

export function hasPluginAuthValue(
  auth: LocalAuthSecretInput | undefined,
): boolean {
  return Boolean(
    trimSecret(auth?.value) || hasLocalSecret(auth?.localValueSecret),
  );
}

export async function resolvePluginAuthValue(
  pluginId: string,
  auth: LocalAuthSecretInput | undefined,
): Promise<string | undefined> {
  const plain = trimSecret(auth?.value);
  if (plain) return plain;

  const localValueSecret = hasLocalSecret(auth?.localValueSecret)
    ? auth.localValueSecret
    : undefined;

  return decryptLocalSecret(
    localValueSecret,
    LOCAL_SECRET_CONTEXTS.pluginAuth(pluginId),
  );
}
