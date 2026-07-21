import "server-only";

import { SYSTEM_SETTINGS_LIMITS } from "../../config/limits";
import { DEFAULT_SYSTEM_SETTINGS } from "../../config/defaults";
import { normalizeSystemSettings } from "../settings/appConfig";
import type {
  MimoVoiceID,
  ServerDefaultVoiceProvider,
  SystemSettings,
  VoiceSettings,
} from "../../types";
import {
  PublicServerConfig,
  PublicDeploymentStoreState,
  SERVER_DEFAULT_PROVIDER_ID,
} from "./shared";
import { getDeploymentMode } from "../security/deployment";
import {
  DEFAULT_ELEVENLABS_TTS_MODEL,
  isElevenLabsSTTModel,
  isElevenLabsTTSModel,
} from "../utils/voiceModels";

const DEFAULT_PROVIDER_NAME = "Server Default";
const DEFAULT_ELEVENLABS_STT_MODEL = "scribe_v2";
const DEFAULT_ELEVENLABS_TTS_VOICE_ID: VoiceSettings["ttsVoiceId"] =
  "bIHbv24MWmeRgasZH58o";
const DEFAULT_MIMO_STT_MODEL = "mimo-v2.5-asr";
const DEFAULT_MIMO_TTS_MODEL = "mimo-v2.5-tts";
const DEFAULT_MIMO_TTS_VOICE_ID: MimoVoiceID = "mimo_default";
const MIMO_TTS_VOICE_IDS = new Set<MimoVoiceID>([
  "mimo_default",
  "冰糖",
  "茉莉",
  "苏打",
  "白桦",
  "Mia",
  "Chloe",
  "Milo",
  "Dean",
]);

function env(name: string): string {
  return process.env[name]?.trim() || "";
}

function envWithDefault(name: string, defaultValue: string): string {
  const value = process.env[name];
  return value === undefined ? defaultValue : value.trim();
}

function envBool(name: string): boolean | undefined {
  const value = env(name).toLowerCase();
  if (!value) return undefined;
  if (["true", "1", "yes", "on"].includes(value)) return true;
  if (["false", "0", "no", "off"].includes(value)) return false;
  return undefined;
}

function clampInteger(
  value: string,
  min: number,
  max: number,
): number | undefined {
  if (!value) return undefined;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return undefined;
  return Math.min(max, Math.max(min, Math.round(parsed)));
}

export function getDefaultProviderApiKey(): string {
  return "";
}

export function getDefaultProviderRuntimeConfig() {
  return null;
}

export function getDefaultElevenLabsApiKey(): string {
  return env("DEFAULT_ELEVENLABS_API_KEY");
}

export function getDefaultMimoApiKey(): string {
  return env("DEFAULT_MIMO_API_KEY");
}

export function getDefaultElevenLabsSttModel(): string {
  const model = envWithDefault(
    "DEFAULT_ELEVENLABS_STT_MODEL",
    DEFAULT_ELEVENLABS_STT_MODEL,
  );
  if (!model) return "";
  return isElevenLabsSTTModel(model) ? model : DEFAULT_ELEVENLABS_STT_MODEL;
}

export function getDefaultElevenLabsTtsModel(): string {
  const model = envWithDefault(
    "DEFAULT_ELEVENLABS_TTS_MODEL",
    DEFAULT_ELEVENLABS_TTS_MODEL,
  );
  if (!model) return "";
  return isElevenLabsTTSModel(model) ? model : DEFAULT_ELEVENLABS_TTS_MODEL;
}

export function getDefaultElevenLabsTtsVoiceId(): VoiceSettings["ttsVoiceId"] {
  const voiceId = env("DEFAULT_ELEVENLABS_TTS_VOICE_ID");
  return voiceId === "SAz9YHcvj6GT2YYXdXww" ||
    voiceId === "bIHbv24MWmeRgasZH58o"
    ? voiceId
    : DEFAULT_ELEVENLABS_TTS_VOICE_ID;
}

