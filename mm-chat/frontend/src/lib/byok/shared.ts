export const BYOK_ALG = "RSA-OAEP-256+A256GCM" as const;

export interface EncryptedSecretEnvelope {
  v: 1;
  kid: string;
  alg: typeof BYOK_ALG;
  iv: string;
  wrappedKey: string;
  ciphertext: string;
  context: string;
}

export interface ByokPublicKeyResponse {
  kid: string;
  alg: typeof BYOK_ALG;
  publicKeyJwk: JsonWebKey;
}

export const BYOK_CONTEXTS = {
  provider: (providerType: string) => `provider:${providerType}`,
  searchProvider: (provider: string) => `provider:search:${provider}`,
  ragProvider: (provider: string) => `provider:rag:${provider}`,
  voiceProvider: (provider: string) => `provider:voice:${provider}`,
  elevenLabs: "voice:elevenlabs",
  mimo: "voice:mimo",
  pluginAuth: (pluginId: string) => `plugin:${pluginId}:auth`,
} as const;
