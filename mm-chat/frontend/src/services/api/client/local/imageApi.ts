import { unsupportedFeature } from "../errors";
import type { GenerateImageResponse, ImageGenerationApi } from "../types";

export function createLocalImageGenerationApiShell(): ImageGenerationApi {
  return {
    async generateImage(): Promise<GenerateImageResponse> {
      throw unsupportedFeature("local image generation adapter wiring");
    },
  };
}
