import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PublicServerConfig } from "../lib/defaultConfig/shared";
import { SERVER_DEFAULT_PROVIDER_ID } from "../lib/defaultConfig/shared";

vi.mock("server-only", () => ({}));

vi.mock("@/config/api", async () => vi.importActual("../config/api"));
vi.mock("@/config/defaults", async () => vi.importActual("../config/defaults"));
vi.mock("@/config/limits", async () => vi.importActual("../config/limits"));
vi.mock("@/lib/defaultConfig/shared", async () =>
  vi.importActual("../lib/defaultConfig/shared"),
);
vi.mock("@/lib/market/agents", async () =>
  vi.importActual("../lib/market/agents"),
);
vi.mock("@/lib/providers/config", async () =>
  vi.importActual("../lib/providers/config"),
);
vi.mock("@/lib/providers/providerTypes", async () =>
  vi.importActual("../lib/providers/providerTypes"),
);
vi.mock("@/lib/providers/metadata", async () =>
  vi.importActual("../lib/providers/metadata"),
);
vi.mock("@/lib/security/urlPolicy", async () =>
  vi.importActual("../lib/security/urlPolicy"),
);
vi.mock("@/lib/utils/defaultModels", async () =>
  vi.importActual("../lib/utils/defaultModels"),
);

const serverConfig: PublicServerConfig = {
  modelProvider: {
    available: true,
    id: SERVER_DEFAULT_PROVIDER_ID,
    name: "Hosted Default",
    type: "Gemini",
    models: ["gemini-default"],
    defaultModels: {
      titleGeneration: "gemini-default",
      relatedQuestions: "gemini-default",
      memory: "gemini-default",
    },
    modelMetadata: {},
  },
  search: {
    available: true,
  },
  mcp: {
    enabled: true,
    remoteEnabled: true,
    stdioEnabled: false,
  },
  voice: {
    elevenLabsAvailable: false,
    mimoAvailable: false,
    defaultSttAvailable: false,
    defaultTtsAvailable: false,
  },
};

