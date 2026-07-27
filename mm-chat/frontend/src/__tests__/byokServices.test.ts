import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ModelProvider } from "../types";

const mocks = vi.hoisted(() => ({
  coreGetState: vi.fn(),
  getTaskModel: vi.fn(),
  settingsGetState: vi.fn(),
}));

vi.mock("@/store/core/coreSettingsStore", () => ({
  useCoreSettingsStore: {
    getState: mocks.coreGetState,
  },
}));

vi.mock("@/store/core/settingsStore", () => ({
  getTaskModel: mocks.getTaskModel,
  useSettingsStore: {
    getState: mocks.settingsGetState,
  },
}));

vi.mock("@/store/core/memoryStore", () => ({
  useMemoryStore: {
    getState: () => ({
      settings: {
        enabled: false,
        searchEnabled: false,
        autoRecordEnabled: false,
        dreamEnabled: false,
        triggerCount: 100,
        targetCount: 50,
      },
      memories: [],
      markMemoriesUsed: vi.fn(),
    }),
  },
}));

vi.mock("@/utils/pluginUtils", () => ({
  executePluginFunction: vi.fn(),
}));

vi.mock("@/lib/plugin/resolve", () => ({
  getEnabledPluginFunctions: vi.fn(() => []),
}));

vi.mock("@/lib/utils/model", async () => vi.importActual("../lib/utils/model"));

vi.mock("@/lib/chat/entities", async () =>
  vi.importActual("../lib/chat/entities"),
);

vi.mock("@/lib/utils/chatInput", () => ({
  appendContextToChatInput: vi.fn((input) => input),
  clampChatInputText: vi.fn((value) => value),
}));

vi.mock("@/lib/settings/search", () => ({
  getSearchCompatibility: vi.fn(() => ({ enabled: true, mode: "none" })),
  getSearchCompatibilityErrorMessage: vi.fn(() => "Search is unavailable"),
}));

vi.mock("@/lib/utils/contextCompression", () => ({
  createContextCompressionSummaryPrompt: vi.fn(() => ""),
  mergeCompressedContent: vi.fn((value) => value),
  normalizeCompressedContent: vi.fn((value) => value),
  textToBase64: vi.fn((value) => value),
}));

vi.mock("@/lib/utils/disposableAudio", () => ({
  createDisposableAudioFromBlob: vi.fn(),
}));

vi.mock("@/lib/utils/voiceModels", async () =>
  vi.importActual("../lib/utils/voiceModels"),
);

const providerWithoutLocalKey: ModelProvider = {
  id: "env-provider",
  name: "Env Gemini",
  type: "Gemini",
  baseUrl: "https://generativelanguage.googleapis.com",
  apiKey: "",
  enabled: true,
  models: ["gemini-title", "audio-model"],
  modelsList: ["gemini-title", "audio-model"],
  toolCapabilityDefault: "auto",
  toolCapabilityModelOverrides: {},
};

function getJsonRequestBody(fetchMock: ReturnType<typeof vi.fn>, index = 0) {
  return JSON.parse(String(fetchMock.mock.calls[index]?.[1]?.body));
}

function setServerModeEnv(): () => void {
  const previousMode = process.env.NEXT_PUBLIC_API_MODE;
  const previousBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
  process.env.NEXT_PUBLIC_API_MODE = "server";
  process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";

  return () => {
    if (previousMode === undefined) {
      delete process.env.NEXT_PUBLIC_API_MODE;
    } else {
      process.env.NEXT_PUBLIC_API_MODE = previousMode;
    }
    if (previousBaseUrl === undefined) {
      delete process.env.NEXT_PUBLIC_API_BASE_URL;
    } else {
      process.env.NEXT_PUBLIC_API_BASE_URL = previousBaseUrl;
    }
  };
}