export function getDefaultMimoSttModel(): string {
  const model = envWithDefault(
    "DEFAULT_MIMO_STT_MODEL",
    DEFAULT_MIMO_STT_MODEL,
  );
  if (!model) return "";
  return model === DEFAULT_MIMO_STT_MODEL
    ? DEFAULT_MIMO_STT_MODEL
    : DEFAULT_MIMO_STT_MODEL;
}

export function getDefaultMimoTtsModel(): string {
  const model = envWithDefault(
    "DEFAULT_MIMO_TTS_MODEL",
    DEFAULT_MIMO_TTS_MODEL,
  );
  if (!model) return "";
  return model === DEFAULT_MIMO_TTS_MODEL
    ? DEFAULT_MIMO_TTS_MODEL
    : DEFAULT_MIMO_TTS_MODEL;
}

export function getDefaultMimoTtsVoiceId(): MimoVoiceID {
  const voiceId = env("DEFAULT_MIMO_TTS_VOICE_ID");
  return MIMO_TTS_VOICE_IDS.has(voiceId as MimoVoiceID)
    ? (voiceId as MimoVoiceID)
    : DEFAULT_MIMO_TTS_VOICE_ID;
}

export function getDefaultVoiceProvider():
  ServerDefaultVoiceProvider | undefined {
  const configured = env("DEFAULT_VOICE_PROVIDER").toLowerCase();
  const elevenLabsAvailable = Boolean(getDefaultElevenLabsApiKey());
  const mimoAvailable = Boolean(getDefaultMimoApiKey());

  if (configured === "mimo") return mimoAvailable ? "mimo" : undefined;
  if (configured === "elevenlabs") {
    return elevenLabsAvailable ? "elevenlabs" : undefined;
  }
  return undefined;
}

function getDefaultSystemSettings(): SystemSettings | undefined {
  const hasSystemEnv = [
    "DEFAULT_SYSTEM_PROMPT",
    "DEFAULT_ENABLE_AUTO_TITLE",
    "DEFAULT_ENABLE_RELATED_QUESTIONS",
    "DEFAULT_ENABLE_AUTO_COMPRESSION",
    "DEFAULT_COMPRESSION_THRESHOLD",
    "DEFAULT_HISTORY_KEEP_COUNT",
    "DEFAULT_ENABLE_CODE_COLLAPSE",
    "DEFAULT_ENABLE_HTML_VISUAL_PROMPT",
  ].some((name) => env(name));

  if (!hasSystemEnv) return undefined;

  return normalizeSystemSettings(
    {
      systemPrompt:
        env("DEFAULT_SYSTEM_PROMPT") || DEFAULT_SYSTEM_SETTINGS.systemPrompt,
      enableAutoTitle: envBool("DEFAULT_ENABLE_AUTO_TITLE"),
      enableRelatedQuestions: envBool("DEFAULT_ENABLE_RELATED_QUESTIONS"),
      enableAutoCompression: envBool("DEFAULT_ENABLE_AUTO_COMPRESSION"),
      compressionThreshold: clampInteger(
        env("DEFAULT_COMPRESSION_THRESHOLD"),
        SYSTEM_SETTINGS_LIMITS.minCompressionThreshold,
        SYSTEM_SETTINGS_LIMITS.maxCompressionThreshold,
      ),
      historyKeepCount: clampInteger(
        env("DEFAULT_HISTORY_KEEP_COUNT"),
        SYSTEM_SETTINGS_LIMITS.minHistoryKeepCount,
        SYSTEM_SETTINGS_LIMITS.maxHistoryKeepCount,
      ),
      enableCodeCollapse: envBool("DEFAULT_ENABLE_CODE_COLLAPSE"),
      enableHtmlVisualPrompt: envBool("DEFAULT_ENABLE_HTML_VISUAL_PROMPT"),
    },
    DEFAULT_SYSTEM_SETTINGS,
  );
}

