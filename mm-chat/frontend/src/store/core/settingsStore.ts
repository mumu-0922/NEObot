import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import {
  ModelMetadata,
  SearchProviderID,
  SearchServiceConfig,
  LobeAgent,
  VoiceSettings,
  SystemSettings,
  DefaultModels,
} from "@/types";
import { DEFAULT_SYSTEM_SETTINGS } from "@/config/defaults";
import { PublicServerConfig } from "@/lib/defaultConfig/shared";
import {
  STORAGE_KEYS,
  STORAGE_VERSION,
  getAppDbStorage,
} from "../storage/storageConfig";
import { CACHE_CONFIG } from "@/config/api";
import { useCoreSettingsStore } from "./coreSettingsStore";
import { normalizeProviderBaseUrl } from "@/lib/security/urlPolicy";
import {
  normalizeLocalAgent,
  normalizeLocalAgents,
  normalizeMarketAgents,
} from "@/lib/market/agents";
import type { AgentMarketLocale } from "@/lib/market/agentLocale";
import { MARKET_LIMITS } from "@/config/limits";
import {
  extractKnownProviderModelMetadata,
  normalizeModelMetadata,
  normalizeModelMetadataMap,
} from "@/lib/providers/metadata";
import { logDevError } from "../../lib/utils/devLogger";
import { normalizeSearchSettings } from "../../lib/settings/search";
import { getDefaultModelSelectValue } from "../../lib/utils/defaultModels";
import { isElevenLabsVoiceId } from "../../lib/utils/voiceModels";
import { readJsonResponseOrThrow } from "../../lib/api/client";
import { normalizeSystemSettings } from "../../lib/settings/appConfig";
import { clearBrowserAppData } from "../../lib/data/clearAppData";
import {
  createBrowserAppExportPayload,
  type AppExportPayload,
} from "../../lib/data/appExport";
import {
  migrateVoiceLocalSecrets,
  stripVoicePlainSecrets,
} from "../../lib/settings/localSecretMigration";
import { stripRetiredLegacySkillSettings } from "../storage/legacySkillRetirement";

interface SettingsState {
  _hasHydrated: boolean;
  setHasHydrated: (state: boolean) => void;
  serverConfig: PublicServerConfig | null;
  applyServerConfig: (config: PublicServerConfig) => void;

  // Market Cache
  marketAgents: LobeAgent[];
  marketAgentsTimestamp: number;
  marketAgentsLocale: AgentMarketLocale | "";
  setMarketAgents: (
    agents: LobeAgent[],
    locale?: AgentMarketLocale | "",
  ) => void;

  // System Settings
  system: SystemSettings;
  updateSystemSettings: (settings: Partial<SystemSettings>) => void;

  // Model Metadata
  modelMetadata: Record<string, ModelMetadata>;
  modelMetadataTimestamp: number;
  customModelMetadata: Record<string, ModelMetadata>;
  setCustomModelMetadata: (id: string, meta: ModelMetadata) => void;
  fetchModelMetadata: (forceRefresh?: boolean) => Promise<void>;

  // Search Settings
  search: {
    provider: SearchProviderID;
    resultsLimit: number;
    configs: Record<string, SearchServiceConfig>;
  };
  setSearchResultsLimit: (limit: number) => void;

  // Voice Settings
  voice: VoiceSettings;
  updateVoiceSettings: (settings: Partial<VoiceSettings>) => void;

  // Agent Management
  customAgents: LobeAgent[];
  usedAgents: LobeAgent[];
  agentOverrides: Record<string, Partial<LobeAgent>>;
  addCustomAgent: (agent: LobeAgent) => void;
  updateAgent: (
    identifier: string,
    updates: Partial<LobeAgent>,
    isCustom: boolean,
  ) => void;
  removeLocalAgent: (identifier: string) => void;
  recordUsedAgent: (agent: LobeAgent) => void;
  resetAgent: (identifier: string) => void;

