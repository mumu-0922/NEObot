import type { ModelProvider, VoiceSettings } from "../../types";
import {
  encryptLocalSecret,
  hasLocalSecret,
  LOCAL_SECRET_CONTEXTS,
  type LocalEncryptedSecretEnvelope,
} from "../security/localSecrets";

export async function migrateLocalSecretField(
  plainSecret: string | undefined,
  existingSecret: unknown,
  context: string,
): Promise<LocalEncryptedSecretEnvelope | undefined> {
  const trimmed = plainSecret?.trim();
  if (trimmed) {
    return encryptLocalSecret(trimmed, context);
  }

  return hasLocalSecret(existingSecret) ? existingSecret : undefined;
}

export async function migrateProviderLocalSecret(
  provider: ModelProvider,
): Promise<ModelProvider> {
  const apiKeySecret = await migrateLocalSecretField(
    provider.apiKey,
    provider.apiKeySecret,
    LOCAL_SECRET_CONTEXTS.providerApiKey(provider.id),
  );

  return {
    ...provider,
    apiKey: "",
    ...(apiKeySecret ? { apiKeySecret } : {}),
  };
}

export function stripProviderPlainSecret(
  provider: ModelProvider,
): ModelProvider {
  return {
    ...provider,
    apiKey: "",
  };
}

export async function migrateVoiceLocalSecrets(
  voice: Partial<VoiceSettings> | undefined,
): Promise<VoiceSettings> {
  const normalized: VoiceSettings = {
    sttProvider: "browser",
    sttModel: "",
    sttLanguage: "auto",
    ttsProvider: "browser",
    ttsModel: "",
    ttsVoiceId: "bIHbv24MWmeRgasZH58o",
    ttsLanguage: "auto",
    elevenLabsApiKey: "",
    mimoApiKey: "",
    mimoTtsVoiceId: "mimo_default",
    autoTranscribe: true,
    ...voice,
  };
  const elevenLabsApiKeySecret = await migrateLocalSecretField(
    normalized.elevenLabsApiKey,
    normalized.elevenLabsApiKeySecret,
    LOCAL_SECRET_CONTEXTS.elevenLabsApiKey,
  );
  const mimoApiKeySecret = await migrateLocalSecretField(
    normalized.mimoApiKey,
    normalized.mimoApiKeySecret,
    LOCAL_SECRET_CONTEXTS.mimoApiKey,
  );

  return {
    ...normalized,
    elevenLabsApiKey: "",
    mimoApiKey: "",
    ...(elevenLabsApiKeySecret ? { elevenLabsApiKeySecret } : {}),
    ...(mimoApiKeySecret ? { mimoApiKeySecret } : {}),
  };
}

export function stripVoicePlainSecrets(voice: VoiceSettings): VoiceSettings {
  return {
    ...voice,
    elevenLabsApiKey: "",
    mimoApiKey: "",
  };
}