function getPublicStoreState(
  storeEnvName: "RATE_LIMIT_STORE" | "PLUGIN_REGISTRY_STORE",
): PublicDeploymentStoreState {
  const mode = getDeploymentMode();
  const store = env(storeEnvName).toLowerCase();
  const upstashConfigured = Boolean(
    env("UPSTASH_REDIS_REST_URL") && env("UPSTASH_REDIS_REST_TOKEN"),
  );
  const wantsSharedStore =
    store === "upstash" || store === "redis" || store === "kv";

  if (wantsSharedStore && upstashConfigured) return "shared";
  if (mode === "hosted" || wantsSharedStore) return "missing";
  return "memory";
}

export function getPublicServerConfig(): PublicServerConfig {
  const defaultElevenLabsApiKey = getDefaultElevenLabsApiKey();
  const defaultMimoApiKey = getDefaultMimoApiKey();
  const defaultVoiceProvider = getDefaultVoiceProvider();
  const defaultVoiceSttModel =
    defaultVoiceProvider === "mimo"
      ? getDefaultMimoSttModel()
      : defaultVoiceProvider === "elevenlabs"
        ? getDefaultElevenLabsSttModel()
        : "";
  const defaultVoiceTtsModel =
    defaultVoiceProvider === "mimo"
      ? getDefaultMimoTtsModel()
      : defaultVoiceProvider === "elevenlabs"
        ? getDefaultElevenLabsTtsModel()
        : "";
  const defaultVoiceSttAvailable = Boolean(
    defaultVoiceProvider && defaultVoiceSttModel,
  );
  const defaultVoiceTtsAvailable = Boolean(
    defaultVoiceProvider && defaultVoiceTtsModel,
  );
  const mimoSttModel = getDefaultMimoSttModel();
  const mimoTtsModel = getDefaultMimoTtsModel();
  const system = getDefaultSystemSettings();
  const deploymentMode = getDeploymentMode();

  return {
    modelProvider: {
      available: false,
      id: SERVER_DEFAULT_PROVIDER_ID,
      name: DEFAULT_PROVIDER_NAME,
      type: "OpenAI Compatible",
      models: [],
      modelMetadata: {},
      defaultModels: {},
    },
    search: {
      available: false,
    },
    voice: {
      ...(defaultVoiceProvider
        ? { defaultProvider: defaultVoiceProvider }
        : {}),
      elevenLabsAvailable: Boolean(defaultElevenLabsApiKey),
      mimoAvailable: Boolean(defaultMimoApiKey),
      defaultSttAvailable: defaultVoiceSttAvailable,
      defaultTtsAvailable: defaultVoiceTtsAvailable,
      ...(defaultVoiceSttAvailable
        ? {
            sttModel: defaultVoiceSttModel,
          }
        : {}),
      ...(defaultVoiceTtsAvailable
        ? {
            ttsModel: defaultVoiceTtsModel,
          }
        : {}),
      ...(defaultVoiceTtsAvailable && defaultVoiceProvider === "elevenlabs"
        ? {
            ttsVoiceId: getDefaultElevenLabsTtsVoiceId(),
          }
        : {}),
      ...(defaultMimoApiKey
        ? {
            ...(mimoSttModel ? { mimoSttModel } : {}),
            ...(mimoTtsModel
              ? {
                  mimoTtsModel,
                  mimoTtsVoiceId: getDefaultMimoTtsVoiceId(),
                }
              : {}),
          }
        : {}),
    },
    deployment: {
      mode: deploymentMode,
      accessPasswordEnabled: Boolean(env("ACCESS_PASSWORD")),
      trustedProxyHeaders: envBool("TRUST_PROXY_HEADERS") === true,
      byokStableKeyConfigured: Boolean(env("BYOK_PRIVATE_KEY_PEM")),
      byokEphemeralAllowed: envBool("BYOK_ALLOW_EPHEMERAL_KEY") === true,
      rateLimitStore: getPublicStoreState("RATE_LIMIT_STORE"),
      pluginRegistryStore: getPublicStoreState("PLUGIN_REGISTRY_STORE"),
    },
    ...(system ? { system } : {}),
  };
}
