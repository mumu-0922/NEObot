import { ModelInfo } from "@/services/api/chatService";

const IMAGE_GENERATION_MODEL_PREFIXES = [
  "gpt-image-",
  "dall-e-",
  "imagen-",
] as const;

const IMAGE_ANALYSIS_INTENT_PATTERNS = [
  /(?:不要|不用|无需|别|禁止|停止).{0,6}(?:生成|创建|创作|制作|设计|画|绘制|生图)/u,
  /(?:分析|识别|描述|解释|总结|读取|提取).{0,8}(?:这|该|上面|以下)?(?:张|幅|个)?(?:图片|图像|照片|截图)/u,
  /(?:这|该|上面|以下)(?:张|幅|个)?(?:图片|图像|照片|截图).{0,8}(?:是|有|包含|写了|显示)/u,
  /(?:图片|图像|照片|截图).{0,8}(?:的)?(?:描述|分析|解释|识别|内容|文字)/u,
  /(?:analy[sz]e|describe|explain|identify|recognize|read|extract).{0,24}(?:image|picture|photo|screenshot)/i,
  /(?:do\s+not|don't|dont|never|stop)\s+(?:generate|create|make|design|draw|paint)/i,
] as const;

const IMAGE_GENERATION_INTENT_PATTERNS = [
  /(?:生成|创建|创作|制作|设计|做)(?:一|两|三|几|多)?(?:张|幅|个|套)?[^，。！？\n]{0,20}(?:图片|图像|照片|海报|插画|头像|壁纸|封面|配图)/u,
  /(?:生|出)(?:一|两|三|几|多)?(?:张|幅|个)?图/u,
  /(?:^|[，。！？\s])(?:请|帮我|给我|为我)?(?:生|出)(?:一|两|三|几|多)?(?:张|幅|个|套)[^，。！？\n]{0,20}(?:图片|图像|照片|海报|插画|头像|壁纸|封面|配图|图(?=$|[，。！？\s]))/u,
  /(?:帮我|请|给我|为我)?(?:画|绘制|描绘)(?:一|两|三|几|个|只|幅|张|些)/u,
  /(?:generate|create|make|design)\s+(?:an?\s+|some\s+)?(?:image|picture|photo|poster|illustration|avatar|wallpaper|cover)/i,
  /(?:draw|paint|illustrate)\s+(?:me\s+)?(?:an?\s+|some\s+)?\S+/i,
] as const;

export interface ImageGenerationRouteOptions {
  selectedModel: string;
  availableModels: ModelInfo[];
  prompt: string;
  hasAttachments?: boolean;
}

/**
 * Keep this capability check aligned with the Go chat handler. The backend
 * routes these model families through the image job executor instead of chat.
 */
export const isImageGenerationModel = (model: string): boolean => {
  const separatorIndex = model.indexOf(":");
  const modelId = separatorIndex >= 0 ? model.slice(separatorIndex + 1) : model;
  const normalizedModelId = modelId.trim().toLowerCase();
  return IMAGE_GENERATION_MODEL_PREFIXES.some((prefix) =>
    normalizedModelId.startsWith(prefix),
  );
};

export const hasExplicitImageGenerationIntent = (prompt: string): boolean => {
  const normalizedPrompt = prompt.trim();
  if (!normalizedPrompt) return false;
  if (
    IMAGE_ANALYSIS_INTENT_PATTERNS.some((pattern) =>
      pattern.test(normalizedPrompt),
    )
  ) {
    return false;
  }
  return IMAGE_GENERATION_INTENT_PATTERNS.some((pattern) =>
    pattern.test(normalizedPrompt),
  );
};

/**
 * Select an enabled image model for an explicit text-to-image request without
 * changing the user's selected chat model. Existing attachments disable the
 * automatic route because the current image executor is prompt-only.
 */
export const resolveImageGenerationRoute = ({
  selectedModel,
  availableModels,
  prompt,
  hasAttachments = false,
}: ImageGenerationRouteOptions): string => {
  if (
    isImageGenerationModel(selectedModel) ||
    hasAttachments ||
    !hasExplicitImageGenerationIntent(prompt)
  ) {
    return selectedModel;
  }

  const selectedProviderId = selectedModel.split(":", 1)[0];
  const imageModels = availableModels.filter((model) =>
    isImageGenerationModel(model.name),
  );
  const sameProviderImageModel = imageModels.find(
    (model) => model.name.split(":", 1)[0] === selectedProviderId,
  );
  return sameProviderImageModel?.name || imageModels[0]?.name || selectedModel;
};

/**
 * Default Gemini models configuration
 */
export const DEFAULT_GEMINI_MODELS: ModelInfo[] = [
  {
    name: "GEMINI:gemini-flash-latest",
    displayName: "Gemini Flash Latest",
    description: "Always Latest Flash",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-3-flash-preview",
    displayName: "Gemini 3 Flash Preview",
    description: "Latest Flash Preview",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-flash",
    displayName: "Gemini 2.5 Flash",
    description: "Fast & Versatile",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-flash-lite",
    displayName: "Gemini 2.5 Flash Lite",
    description: "Lightweight & Fast",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-pro",
    displayName: "Gemini 2.5 Pro",
    description: "Complex Reasoning",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-flash-image",
    displayName: "Gemini 2.5 Flash Image",
    description: "Image Generation",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-flash-native-audio-preview-12-2025",
    displayName: "Gemini 2.5 Flash Native Audio",
    description: "Native Audio",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-flash-preview-tts",
    displayName: "Gemini 2.5 Flash TTS",
    description: "Text to Speech",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:gemini-2.5-pro-preview-tts",
    displayName: "Gemini 2.5 Pro TTS",
    description: "Pro Text to Speech",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:imagen-4.0-generate-001",
    displayName: "Imagen 4.0",
    description: "Image Generation",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:imagen-4.0-ultra-generate-001",
    displayName: "Imagen 4.0 Ultra",
    description: "Ultra Image Generation",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:imagen-4.0-fast-generate-001",
    displayName: "Imagen 4.0 Fast",
    description: "Fast Image Generation",
    providerName: "Google Gemini",
  },
  {
    name: "GEMINI:imagen-3.0-generate-002",
    displayName: "Imagen 3.0",
    description: "Image Generation",
    providerName: "Google Gemini",
  },
];

/**
 * Build available models list from providers
 */
export const buildAvailableModels = (
  providers: any[],
  modelMetadata: Record<string, any>,
  customModelMetadata: Record<string, any>,
  formatModelName: (
    modelId: string,
    modelMetadata: Record<string, any>,
    customModelMetadata: Record<string, any>,
  ) => string | null,
): ModelInfo[] => {
  const allModels: ModelInfo[] = [];

  providers.forEach((p) => {
    if (p.enabled && p.models && p.models.length > 0) {
      p.models.forEach((modelId: string) => {
        const displayName =
          formatModelName(modelId, modelMetadata, customModelMetadata) ||
          modelId;

        allModels.push({
          name: `${p.id}:${modelId}`,
          displayName: displayName,
          description: `Model from ${p.name}`,
          providerName: p.name,
        });
      });
    }
  });

  return allModels;
};

export const resolveSelectedModel = (
  availableModels: ModelInfo[],
  selectedModel: string,
  preferredProviderId: string,
): string => {
  if (availableModels.length === 0) return "";

  if (
    selectedModel &&
    availableModels.some((model) => model.name === selectedModel)
  ) {
    return selectedModel;
  }

  const preferredModel = preferredProviderId
    ? availableModels.find((model) =>
        model.name.startsWith(`${preferredProviderId}:`),
      )
    : undefined;

  return preferredModel?.name || availableModels[0].name;
};
