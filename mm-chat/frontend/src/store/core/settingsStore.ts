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
  TextSkill,
  SkillCatalog,
  SkillDataLocale,
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
import {
  normalizeCustomSkills,
  normalizeSkillCatalog,
  normalizeTextSkill,
} from "../../lib/skills";
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

interface SettingsState {
  _hasHydrated: boolean;
  setHasHydrated: (state: boolean) => void;
  serverConfig: PublicServerConfig | null;
  applyServerConfig: (config: PublicServerConfig) => void;

  // Market Cache
  marketAgents: LobeAgent[];
  marketAgentsTimestamp: number;
  marketAgentsLocale: AgentMarketLocale | "";
  skillCatalogs: Partial<Record<SkillDataLocale, SkillCatalog>>;
  skillCatalogTimestamps: Partial<Record<SkillDataLocale, number>>;
  skillDefinitions: Record<string, TextSkill>;
  skillDefinitionTimestamps: Record<string, number>;
  setMarketAgents: (
    agents: LobeAgent[],
    locale?: AgentMarketLocale | "",
  ) => void;
  setSkillCatalog: (locale: SkillDataLocale, catalog: SkillCatalog) => void;
  setSkillDefinition: (cacheKey: string, skill: TextSkill) => void;

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

  // Skill Management
  installedSkills: TextSkill[];
  customSkills: TextSkill[];
  activeSkillIds: string[];
  skillAutoSelect: boolean;
  installSkill: (skill: TextSkill) => void;
  uninstallSkill: (skillId: string) => void;
  updateInstalledSkill: (skillId: string, skill: Partial<TextSkill>) => void;
  addCustomSkill: (skill: TextSkill) => void;
  updateCustomSkill: (skillId: string, skill: Partial<TextSkill>) => void;
  removeCustomSkill: (skillId: string) => void;
  setActiveSkillIds: (skillIds: string[]) => void;
  toggleSkillActive: (skillId: string) => void;
  setSkillAutoSelect: (enabled: boolean) => void;

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

const SKILL_ID_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

const normalizeSkillIdRefsForStorage = (
  value: unknown,
  maxCount: number = MARKET_LIMITS.maxActiveSkills,
): string[] => {
  if (!Array.isArray(value)) return [];
  const refs: string[] = [];
  const seen = new Set<string>();
  for (const item of value) {
    const id =
      typeof item === "string"
        ? item.trim().slice(0, MARKET_LIMITS.maxSkillIdChars)
        : "";
    if (!id || !SKILL_ID_RE.test(id) || seen.has(id)) continue;
    refs.push(id);
    seen.add(id);
    if (refs.length >= maxCount) break;
  }
  return refs;
};

const normalizeInstalledSkills = (
  value: unknown,
  maxCount: number = MARKET_LIMITS.maxSkills,
): TextSkill[] => {
  if (!Array.isArray(value)) return [];
  const skills: TextSkill[] = [];
  const seen = new Set<string>();

  for (const item of value) {
    const skill = normalizeTextSkill(item);
    if (!skill || seen.has(skill.id)) continue;
    skills.push({
      ...skill,
      builtIn: skill.builtIn === true || undefined,
      isCustom: skill.isCustom === true || undefined,
    });
    seen.add(skill.id);
    if (skills.length >= maxCount) break;
  }

  return skills;
};

const syncCustomSkillsFromInstalled = (skills: readonly TextSkill[]) =>
  normalizeCustomSkills(
    skills.filter((skill) => skill.isCustom && !skill.builtIn),
    MARKET_LIMITS.maxCustomSkills,
  );

const SKILL_DATA_LOCALES: readonly SkillDataLocale[] = ["en", "zh-CN"];

const normalizeSkillCatalogCache = (
  value: unknown,
): Partial<Record<SkillDataLocale, SkillCatalog>> => {
  if (!value || typeof value !== "object") return {};
  const raw = value as Partial<Record<SkillDataLocale, unknown>>;
  const result: Partial<Record<SkillDataLocale, SkillCatalog>> = {};

  for (const locale of SKILL_DATA_LOCALES) {
    const catalog = normalizeSkillCatalog(raw[locale]);
    if (catalog.skills.length > 0) {
      result[locale] = { ...catalog, locale };
    }
  }

  return result;
};

const normalizeSkillDefinitionCache = (
  value: unknown,
): Record<string, TextSkill> => {
  if (!value || typeof value !== "object") return {};
  const result: Record<string, TextSkill> = {};
  for (const [cacheKey, item] of Object.entries(value)) {
    const skill = normalizeTextSkill(item);
    if (!skill || cacheKey.length > 320) continue;
    result[cacheKey] = skill;
  }
  return result;
};

const normalizeTimestampCache = (value: unknown): Record<string, number> => {
  if (!value || typeof value !== "object") return {};
  const result: Record<string, number> = {};
  for (const [cacheKey, timestamp] of Object.entries(value)) {
    const normalizedTimestamp = Number(timestamp);
    if (
      !cacheKey ||
      cacheKey.length > 320 ||
      !Number.isFinite(normalizedTimestamp) ||
      normalizedTimestamp <= 0
    ) {
      continue;
    }
    result[cacheKey] = normalizedTimestamp;
  }
  return result;
};

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
      skillCatalogs: {},
      skillCatalogTimestamps: {},
      skillDefinitions: {},
      skillDefinitionTimestamps: {},
      setMarketAgents: (agents, locale = "") =>
        set({
          marketAgents: normalizeMarketAgents(agents),
          marketAgentsTimestamp: Date.now(),
          marketAgentsLocale: locale,
        }),
      setSkillCatalog: (locale, catalog) => {
        const normalizedCatalog = normalizeSkillCatalog(catalog);
        set((state) => ({
          skillCatalogs: {
            ...state.skillCatalogs,
            [locale]: { ...normalizedCatalog, locale },
          },
          skillCatalogTimestamps: {
            ...state.skillCatalogTimestamps,
            [locale]: Date.now(),
          },
        }));
      },
      setSkillDefinition: (cacheKey, skill) => {
        const normalizedSkill = normalizeTextSkill(skill);
        if (!normalizedSkill || !cacheKey || cacheKey.length > 320) return;
        set((state) => ({
          skillDefinitions: {
            ...state.skillDefinitions,
            [cacheKey]: normalizedSkill,
          },
          skillDefinitionTimestamps: {
            ...state.skillDefinitionTimestamps,
            [cacheKey]: Date.now(),
          },
        }));
      },

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