describe("server default store injection", () => {
  beforeEach(async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");
    const { useCoreSettingsStore } =
      await import("../store/core/coreSettingsStore");

    useSettingsStore.setState(useSettingsStore.getInitialState(), true);
    useCoreSettingsStore.setState(useCoreSettingsStore.getInitialState(), true);
  });

  it("keeps Search server-owned when applying runtime config", async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");

    useSettingsStore.getState().applyServerConfig(serverConfig);
    expect(useSettingsStore.getState().search.provider).toBe("default");

    useSettingsStore.setState(useSettingsStore.getInitialState(), true);
    useSettingsStore.getState().applyServerConfig(serverConfig);
    expect(useSettingsStore.getState().search).toMatchObject({
      provider: "default",
      configs: { default: { serverAvailable: true } },
    });
  });

  it("selects hosted SiliconFlow TTS without copying its voice into ElevenLabs state", async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");
    const initialVoiceId = useSettingsStore.getState().voice.ttsVoiceId;

    useSettingsStore.getState().applyServerConfig({
      ...serverConfig,
      voice: {
        ...serverConfig.voice,
        defaultProvider: "siliconflow",
        defaultSttAvailable: false,
        defaultTtsAvailable: true,
        ttsModel: "FunAudioLLM/CosyVoice2-0.5B",
        ttsVoiceId: "FunAudioLLM/CosyVoice2-0.5B:claire",
      },
    });

    expect(useSettingsStore.getState().voice).toMatchObject({
      ttsProvider: "default",
      ttsVoiceId: initialVoiceId,
      serverDefaultVoiceProvider: "siliconflow",
      serverDefaultSttAvailable: false,
      serverDefaultTtsAvailable: true,
    });
  });

  it("selects newly activated hosted TTS for an existing disabled server snapshot", async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");
    useSettingsStore.setState((state) => ({
      voice: {
        ...state.voice,
        ttsProvider: "browser",
        serverDefaultTtsAvailable: false,
      },
    }));

    const hostedVoiceConfig = {
      ...serverConfig,
      voice: {
        ...serverConfig.voice,
        defaultProvider: "siliconflow" as const,
        defaultTtsAvailable: true,
        ttsModel: "FunAudioLLM/CosyVoice2-0.5B",
        ttsVoiceId: "FunAudioLLM/CosyVoice2-0.5B:claire",
      },
    };
    useSettingsStore.getState().applyServerConfig(hostedVoiceConfig);
    expect(useSettingsStore.getState().voice.ttsProvider).toBe("default");

    useSettingsStore.getState().updateVoiceSettings({ ttsProvider: "browser" });
    useSettingsStore.getState().applyServerConfig(hostedVoiceConfig);
    expect(useSettingsStore.getState().voice.ttsProvider).toBe("browser");
  });

  it("removes unsupported local Voice providers when hosted config becomes authoritative", async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");
    useSettingsStore.setState((state) => ({
      voice: {
        ...state.voice,
        sttProvider: "mimo",
        ttsProvider: "elevenlabs",
      },
    }));

    useSettingsStore.getState().applyServerConfig({
      ...serverConfig,
      voice: {
        ...serverConfig.voice,
        defaultProvider: "siliconflow",
        defaultTtsAvailable: true,
      },
    });

    expect(useSettingsStore.getState().voice).toMatchObject({
      sttProvider: "browser",
      ttsProvider: "default",
    });
  });

  it("seeds missing task-model defaults without overwriting persisted user choices", async () => {
    const { useCoreSettingsStore } =
      await import("../store/core/coreSettingsStore");

    useCoreSettingsStore.setState((state) => ({
      ...state,
      serverDefaultProviderEnabled: false,
      providers: [
        {
          id: "GEMINI",
          name: "Google Gemini",
          type: "Gemini",
          baseUrl: "https://generativelanguage.googleapis.com",
          apiKey: "user-key",
          enabled: true,
          models: ["gemini-flash-latest"],
          modelsList: ["gemini-flash-latest"],
          toolCapabilityDefault: "auto",
          toolCapabilityModelOverrides: {},
        },
        {
          id: "CUSTOM",
          name: "Custom",
          type: "OpenAI",
          baseUrl: "https://api.example.com",
          apiKey: "user-key",
          enabled: true,
          models: ["custom-model"],
          modelsList: ["custom-model"],
          toolCapabilityDefault: "auto",
          toolCapabilityModelOverrides: {},
        },
      ],
      defaultModels: {
        titleGeneration: "GEMINI:gemini-flash-latest",
        relatedQuestions: "",
        contextCompression: "GEMINI:gemini-flash-latest",
        promptOptimization: "",
        ragQuery: "CUSTOM:custom-model",
        memory: "GEMINI:gemini-flash-latest",
        recallFiltering: "",
      },
    }));

    useCoreSettingsStore.getState().applyServerConfig(serverConfig);

    const provider = useCoreSettingsStore
      .getState()
      .providers.find((item) => item.id === SERVER_DEFAULT_PROVIDER_ID);
    expect(provider?.enabled).toBe(true);
    expect(useCoreSettingsStore.getState().defaultModels).toEqual({
      titleGeneration: "GEMINI:gemini-flash-latest",
      relatedQuestions: "SERVER_DEFAULT:gemini-default",
      contextCompression: "GEMINI:gemini-flash-latest",
      promptOptimization: "",
      ragQuery: "CUSTOM:custom-model",
      memory: "GEMINI:gemini-flash-latest",
      recallFiltering: "",
    });
  });

  it("replaces browser task models when the server reports authoritative settings", async () => {
    const { useCoreSettingsStore } =
      await import("../store/core/coreSettingsStore");

    useCoreSettingsStore.setState((state) => ({
      ...state,
      defaultModels: {
        titleGeneration: "LOCAL:old-title",
        relatedQuestions: "LOCAL:old-related",
        contextCompression: "LOCAL:old-compression",
        promptOptimization: "LOCAL:old-polish",
        ragQuery: "LOCAL:old-rag",
        memory: "LOCAL:old-memory",
        recallFiltering: "LOCAL:old-recall",
      },
    }));

    useCoreSettingsStore.getState().applyServerConfig({
      ...serverConfig,
      modelProvider: {
        ...serverConfig.modelProvider,
        defaultModelsConfigured: true,
        defaultModels: {
          titleGeneration: "CUSTOM:gpt-title",
          relatedQuestions: "gemini-default",
          contextCompression: "CUSTOM:gpt-compress",
          promptOptimization: "CUSTOM:gpt-polish",
          ragQuery: "CUSTOM:gpt-rag",
          memory: "CUSTOM:gpt-memory",
          recallFiltering: "CUSTOM:gpt-5.6-luna",
        },
      },
    });

    expect(useCoreSettingsStore.getState().defaultModels).toEqual({
      titleGeneration: "CUSTOM:gpt-title",
      relatedQuestions: "SERVER_DEFAULT:gemini-default",
      contextCompression: "CUSTOM:gpt-compress",
      promptOptimization: "CUSTOM:gpt-polish",
      ragQuery: "CUSTOM:gpt-rag",
      memory: "CUSTOM:gpt-memory",
      recallFiltering: "CUSTOM:gpt-5.6-luna",
    });
  });

  it("does not persist server-owned task models in browser preferences", async () => {
    const { useCoreSettingsStore } =
      await import("../store/core/coreSettingsStore");
    const partialize = (useCoreSettingsStore as any).persist.getOptions()
      .partialize;

    expect(partialize(useCoreSettingsStore.getState())).not.toHaveProperty(
      "defaultModels",
    );
  });

  it("initializes missing server model metadata without overwriting user edits", async () => {
    const { useSettingsStore } = await import("../store/core/settingsStore");

    useSettingsStore.setState((state) => ({
      ...state,
      customModelMetadata: {
        "user-edited": {
          id: "user-edited",
          name: "User Edited Name",
          reasoning: false,
        },
      },
    }));

    useSettingsStore.getState().applyServerConfig({
      ...serverConfig,
      modelProvider: {
        ...serverConfig.modelProvider,
        models: ["server-new", "user-edited"],
        modelMetadata: {
          "server-new": {
            id: "server-new",
            name: "Server New",
            tool_call: true,
          },
          "user-edited": {
            id: "user-edited",
            name: "Server Name",
            reasoning: true,
          },
        },
      },
    });

    expect(useSettingsStore.getState().customModelMetadata).toMatchObject({
      "server-new": {
        id: "server-new",
        name: "Server New",
        tool_call: true,
      },
      "user-edited": {
        id: "user-edited",
        name: "User Edited Name",
        reasoning: false,
      },
    });
  });

  it("removes only the unmodified legacy Gemini provider during migration", async () => {
    const { useCoreSettingsStore } =
      await import("../store/core/coreSettingsStore");
    const migrate = (useCoreSettingsStore as any).persist.getOptions().migrate;

    const migrated = await migrate(
      {
        providers: [
          {
            id: "GEMINI",
            name: "Google Gemini",
            type: "Gemini",
            baseUrl: "https://generativelanguage.googleapis.com",
            apiKey: "",
            enabled: true,
            models: ["gemini-flash-latest"],
            modelsList: ["gemini-flash-latest"],
          },
          {
            id: "GEMINI_CUSTOM",
            name: "My Gemini",
            type: "Gemini",
            baseUrl: "https://generativelanguage.googleapis.com",
            apiKey: "user-key",
            enabled: true,
            models: ["gemini-flash-latest"],
            modelsList: ["gemini-flash-latest"],
          },
        ],
        defaultModels: {
          titleGeneration: "GEMINI:gemini-flash-latest",
          relatedQuestions: "GEMINI_CUSTOM:gemini-flash-latest",
          contextCompression: "",
          promptOptimization: "",
          ragQuery: "",
          memory: "",
        },
      },
      3,
    );

    expect(migrated.providers.map((provider: any) => provider.id)).toEqual([
      "GEMINI_CUSTOM",
    ]);
    expect(migrated.defaultModels).toMatchObject({
      titleGeneration: "",
      relatedQuestions: "GEMINI_CUSTOM:gemini-flash-latest",
      memory: "",
    });
  });
});