  // Data Management
  exportAllData: () => Promise<AppExportPayload>;
  clearAllData: () => Promise<void>;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set, get) => ({
      _hasHydrated: false,
      setHasHydrated: (state) => set({ _hasHydrated: state }),
      serverConfig: null,
      applyServerConfig: (config) =>
        set((state) => {
          let nextSttProvider = state.voice.sttProvider;
          if (nextSttProvider !== "default" && nextSttProvider !== "browser") {
            nextSttProvider = config.voice.defaultSttAvailable
              ? "default"
              : "browser";
          } else if (
            nextSttProvider === "default" &&
            !config.voice.defaultSttAvailable
          ) {
            nextSttProvider = "browser";
          } else if (
            nextSttProvider === "browser" &&
            config.voice.defaultSttAvailable &&
            state.voice.serverDefaultSttAvailable !== true
          ) {
            nextSttProvider = "default";
          }

          let nextTtsProvider = state.voice.ttsProvider;
          if (nextTtsProvider !== "default" && nextTtsProvider !== "browser") {
            nextTtsProvider = config.voice.defaultTtsAvailable
              ? "default"
              : "browser";
          } else if (
            nextTtsProvider === "default" &&
            !config.voice.defaultTtsAvailable
          ) {
            nextTtsProvider = "browser";
          } else if (
            nextTtsProvider === "browser" &&
            config.voice.defaultTtsAvailable &&
            state.voice.serverDefaultTtsAvailable !== true
          ) {
            nextTtsProvider = "default";
          }

          const shouldUseDefaultStt =
            nextSttProvider === "default" &&
            state.voice.sttProvider !== "default";
          const shouldUseDefaultTts =
            nextTtsProvider === "default" &&
            state.voice.ttsProvider !== "default";

          const isSystemUnchanged =
            JSON.stringify(state.system) ===
            JSON.stringify(DEFAULT_SYSTEM_SETTINGS);
          const serverModelMetadata = normalizeModelMetadataMap(
            config.modelProvider.modelMetadata,
          );
          const nextCustomModelMetadata = { ...state.customModelMetadata };
          for (const [id, metadata] of Object.entries(serverModelMetadata)) {
            if (!nextCustomModelMetadata[id]) {
              nextCustomModelMetadata[id] = metadata;
            }
          }

          return {
            serverConfig: config,
            customModelMetadata: nextCustomModelMetadata,
            search: normalizeSearchSettings({
              ...state.search,
              configs: {
                default: { serverAvailable: config.search.available },
              },
            }),
            voice: {
              ...state.voice,
              serverDefaultVoiceProvider: config.voice.defaultProvider,
              serverDefaultSttAvailable: config.voice.defaultSttAvailable,
              serverDefaultTtsAvailable: config.voice.defaultTtsAvailable,
              serverElevenLabsAvailable: config.voice.elevenLabsAvailable,
              serverElevenLabsTtsModel:
                config.voice.defaultProvider === "elevenlabs"
                  ? config.voice.ttsModel
                  : undefined,
              serverMimoAvailable: config.voice.mimoAvailable,
              serverMimoSttModel: config.voice.mimoSttModel,
              serverMimoTtsModel: config.voice.mimoTtsModel,
              serverMimoTtsVoiceId: config.voice.mimoTtsVoiceId,
              sttProvider: nextSttProvider,
              ttsProvider: nextTtsProvider,
              ...(nextSttProvider === "browser" ? { sttModel: "" } : {}),
              ...(shouldUseDefaultStt
                ? {
                    sttProvider: "default" as const,
                    sttModel: config.voice.sttModel || "",
                  }
                : {}),
              ...(shouldUseDefaultTts
                ? {
                    ttsProvider: "default" as const,
                    ...(config.voice.defaultProvider === "elevenlabs" &&
                    config.voice.ttsModel
                      ? { ttsModel: config.voice.ttsModel }
                      : {}),
                    ...(config.voice.defaultProvider === "mimo"
                      ? {
                          mimoTtsVoiceId:
                            config.voice.mimoTtsVoiceId || "mimo_default",
                        }
                      : {}),
                    ...(config.voice.defaultProvider === "elevenlabs" &&
                    isElevenLabsVoiceId(config.voice.ttsVoiceId)
                      ? { ttsVoiceId: config.voice.ttsVoiceId }
                      : {}),
                  }
                : {}),
            },
            ...(config.system && isSystemUnchanged
              ? { system: normalizeSystemSettings(config.system) }
              : {}),
          };
        }),

      // Market Cache
      marketAgents: [],
      marketAgentsTimestamp: 0,
      marketAgentsLocale: "",
      setMarketAgents: (agents, locale = "") =>
        set({
          marketAgents: normalizeMarketAgents(agents),
          marketAgentsTimestamp: Date.now(),
          marketAgentsLocale: locale,
        }),

      // System Settings
      system: DEFAULT_SYSTEM_SETTINGS,
      updateSystemSettings: (settings) =>
        set((state) => ({
          system: normalizeSystemSettings(
            { ...state.system, ...settings },
            DEFAULT_SYSTEM_SETTINGS,
          ),
        })),

      // Model Metadata
      modelMetadata: {},
      modelMetadataTimestamp: 0,
      customModelMetadata: {},
      setCustomModelMetadata: (id, meta) =>
        set((state) => {
          const metadata = normalizeModelMetadata(meta, id);
          if (!metadata) return state;

          return {
            customModelMetadata: {
              ...state.customModelMetadata,
              [metadata.id]: metadata,
            },
          };
        }),

      fetchModelMetadata: async (forceRefresh = false) => {
        const { modelMetadata, modelMetadataTimestamp } = get();
        const now = Date.now();
        if (
          !forceRefresh &&
          Object.keys(modelMetadata).length > 0 &&
          modelMetadataTimestamp &&
          now - modelMetadataTimestamp < CACHE_CONFIG.modelMetadata
        ) {
          return;
        }

        try {
          const response = await fetch(
            "https://basellm.github.io/llm-metadata/api/all.json",
          );
          if (!response.ok) throw new Error("Failed to fetch model metadata");

          const data = await readJsonResponseOrThrow(
            response,
            "Failed to fetch model metadata",
          );
          const newMetadata = extractKnownProviderModelMetadata(data);

          set({ modelMetadata: newMetadata, modelMetadataTimestamp: now });
        } catch (e) {
          logDevError("Error fetching model metadata:", e);
        }
      },

      // Search Settings
      search: {
        provider: "default",
        resultsLimit: 5,
        configs: {
          default: { serverAvailable: false },
        },
      },
      setSearchResultsLimit: (limit) =>
        set((state) => ({
          search: normalizeSearchSettings({
            ...state.search,
            resultsLimit: limit,
          }),
        })),

      // Voice Settings
      voice: {
        sttProvider: "browser",
        sttModel: "",
        sttLanguage: "auto",
        ttsProvider: "browser",
        ttsModel: "",
        ttsVoiceId: "bIHbv24MWmeRgasZH58o",
        mimoTtsVoiceId: "mimo_default",
        ttsLanguage: "auto",
        elevenLabsApiKey: "",
        mimoApiKey: "",
        autoTranscribe: true,
      },
      updateVoiceSettings: (settings) =>
        set((state) => ({ voice: { ...state.voice, ...settings } })),

      // Agent Management
      customAgents: [],
      usedAgents: [],
      agentOverrides: {},

      addCustomAgent: (agent) =>
        set((state) => {
          const normalizedAgent = normalizeLocalAgent({
            ...agent,
            isCustom: true,
          });
          if (!normalizedAgent) return state;

          return {
            customAgents: normalizeLocalAgents(
              [normalizedAgent, ...state.customAgents],
              MARKET_LIMITS.maxCustomAgents,
            ),
          };
        }),

      updateAgent: (identifier, updates, isCustom) =>
        set((state) => {
          if (isCustom) {
            let changed = false;
            const customAgents = state.customAgents.map((a) => {
              if (a.identifier !== identifier) return a;

              const normalizedAgent = normalizeLocalAgent({
                ...a,
                ...updates,
                meta: { ...a.meta, ...updates.meta },
                isCustom: true,
              });
              if (!normalizedAgent) return a;
              changed = true;
              return normalizedAgent;
            });

            if (!changed) return state;

            return {
              customAgents: normalizeLocalAgents(
                customAgents,
                MARKET_LIMITS.maxCustomAgents,
              ),
            } as Partial<SettingsState>;
          }

          const currentOverride = state.agentOverrides[identifier] || {};
          const newUsedAgents = state.usedAgents.map((a) =>
            a.identifier === identifier
              ? normalizeLocalAgent({
                  ...a,
                  ...updates,
                  meta: { ...a.meta, ...updates.meta },
                }) || a
              : a,
          );
          const normalizedOverride = normalizeLocalAgent({
            identifier,
            ...currentOverride,
            ...updates,
            meta: { ...currentOverride.meta, ...updates.meta },
          });

          return {
            agentOverrides: {
              ...state.agentOverrides,
              ...(normalizedOverride
                ? { [identifier]: normalizedOverride }
                : {}),
            },
            usedAgents: normalizeLocalAgents(
              newUsedAgents,
              MARKET_LIMITS.maxUsedAgents,
            ),
          } as Partial<SettingsState>;
        }),

      removeLocalAgent: (identifier) =>
        set((state) => {
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          const { [identifier]: _removed, ...newOverrides } =
            state.agentOverrides;
          return {
            customAgents: state.customAgents.filter(
              (a) => a.identifier !== identifier,
            ),
            usedAgents: state.usedAgents.filter(
              (a) => a.identifier !== identifier,
            ),
            agentOverrides: newOverrides,
          };
        }),

      recordUsedAgent: (agent) =>
        set((state) => {
          const normalizedAgent = normalizeLocalAgent(agent);
          if (!normalizedAgent) return state;

          if (
            state.customAgents.some(
              (a) => a.identifier === normalizedAgent.identifier,
            )
          ) {
            return state;
          }

          const others = state.usedAgents.filter(
            (a) => a.identifier !== normalizedAgent.identifier,
          );
          return {
            usedAgents: normalizeLocalAgents(
              [normalizedAgent, ...others],
              MARKET_LIMITS.maxUsedAgents,
            ),
          };
        }),

      resetAgent: (identifier) =>
        set((state) => {
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          const { [identifier]: _removed, ...newOverrides } =
            state.agentOverrides;
          return { agentOverrides: newOverrides };
        }),

      // Data Management
      exportAllData: async () => createBrowserAppExportPayload(),
      clearAllData: async () => {
        await clearBrowserAppData();
        if (typeof window !== "undefined") {
          window.location.reload();
        }
      },
    }),
    {
      name: STORAGE_KEYS.SETTINGS,
      storage: createJSONStorage(getAppDbStorage),
      version: STORAGE_VERSION,
      migrate: async (persistedState) => {
        const state = stripRetiredLegacySkillSettings(persistedState)
          .value as Partial<SettingsState> & Record<string, unknown>;
        const {
          activePlugins: _activePlugins,
          installedPlugins: _installedPlugins,
          pluginConfigs: _pluginConfigs,
          marketPlugins: _marketPlugins,
          marketPluginsTimestamp: _marketPluginsTimestamp,
          customAgents: _legacyCustomAgents,
          usedAgents: _legacyUsedAgents,
          agentOverrides: _legacyAgentOverrides,
          marketAgents: _legacyMarketAgents,
          marketAgentsTimestamp: _legacyMarketAgentsTimestamp,
          marketAgentsLocale: _legacyMarketAgentsLocale,
          ...retainedState
        } = state;
        void _activePlugins;
        void _installedPlugins;
        void _pluginConfigs;
        void _marketPlugins;
        void _marketPluginsTimestamp;
        void _legacyCustomAgents;
        void _legacyUsedAgents;
        void _legacyAgentOverrides;
        void _legacyMarketAgents;
        void _legacyMarketAgentsTimestamp;
        void _legacyMarketAgentsLocale;
        const search = normalizeSearchSettings(state.search);
        const voice = await migrateVoiceLocalSecrets(state.voice);
        return {
          ...retainedState,
          // Assistant library authority moved to Backend. Legacy browser values
          // intentionally grant no installation or cross-device state.
          marketAgents: [],
          marketAgentsTimestamp: 0,
          marketAgentsLocale: "",
          system: normalizeSystemSettings(
            state.system,
            DEFAULT_SYSTEM_SETTINGS,
          ),
          modelMetadata: normalizeModelMetadataMap(state.modelMetadata),
          modelMetadataTimestamp: state.modelMetadataTimestamp || 0,
          customModelMetadata: normalizeModelMetadataMap(
            state.customModelMetadata,
          ),
          search,
          voice,
          customAgents: [],
          usedAgents: [],
          agentOverrides: {},
        } as SettingsState;
      },
      partialize: (state) => ({
        system: state.system,
        modelMetadata: state.modelMetadata,
        modelMetadataTimestamp: state.modelMetadataTimestamp,
        customModelMetadata: state.customModelMetadata,
        search: normalizeSearchSettings(state.search),
        voice: stripVoicePlainSecrets(state.voice),
      }),
      onRehydrateStorage: () => (state, error) => {
        if (typeof window === "undefined") return;
        if (error) {
          logDevError("Settings hydration failed:", error);
          state?.setHasHydrated(true);
        } else if (state) {
          state.setHasHydrated(true);
        }
      },
    },
  ),
);

// Utility Functions
export const formatModelName = (
  id: string,
  metadata?: Record<string, ModelMetadata>,
  customMetadata?: Record<string, ModelMetadata>,
): string => {
  if (!id) return "";

  // Priority: custom metadata > fetched metadata > fallback formatting
  const name = customMetadata?.[id]?.name || metadata?.[id]?.name;
  if (name) return name;

  // Fallback: format the ID
  return id
    .replace(/[-_]/g, (match, offset, str) => {
      // Keep hyphen if surrounded by digits (e.g., 06-05)
      if (
        match === "-" &&
        offset > 0 &&
        offset < str.length - 1 &&
        /\d/.test(str[offset - 1]) &&
        /\d/.test(str[offset + 1])
      ) {
        return match;
      }
      return " ";
    })
    .split(" ")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
};

export const getEffectiveBaseUrl = (baseUrl: string, type: string): string => {
  return normalizeProviderBaseUrl(baseUrl, type);
};

export const getTaskModel = (task: keyof DefaultModels): string => {
  const { defaultModels, providers } = useCoreSettingsStore.getState();
  return getDefaultModelSelectValue(defaultModels, task, providers);
};