      // Skill Management
      installedSkills: [],
      customSkills: [],
      activeSkillIds: [],
      skillAutoSelect: true,

      installSkill: (skill) =>
        set((state) => {
          const normalizedSkill = normalizeTextSkill({
            ...skill,
            builtIn: skill.builtIn === true,
            isCustom: skill.isCustom === true || undefined,
            createdAt: skill.createdAt || new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          });
          if (!normalizedSkill) return state;

          const installedSkills = normalizeInstalledSkills([
            normalizedSkill,
            ...state.installedSkills.filter(
              (item) => item.id !== normalizedSkill.id,
            ),
          ]);

          return {
            installedSkills,
            customSkills: syncCustomSkillsFromInstalled(installedSkills),
          };
        }),

      uninstallSkill: (skillId) =>
        set((state) => {
          const normalizedId = normalizeSkillIdRefsForStorage([skillId], 1)[0];
          if (!normalizedId) return state;
          const installedSkills = state.installedSkills.filter(
            (skill) => skill.id !== normalizedId,
          );

          return {
            installedSkills,
            customSkills: syncCustomSkillsFromInstalled(installedSkills),
            activeSkillIds: state.activeSkillIds.filter(
              (id) => id !== normalizedId,
            ),
          };
        }),

      updateInstalledSkill: (skillId, skill) =>
        set((state) => {
          const normalizedId = normalizeSkillIdRefsForStorage([skillId], 1)[0];
          if (!normalizedId) return state;
          let changed = false;
          const installedSkills = state.installedSkills.map((current) => {
            if (current.id !== normalizedId) return current;
            const normalizedSkill = normalizeTextSkill({
              ...current,
              ...skill,
              id: current.id,
              name: skill.name || current.name,
              activation: { ...current.activation, ...skill.activation },
              risk: { ...current.risk, ...skill.risk },
              builtIn: current.builtIn === true,
              isCustom: true,
              updatedAt: new Date().toISOString(),
            });
            if (!normalizedSkill) return current;
            changed = true;
            return {
              ...normalizedSkill,
              builtIn: current.builtIn === true || undefined,
              isCustom: true,
            };
          });
          if (!changed) return state;

          const normalizedInstalledSkills =
            normalizeInstalledSkills(installedSkills);
          return {
            installedSkills: normalizedInstalledSkills,
            customSkills: syncCustomSkillsFromInstalled(
              normalizedInstalledSkills,
            ),
          };
        }),

      addCustomSkill: (skill) =>
        set((state) => {
          const normalizedSkill = normalizeTextSkill({
            ...skill,
            builtIn: false,
            isCustom: true,
            createdAt: skill.createdAt || new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          });
          if (!normalizedSkill) return state;

          const installedSkills = normalizeInstalledSkills([
            { ...normalizedSkill, builtIn: false, isCustom: true },
            ...state.installedSkills.filter(
              (item) => item.id !== normalizedSkill.id,
            ),
          ]);

          return {
            installedSkills,
            customSkills: normalizeCustomSkills(
              [
                { ...normalizedSkill, builtIn: false, isCustom: true },
                ...state.customSkills.filter(
                  (item) => item.id !== normalizedSkill.id,
                ),
              ],
              MARKET_LIMITS.maxCustomSkills,
            ),
          };
        }),

