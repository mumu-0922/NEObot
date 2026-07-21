import type {
  DefaultModels,
  ModelMetadata,
  ProviderType,
  ServerDefaultVoiceProvider,
  SystemSettings,
  VoiceSettings,
} from "../../types";

export const SERVER_DEFAULT_PROVIDER_ID = "SERVER_DEFAULT";

export type ServerDefaultProviderSource = "server-default";
export type PublicDeploymentStoreState = "memory" | "shared" | "missing";

export interface PublicServerConfig {
  modelProvider: {
    available: boolean;
    id: typeof SERVER_DEFAULT_PROVIDER_ID;
    name: string;
    type: ProviderType;
    models: string[];
    modelMetadata: Record<string, ModelMetadata>;
    defaultModels: Partial<DefaultModels>;
    defaultModelsConfigured?: boolean;
  };
  search: {
    available: boolean;
  };
  voice: {
    defaultProvider?: ServerDefaultVoiceProvider;
    elevenLabsAvailable: boolean;
    mimoAvailable: boolean;
    defaultSttAvailable: boolean;
    defaultTtsAvailable: boolean;
    sttModel?: string;
    ttsModel?: string;
    ttsVoiceId?: VoiceSettings["ttsVoiceId"];
    mimoSttModel?: string;
    mimoTtsModel?: string;
    mimoTtsVoiceId?: VoiceSettings["mimoTtsVoiceId"];
  };
  deployment?: {
    mode: "local" | "hosted";
    accessPasswordEnabled: boolean;
    trustedProxyHeaders: boolean;
    byokStableKeyConfigured: boolean;
    byokEphemeralAllowed: boolean;
    rateLimitStore: PublicDeploymentStoreState;
    pluginRegistryStore: PublicDeploymentStoreState;
  };
  system?: SystemSettings;
}
