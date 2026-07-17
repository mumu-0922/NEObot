import { describe, expect, it } from "vitest";
import { parseModelString } from "../lib/utils/model";
import {
  hasExplicitImageGenerationIntent,
  isImageGenerationModel,
  resolveImageGenerationRoute,
  resolveSelectedModel,
} from "../lib/utils/models";
import { SERVER_DEFAULT_PROVIDER_ID } from "../lib/defaultConfig/shared";
import type { ModelInfo } from "../services/api/chatService";

const availableModels: ModelInfo[] = [
  {
    name: "server:model-default",
    displayName: "Server Default",
    description: "Default server model",
    providerName: "Server",
  },
  {
    name: "custom:model-a",
    displayName: "Custom A",
    description: "Custom model",
    providerName: "Custom",
  },
  {
    name: "custom:model-b",
    displayName: "Custom B",
    description: "Custom model",
    providerName: "Custom",
  },
];

describe("model string utilities", () => {
  it("preserves model ids that contain colons", () => {
    expect(parseModelString("provider_1:vendor:model:latest")).toEqual({
      providerId: "provider_1",
      modelName: "vendor:model:latest",
    });
  });

  it("falls back to the full string when no valid provider prefix exists", () => {
    expect(parseModelString("gemini-2.5-flash")).toEqual({
      modelName: "gemini-2.5-flash",
    });
    expect(parseModelString(":missing-provider")).toEqual({
      modelName: ":missing-provider",
    });
    expect(parseModelString("provider-only:")).toEqual({
      modelName: "provider-only:",
    });
  });
});

describe("selected model resolution", () => {
  it("keeps the current model when it is still available", () => {
    expect(
      resolveSelectedModel(availableModels, "custom:model-b", "server"),
    ).toBe("custom:model-b");
  });

  it("uses the preferred provider model when the current model is empty", () => {
    expect(resolveSelectedModel(availableModels, "", "server")).toBe(
      "server:model-default",
    );
  });

  it("uses the preferred provider model when the current model is unavailable", () => {
    expect(
      resolveSelectedModel(availableModels, "missing:model", "server"),
    ).toBe("server:model-default");
  });

  it("prefers the server default provider after its models are available", () => {
    const models: ModelInfo[] = [
      {
        name: "custom:model-a",
        displayName: "Custom A",
        description: "Custom model",
        providerName: "Custom",
      },
      {
        name: `${SERVER_DEFAULT_PROVIDER_ID}:model-default`,
        displayName: "Server Default",
        description: "Default server model",
        providerName: "Server",
      },
    ];

    expect(resolveSelectedModel(models, "", SERVER_DEFAULT_PROVIDER_ID)).toBe(
      `${SERVER_DEFAULT_PROVIDER_ID}:model-default`,
    );
  });

  it("falls back to the first model when no preferred provider model exists", () => {
    expect(resolveSelectedModel(availableModels, "", "missing")).toBe(
      "server:model-default",
    );
  });

  it("returns an empty string when no model is available", () => {
    expect(resolveSelectedModel([], "missing:model", "server")).toBe("");
  });
});

describe("image generation routing", () => {
  const imageModels: ModelInfo[] = [
    ...availableModels,
    {
      name: "custom:gpt-image-2",
      displayName: "GPT Image 2",
      description: "Image generation",
      providerName: "Custom",
    },
    {
      name: "server:imagen-4.0-generate-001",
      displayName: "Imagen 4",
      description: "Image generation",
      providerName: "Server",
    },
  ];

  it("recognizes only backend-supported image model families", () => {
    expect(isImageGenerationModel("custom:gpt-image-2")).toBe(true);
    expect(isImageGenerationModel("server:dall-e-3")).toBe(true);
    expect(isImageGenerationModel("GEMINI:imagen-4.0-generate-001")).toBe(true);
    expect(isImageGenerationModel("custom:gpt-4.1")).toBe(false);
    expect(isImageGenerationModel("GEMINI:gemini-2.5-flash-image")).toBe(false);
  });

  it("detects explicit Chinese and English generation requests", () => {
    expect(hasExplicitImageGenerationIntent("帮我生成一张赛博朋克海报")).toBe(
      true,
    );
    expect(hasExplicitImageGenerationIntent("画一只戴墨镜的猫")).toBe(true);
    expect(
      hasExplicitImageGenerationIntent("Create an illustration of Mars"),
    ).toBe(true);
  });

  it("does not confuse image understanding with image generation", () => {
    expect(hasExplicitImageGenerationIntent("分析这张图片的内容")).toBe(false);
    expect(hasExplicitImageGenerationIntent("描述图片里有什么")).toBe(false);
    expect(hasExplicitImageGenerationIntent("不要生成图片，只回答文字")).toBe(
      false,
    );
    expect(hasExplicitImageGenerationIntent("解释 image generation API")).toBe(
      false,
    );
  });

  it("prefers an image model from the currently selected provider", () => {
    expect(
      resolveImageGenerationRoute({
        selectedModel: "custom:model-a",
        availableModels: imageModels,
        prompt: "生成一张图片",
      }),
    ).toBe("custom:gpt-image-2");
  });

  it("falls back to another enabled provider image model", () => {
    expect(
      resolveImageGenerationRoute({
        selectedModel: "missing:model-a",
        availableModels: imageModels,
        prompt: "make a poster for the launch",
      }),
    ).toBe("custom:gpt-image-2");
  });

  it("keeps the selected model for attachments, non-image intent, or no image model", () => {
    expect(
      resolveImageGenerationRoute({
        selectedModel: "custom:model-a",
        availableModels: imageModels,
        prompt: "生成一张图片",
        hasAttachments: true,
      }),
    ).toBe("custom:model-a");
    expect(
      resolveImageGenerationRoute({
        selectedModel: "custom:model-a",
        availableModels: imageModels,
        prompt: "讲讲图像生成的工作原理",
      }),
    ).toBe("custom:model-a");
    expect(
      resolveImageGenerationRoute({
        selectedModel: "custom:model-a",
        availableModels,
        prompt: "生成一张图片",
      }),
    ).toBe("custom:model-a");
  });

  it("keeps an explicitly selected image model", () => {
    expect(
      resolveImageGenerationRoute({
        selectedModel: "custom:gpt-image-2",
        availableModels: imageModels,
        prompt: "a fox in the snow",
      }),
    ).toBe("custom:gpt-image-2");
  });
});
