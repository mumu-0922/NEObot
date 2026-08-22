import type { LobeAgent } from "../assistant/types";
import type { ChatConfig } from "../chat/types";
import type { ModelMetadata, ModelProvider } from "../providers/types";
import type { SearchProviderID, SearchServiceConfig } from "../search/types";
import type { VoiceSettings } from "../voice/types";
export type {
  MemoryDreamStatus,
  MemoryRecord,
  MemorySettings,
  MemorySource,
  MemoryType,
} from "../memory/types";

export interface DefaultModels {
  titleGeneration: string;
  relatedQuestions: string;
  contextCompression: string;
  promptOptimization: string;
  ragQuery: string;
  memory: string;
  recallFiltering: string;
}

export interface SystemSettings {
  systemPrompt: string;
  enableAutoTitle: boolean;
  enableRelatedQuestions: boolean;
  enableAutoCompression: boolean;
  compressionThreshold: number;
  historyKeepCount: number;
  enableCodeCollapse: boolean;
  enableHtmlVisualPrompt: boolean;
  fontSize: "small" | "medium" | "large";
}

export interface AppSettings {
  theme: "light" | "dark" | "system";
  language: "en" | "zh" | "ja" | "auto";
  system: SystemSettings;
  providers: ModelProvider[];
  modelMetadata: Record<string, ModelMetadata>;
  defaultModels: DefaultModels;
  search: {
    provider: SearchProviderID;
    resultsLimit: number;
    configs: Record<string, SearchServiceConfig>;
  };
  voice: VoiceSettings;
  customAgents: LobeAgent[];
  usedAgents: LobeAgent[];
  agentOverrides: Record<string, Partial<LobeAgent>>;
}

export type { ChatConfig };
