import { afterEach, describe, expect, it } from "vitest";
import {
  clearLocalSecretKeyCache,
  decryptLocalSecret,
  deleteLocalSecretMasterKey,
  LOCAL_SECRET_CONTEXTS,
} from "../lib/security/localSecrets";
import {
  migrateProviderLocalSecret,
  migrateVoiceLocalSecrets,
} from "../lib/settings/localSecretMigration";
import { normalizeModelProvider } from "../lib/providers/config";

describe("local secret settings migration", () => {
  afterEach(async () => {
    await deleteLocalSecretMasterKey();
    clearLocalSecretKeyCache();
  });

  it("migrates provider plaintext API keys to encrypted local secrets", async () => {
    const provider = normalizeModelProvider({
      id: "LOCAL1",
      name: "Local Provider",
      type: "Gemini",
      baseUrl: "https://generativelanguage.googleapis.com",
      apiKey: "provider-secret",
      enabled: true,
      models: [],
      modelsList: [],
    })!;
    const migrated = await migrateProviderLocalSecret(provider);

    expect(migrated.apiKey).toBe("");
    expect(JSON.stringify(migrated)).not.toContain("provider-secret");
    await expect(
      decryptLocalSecret(
        migrated.apiKeySecret,
        LOCAL_SECRET_CONTEXTS.providerApiKey("LOCAL1"),
      ),
    ).resolves.toBe("provider-secret");
  });

  it("migrates voice plaintext secrets", async () => {
    const voice = await migrateVoiceLocalSecrets({
      elevenLabsApiKey: "voice-secret",
      mimoApiKey: "mimo-secret",
    });
    const migrated = { voice };

    expect(JSON.stringify(migrated)).not.toContain("voice-secret");
    expect(JSON.stringify(migrated)).not.toContain("mimo-secret");

    await expect(
      decryptLocalSecret(
        voice.elevenLabsApiKeySecret,
        LOCAL_SECRET_CONTEXTS.elevenLabsApiKey,
      ),
    ).resolves.toBe("voice-secret");
    await expect(
      decryptLocalSecret(
        voice.mimoApiKeySecret,
        LOCAL_SECRET_CONTEXTS.mimoApiKey,
      ),
    ).resolves.toBe("mimo-secret");
  });
});
