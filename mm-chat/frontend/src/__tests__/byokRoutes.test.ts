import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  safeFetchJson: vi.fn(),
  safeFetchText: vi.fn(),
  safeFetchArrayBuffer: vi.fn(),
  decryptSecretEnvelope: vi.fn(),
  decryptOptionalSecret: vi.fn(),
  resolveProviderRuntimeConfig: vi.fn(),
}));

vi.mock("server-only", () => ({}));

vi.mock("@/lib/security/safeFetch", () => ({
  safeFetchJson: mocks.safeFetchJson,
  safeFetchText: mocks.safeFetchText,
  safeFetchArrayBuffer: mocks.safeFetchArrayBuffer,
}));

vi.mock("@/config/limits", async () => vi.importActual("../config/limits"));

vi.mock("@/lib/api/middleware", async () =>
  vi.importActual("../lib/api/middleware"),
);

vi.mock("@/lib/api/schemas", async () => vi.importActual("../lib/api/schemas"));

vi.mock("@/lib/api/uploads", async () => vi.importActual("../lib/api/uploads"));

vi.mock("@/lib/utils/safeServerLog", () => ({
  safeServerLogError: vi.fn(),
  safeServerLogWarn: vi.fn(),
}));

vi.mock("@/lib/providers/base", () => ({
  ProviderFactory: {
    createGeminiClient: vi.fn(),
    createOpenAIClient: vi.fn(),
  },
}));

vi.mock("@/lib/providers/models", async () =>
  vi.importActual("../lib/providers/models"),
);
vi.mock("@/lib/providers/providerTypes", async () =>
  vi.importActual("../lib/providers/providerTypes"),
);

vi.mock("@/lib/security/urlPolicy", async () =>
  vi.importActual("../lib/security/urlPolicy"),
);

vi.mock("@/lib/byok/shared", async () => vi.importActual("../lib/byok/shared"));

vi.mock("@/lib/defaultConfig/server", async () =>
  vi.importActual("../lib/defaultConfig/server"),
);

vi.mock("@/lib/defaultConfig/shared", async () =>
  vi.importActual("../lib/defaultConfig/shared"),
);

vi.mock("@/lib/byok/server", () => ({
  decryptSecretEnvelope: mocks.decryptSecretEnvelope,
  decryptOptionalSecret: mocks.decryptOptionalSecret,
  resolveProviderRuntimeConfig: mocks.resolveProviderRuntimeConfig,
}));

const apiKeySecret = {
  v: 1,
  kid: "test-key",
  alg: "RSA-OAEP-256+A256GCM",
  iv: "iv",
  wrappedKey: "wrapped",
  ciphertext: "ciphertext",
  context: "search:tavily",
} as const;

describe("BYOK route integration", () => {
  beforeEach(() => {
    vi.resetModules();
    mocks.safeFetchJson.mockReset();
    mocks.safeFetchText.mockReset();
    mocks.safeFetchArrayBuffer.mockReset();
    mocks.decryptSecretEnvelope.mockReset();
    mocks.decryptOptionalSecret.mockReset();
    mocks.resolveProviderRuntimeConfig.mockReset();
  });

  it("rejects plaintext voice API keys in transcription multipart requests", async () => {
    const { POST } = await import("../app/api/voice/transcribe/route");
    const formData = new FormData();
    formData.set(
      "audio",
      new File(["audio"], "speech.webm", { type: "audio/webm" }),
    );
    formData.set("provider", "elevenlabs");
    formData.set(
      "apiKeySecret",
      JSON.stringify({ ...apiKeySecret, context: "voice:elevenlabs" }),
    );
    formData.set("apiKey", "voice-plaintext");

    const response = await POST(
      new Request("https://neo.test/api/voice/transcribe", {
        method: "POST",
        headers: { "content-length": "2048" },
        body: formData,
      }) as any,
    );

    expect(response.status).toBe(400);
    expect(mocks.decryptSecretEnvelope).not.toHaveBeenCalled();
    expect(mocks.safeFetchJson).not.toHaveBeenCalled();
    expect(JSON.stringify(await response.json())).not.toContain(
      "voice-plaintext",
    );
  });

  it("rejects transcription multipart requests without a trustworthy content length before parsing", async () => {
    const { POST } = await import("../app/api/voice/transcribe/route");
    const formData = new FormData();
    formData.set(
      "audio",
      new File(["audio"], "speech.webm", { type: "audio/webm" }),
    );
    formData.set("provider", "elevenlabs");
    formData.set(
      "apiKeySecret",
      JSON.stringify({ ...apiKeySecret, context: "voice:elevenlabs" }),
    );

    const response = await POST(
      new Request("https://neo.test/api/voice/transcribe", {
        method: "POST",
        body: formData,
      }) as any,
    );

    expect(response.status).toBe(411);
    expect(await response.json()).toMatchObject({
      code: "LENGTH_REQUIRED",
    });
    expect(mocks.decryptSecretEnvelope).not.toHaveBeenCalled();
    expect(mocks.safeFetchJson).not.toHaveBeenCalled();
  });
});
