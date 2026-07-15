import { ApiClientError } from "../errors";
import type {
  GenerateImageInput,
  GenerateImageResponse,
  GeneratedImageArtifactDTO,
  ImageGenerationApi,
} from "../types";
import type { HttpClient } from "./httpClient";

const imageGenerationsPath = "/v1/images/generations";

export function createServerImageGenerationApiShell(
  httpClient: HttpClient,
): ImageGenerationApi {
  return {
    async generateImage(
      input: GenerateImageInput,
    ): Promise<GenerateImageResponse> {
      const response = await httpClient.requestJson<unknown>(
        imageGenerationsPath,
        {
          method: "POST",
          body: normalizeGenerateImageRequest(input),
          signal: input.signal,
        },
      );
      return normalizeGenerateImageResponse(response);
    },
  };
}

function normalizeGenerateImageRequest(
  input: GenerateImageInput,
): Record<string, unknown> {
  const providerId = input.modelRef.providerId.trim();
  const modelId = input.modelRef.modelId.trim();
  const prompt = input.prompt.trim();
  const size = input.size?.trim();
  if (!providerId || !modelId) {
    throw new ApiClientError(
      "MODEL_REF_REQUIRED",
      "Image generation requires modelRef.providerId and modelRef.modelId.",
    );
  }
  if (!prompt) {
    throw new ApiClientError("PROMPT_REQUIRED", "Image prompt is required.");
  }

  return {
    modelRef: { providerId, modelId },
    prompt,
    ...(size ? { size } : {}),
    ...(input.count !== undefined ? { count: input.count } : {}),
  };
}

function normalizeGenerateImageResponse(value: unknown): GenerateImageResponse {
  if (!value || typeof value !== "object") {
    throw invalidImageResponse();
  }
  const record = value as Record<string, unknown>;
  if (!Array.isArray(record.images)) {
    throw invalidImageResponse();
  }

  return {
    images: record.images.map(normalizeGeneratedImageArtifact),
    message: typeof record.message === "string" ? record.message : "",
  };
}

function normalizeGeneratedImageArtifact(
  value: unknown,
): GeneratedImageArtifactDTO {
  if (!value || typeof value !== "object") {
    throw invalidImageResponse();
  }
  const record = value as Record<string, unknown>;
  const fileId = readString(record, "fileId");
  const purpose = readString(record, "purpose");
  const contentType = readString(record, "contentType");
  const size = readFiniteNumber(record, "size");
  if (purpose !== "image" || !contentType.startsWith("image/") || size <= 0) {
    throw invalidImageResponse();
  }

  return { fileId, purpose, contentType, size };
}

function readString(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== "string" || !value.trim()) {
    throw invalidImageResponse();
  }
  return value.trim();
}

function readFiniteNumber(
  record: Record<string, unknown>,
  field: string,
): number {
  const value = record[field];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw invalidImageResponse();
  }
  return value;
}

function invalidImageResponse(): ApiClientError {
  return new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    "Server returned invalid image generation metadata.",
    { recoverable: true },
  );
}