      updateCustomSkill: (skillId, skill) =>
        set((state) => {
          let changed = false;
          const installedSkills = state.installedSkills.map((current) => {
            if (current.id !== skillId || current.builtIn) return current;
            const normalizedSkill = normalizeTextSkill({
              ...current,
              ...skill,
              id: current.id,
              name: skill.name || current.name,
              activation: { ...current.activation, ...skill.activation },
              risk: { ...current.risk, ...skill.risk },
              builtIn: false,
              isCustom: true,
              updatedAt: new Date().toISOString(),
            });
            if (!normalizedSkill) return current;
            changed = true;
            return { ...normalizedSkill, builtIn: false, isCustom: true };
          });
          const customSkills = state.customSkills.map((current) => {
            if (current.id !== skillId) return current;
            const normalizedSkill = normalizeTextSkill({
              ...current,
              ...skill,
              id: current.id,
              name: skill.name || current.name,
              activation: { ...current.activation, ...skill.activation },
              risk: { ...current.risk, ...skill.risk },
              builtIn: false,
              isCustom: true,
              updatedAt: new Date().toISOString(),
            });
            if (!normalizedSkill) return current;
            changed = true;
            return { ...normalizedSkill, builtIn: false, isCustom: true };
          });
          if (!changed) return state;

          const normalizedInstalledSkills =
            normalizeInstalledSkills(installedSkills);
          return {
            installedSkills: normalizedInstalledSkills,
            customSkills: normalizeCustomSkills(
              customSkills,
              MARKET_LIMITS.maxCustomSkills,
            ),
          };
        }),

      removeCustomSkill: (skillId) =>
        set((state) => {
          const installedSkills = state.installedSkills.filter(
            (skill) => skill.id !== skillId || skill.builtIn,
          );
          return {
            installedSkills,
            customSkills: state.customSkills.filter(
              (skill) => skill.id !== skillId,
            ),
            activeSkillIds: state.activeSkillIds.filter((id) => id !== skillId),
          };
        }),

      setActiveSkillIds: (skillIds) =>
        set({
          activeSkillIds: normalizeSkillIdRefsForStorage(skillIds),
        }),

      toggleSkillActive: (skillId) =>
        set((state) => {
          const normalizedId = normalizeSkillIdRefsForStorage([skillId], 1)[0];
          if (!normalizedId) return state;
          const isActive = state.activeSkillIds.includes(normalizedId);
          return {
            activeSkillIds: normalizeSkillIdRefsForStorage(
              isActive
                ? state.activeSkillIds.filter((id) => id !== normalizedId)
                : [...state.activeSkillIds, normalizedId],
            ),
          };
        }),

      setSkillAutoSelect: (enabled) => set({ skillAutoSelect: enabled }),

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
        const state = persistedState as Partial<SettingsState> &
          Record<string, unknown>;
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
          skillCatalogs: normalizeSkillCatalogCache(state.skillCatalogs),
          skillCatalogTimestamps: normalizeTimestampCache(
            state.skillCatalogTimestamps,
          ),
          skillDefinitions: normalizeSkillDefinitionCache(
            state.skillDefinitions,
          ),
          skillDefinitionTimestamps: normalizeTimestampCache(
            state.skillDefinitionTimestamps,
          ),
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
          installedSkills: normalizeInstalledSkills(
            state.installedSkills && state.installedSkills.length > 0
              ? state.installedSkills
              : state.customSkills,
          ),
          customSkills: normalizeCustomSkills(
            state.customSkills,
            MARKET_LIMITS.maxCustomSkills,
          ),
          activeSkillIds: normalizeSkillIdRefsForStorage(state.activeSkillIds),
          skillAutoSelect:
            typeof state.skillAutoSelect === "boolean"
              ? state.skillAutoSelect
              : true,
          customAgents: [],
          usedAgents: [],
          agentOverrides: {},
        } as SettingsState;
      },
      partialize: (state) => ({
        skillCatalogs: state.skillCatalogs,
        skillCatalogTimestamps: state.skillCatalogTimestamps,
        skillDefinitions: state.skillDefinitions,
        skillDefinitionTimestamps: state.skillDefinitionTimestamps,
        system: state.system,
        modelMetadata: state.modelMetadata,
        modelMetadataTimestamp: state.modelMetadataTimestamp,
        customModelMetadata: state.customModelMetadata,
        search: normalizeSearchSettings(state.search),
        voice: stripVoicePlainSecrets(state.voice),
        installedSkills: state.installedSkills,
        customSkills: state.customSkills,
        activeSkillIds: state.activeSkillIds,
        skillAutoSelect: state.skillAutoSelect,
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