describe("BYOK service requests", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
    mocks.coreGetState.mockReturnValue({
      providers: [providerWithoutLocalKey],
    });
    mocks.getTaskModel.mockReturnValue("env-provider:gemini-title");
    mocks.settingsGetState.mockReturnValue({
      search: {
        provider: "google",
        configs: {},
        resultsLimit: 5,
      },
    });
  });

  it("allows chat helper calls to use server env fallback without sending apiKey", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ title: "Generated title" }));
    const { generateChatTitle } = await import("../services/api/chatService");

    await expect(
      generateChatTitle([
        {
          id: "msg-1",
          role: "user",
          content: "hello",
          timestamp: 0,
        },
      ]),
    ).resolves.toBe("Generated title");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/chat/generate-title",
      expect.objectContaining({
        method: "POST",
      }),
    );
    const body = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(body.provider).toMatchObject({
      type: "Gemini",
      name: "Env Gemini",
    });
    expect(JSON.stringify(body)).not.toContain("apiKey");
  });

  it("allows auxiliary provider helpers to use server env fallback", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ output: "ok" }))
      .mockResolvedValueOnce(Response.json({ questions: ["next?"] }))
      .mockResolvedValueOnce(Response.json({ images: [], message: "done" }));
    const { executeCode, generateImage, generateRelatedQuestions } =
      await import("../services/api/chatService");

    await expect(
      executeCode("env-provider:gemini-title", "print('hi')"),
    ).resolves.toBe("ok");
    await expect(generateRelatedQuestions([])).resolves.toEqual(["next?"]);
    await expect(
      generateImage("env-provider:gemini-title", "paint a quiet UI"),
    ).resolves.toMatchObject({ message: "done" });

    expect(fetchMock).toHaveBeenCalledTimes(3);
    for (let index = 0; index < 3; index += 1) {
      expect(
        JSON.stringify(getJsonRequestBody(fetchMock, index)),
      ).not.toContain("apiKey");
    }
    expect(fetchMock).not.toHaveBeenCalledWith(
      "/api/chat/rag-queries",
      expect.anything(),
    );
  });

  it("routes server-mode related questions through Go without sending client history", async () => {
    const previousMode = process.env.NEXT_PUBLIC_API_MODE;
    const previousBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
    process.env.NEXT_PUBLIC_API_MODE = "server";
    process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ questions: ["server next?"] }));
    const { generateRelatedQuestions } =
      await import("../services/api/chatService");

    try {
      await expect(
        generateRelatedQuestions(
          [
            {
              id: "msg-1",
              role: "user",
              content: "client history must stay out",
              timestamp: 0,
            },
          ],
          { conversationId: "conversation-1" },
        ),
      ).resolves.toEqual(["server next?"]);

      expect(fetchMock).toHaveBeenCalledWith(
        "/mm-api/v1/chat/conversations/conversation-1/related-questions",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            modelRef: {
              providerId: "env-provider",
              modelId: "gemini-title",
            },
          }),
        }),
      );
      expect(String(fetchMock.mock.calls[0]?.[1]?.body)).not.toContain(
        "client history must stay out",
      );
    } finally {
      if (previousMode === undefined) {
        delete process.env.NEXT_PUBLIC_API_MODE;
      } else {
        process.env.NEXT_PUBLIC_API_MODE = previousMode;
      }
      if (previousBaseUrl === undefined) {
        delete process.env.NEXT_PUBLIC_API_BASE_URL;
      } else {
        process.env.NEXT_PUBLIC_API_BASE_URL = previousBaseUrl;
      }
    }
  });

  it("routes server-mode content polishing through Go Server Default", async () => {
    const restoreServerMode = setServerModeEnv();
    mocks.coreGetState.mockReturnValue({
      providers: [
        {
          ...providerWithoutLocalKey,
          id: "SERVER_DEFAULT",
          name: "Server Default",
          type: "OpenAI Compatible",
          isServerDefault: true,
        },
      ],
    });
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ text: "polished content" }));
    const { streamGenerateContent } =
      await import("../services/api/chatService");
    const onChunk = vi.fn();

    try {
      await expect(
        streamGenerateContent(
          "SERVER_DEFAULT:gemini-title",
          "polish this",
          onChunk,
        ),
      ).resolves.toBe("polished content");
      expect(onChunk).toHaveBeenCalledOnce();
      expect(onChunk).toHaveBeenCalledWith("polished content");
      expect(fetchMock).toHaveBeenCalledWith(
        "/mm-api/v1/chat/generate",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            modelRef: {
              providerId: "openai_compatible",
              modelId: "gemini-title",
            },
            provider: {
              type: "OpenAI Compatible",
              name: "Server Default",
              source: "server-default",
            },
            prompt: "polish this",
          }),
        }),
      );
      expect(fetchMock.mock.calls[0]?.[0]).not.toBe("/api/chat/generate");
    } finally {
      restoreServerMode();
    }
  });

  it("does not fall back to the legacy related-questions route in server mode", async () => {
    const previousMode = process.env.NEXT_PUBLIC_API_MODE;
    const previousBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
    process.env.NEXT_PUBLIC_API_MODE = "server";
    process.env.NEXT_PUBLIC_API_BASE_URL = "/mm-api";
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValue(new Error("legacy route must not be called"));
    const { generateRelatedQuestions } =
      await import("../services/api/chatService");

    try {
      await expect(generateRelatedQuestions([])).resolves.toEqual([]);
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      if (previousMode === undefined) {
        delete process.env.NEXT_PUBLIC_API_MODE;
      } else {
        process.env.NEXT_PUBLIC_API_MODE = previousMode;
      }
      if (previousBaseUrl === undefined) {
        delete process.env.NEXT_PUBLIC_API_BASE_URL;
      } else {
        process.env.NEXT_PUBLIC_API_BASE_URL = previousBaseUrl;
      }
    }
  });

  it("fails closed for server-mode code but routes image jobs through Go", async () => {
    const restoreServerMode = setServerModeEnv();
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        images: [
          {
            fileId: "33333333-3333-4333-8333-333333333333",
            purpose: "image",
            contentType: "image/png",
            size: 5,
          },
        ],
        message: "stored",
      }),
    );
    const { executeCode, generateImage } =
      await import("../services/api/chatService");

    try {
      await expect(
        executeCode("env-provider:gemini-title", "print('hi')"),
      ).resolves.toContain("server code execution jobs");
      await expect(
        generateImage("env-provider:gemini-title", "paint a quiet UI"),
      ).resolves.toEqual({
        images: [
          expect.objectContaining({
            id: "33333333-3333-4333-8333-333333333333",
            source: "server",
            fileId: "33333333-3333-4333-8333-333333333333",
            fileName: "generated-1.png",
            mimeType: "image/png",
            size: 5,
            purpose: "image",
            url: "/mm-api/v1/files/33333333-3333-4333-8333-333333333333/content",
          }),
        ],
        message: "stored",
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock).toHaveBeenCalledWith(
        "/mm-api/v1/images/generations",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            modelRef: {
              providerId: "env-provider",
              modelId: "gemini-title",
            },
            prompt: "paint a quiet UI",
            count: 1,
          }),
        }),
      );
      const body = getJsonRequestBody(fetchMock);
      expect(JSON.stringify(body)).not.toContain("apiKey");
      expect(fetchMock.mock.calls[0]?.[0]).not.toBe("/api/chat/generate-image");
    } finally {
      restoreServerMode();
    }
  });

  it("fails closed for server-mode voice transcription without calling legacy routes", async () => {
    const restoreServerMode = setServerModeEnv();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValue(new Error("legacy route must not be called"));
    const { transcribeAudio } = await import("../services/api/voiceService");

    try {
      await expect(
        transcribeAudio(new Blob(["audio"], { type: "audio/webm" }), {
          sttProvider: "default",
        } as any),
      ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      restoreServerMode();
    }
  });

  it("routes server default TTS through Go metadata and authenticated file fetch", async () => {
    const restoreServerMode = setServerModeEnv();
    const fileId = "33333333-3333-4333-8333-333333333333";
    const messageId = "22222222-2222-4222-8222-222222222222";
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input, init) => {
        const url = String(input);
        if (url === "/mm-api/v1/voice/synthesize") {
          return Response.json({
            fileId,
            purpose: "audio",
            contentType: "audio/mpeg",
            size: 5,
            cached: false,
          });
        }
        if (url === `/mm-api/v1/files/${fileId}/content`) {
          return new Response("audio", {
            status: 200,
            headers: {
              "content-type": "audio/mpeg",
              "content-length": "5",
            },
          });
        }
        throw new Error(`unexpected request ${url} ${String(init?.method)}`);
      });
    const { synthesizeSpeech } = await import("../services/api/voiceService");

    try {
      await expect(
        synthesizeSpeech("hello", { ttsProvider: "default" } as any, messageId),
      ).resolves.toBeUndefined();

      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(fetchMock.mock.calls[0]?.[0]).toBe("/mm-api/v1/voice/synthesize");
      expect(getJsonRequestBody(fetchMock)).toEqual({
        messageId,
        text: "hello",
        provider: "default",
      });
      expect(fetchMock.mock.calls[1]?.[0]).toBe(
        `/mm-api/v1/files/${fileId}/content`,
      );
      expect(
        fetchMock.mock.calls.some(([url]) =>
          String(url).startsWith("/api/voice/"),
        ),
      ).toBe(false);
    } finally {
      restoreServerMode();
    }
  });

  it("keeps browser TTS local in server mode without calling legacy routes", async () => {
    const restoreServerMode = setServerModeEnv();
    const speakMock = vi.fn();
    const getVoicesMock = vi.fn(() => [
      { lang: "zh-CN", name: "Local Chinese" },
    ]);
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValue(new Error("legacy route must not be called"));

    class FakeSpeechSynthesisUtterance {
      lang = "";
      voice?: SpeechSynthesisVoice;

      constructor(public readonly text: string) {}
    }

    vi.stubGlobal("SpeechSynthesisUtterance", FakeSpeechSynthesisUtterance);
    vi.stubGlobal("navigator", { language: "zh-CN" });
    vi.stubGlobal("window", {
      speechSynthesis: {
        getVoices: getVoicesMock,
        speak: speakMock,
      },
    });

    const { synthesizeSpeech } = await import("../services/api/voiceService");

    try {
      await expect(
        synthesizeSpeech("你好，魔尊", {
          ttsProvider: "browser",
          ttsLanguage: "zh",
        } as any),
      ).resolves.toBeUndefined();

      expect(fetchMock).not.toHaveBeenCalled();
      expect(getVoicesMock).toHaveBeenCalledTimes(1);
      expect(speakMock).toHaveBeenCalledTimes(1);
      expect(speakMock.mock.calls[0]?.[0]).toMatchObject({
        text: "你好，魔尊",
        lang: "zh-CN",
      });
    } finally {
      vi.unstubAllGlobals();
      restoreServerMode();
    }
  });

  it("rejects non-default hosted TTS choices without calling legacy routes", async () => {
    const restoreServerMode = setServerModeEnv();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValue(new Error("legacy route must not be called"));
    const { synthesizeSpeech } = await import("../services/api/voiceService");

    try {
      for (const ttsProvider of ["elevenlabs", "mimo", "model"] as const) {
        await expect(
          synthesizeSpeech("hello", { ttsProvider } as any),
        ).rejects.toMatchObject({ code: "FEATURE_NOT_IMPLEMENTED" });
      }
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      restoreServerMode();
    }
  });

  it("allows voice model STT to use server env fallback without sending apiKey", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ text: "Transcript" }));
    const { transcribeAudio } = await import("../services/api/voiceService");

    await expect(
      transcribeAudio(new Blob(["audio"], { type: "audio/webm" }), {
        sttProvider: "model",
        sttModel: "env-provider:audio-model",
        sttLanguage: "auto",
      } as any),
    ).resolves.toBe("Transcript");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/voice/transcribe",
      expect.objectContaining({
        method: "POST",
      }),
    );
    const body = fetchMock.mock.calls[0]?.[1]?.body as FormData;
    const modelProvider = JSON.parse(String(body.get("modelProvider")));
    expect(modelProvider).toMatchObject({
      type: "Gemini",
      name: "Env Gemini",
    });
    expect(JSON.stringify(modelProvider)).not.toContain("apiKey");
  });

  it("allows voice model TTS to use server env fallback without sending apiKey", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response("audio", { status: 200 }));
    const { synthesizeSpeech } = await import("../services/api/voiceService");

    await expect(
      synthesizeSpeech("hello", {
        ttsProvider: "model",
        ttsModel: "env-provider:audio-model",
      } as any),
    ).resolves.toBeUndefined();

    const body = getJsonRequestBody(fetchMock);
    expect(body.modelProvider).toMatchObject({
      type: "Gemini",
      name: "Env Gemini",
    });
    expect(JSON.stringify(body)).not.toContain("apiKey");
  });
});
