import { ApiClientError } from "../errors";
import type { SynthesizedVoiceArtifactDTO, VoiceJobApi } from "../types";
import type { HttpClient } from "./httpClient";

const maxVoiceArtifactBytes = 10 << 20;

export function createServerVoiceJobApiShell(
  httpClient: HttpClient,
): VoiceJobApi {
  return {
    async synthesizeVoice(input): Promise<SynthesizedVoiceArtifactDTO> {
      const messageId = input.messageId.trim();
      const text = input.text.trim();
      if (!messageId || !text) {
        throw new ApiClientError(
          "INVALID_VOICE_SYNTHESIS_REQUEST",
          "message id and text are required",
        );
      }
      return normalizeSynthesizedVoiceArtifact(
        await httpClient.requestJson<unknown>("/v1/voice/synthesize", {
          method: "POST",
          body: { messageId, text, provider: "default" },
          signal: input.signal,
        }),
      );
    },
  };
}

function normalizeSynthesizedVoiceArtifact(
  value: unknown,
): SynthesizedVoiceArtifactDTO {
  if (!value || typeof value !== "object") throw invalidResponse();
  const record = value as Record<string, unknown>;
  const fileId = readString(record.fileId);
  const contentType = readString(record.contentType);
  const size = record.size;
  if (
    record.purpose !== "audio" ||
    !isUuid(fileId) ||
    !contentType.startsWith("audio/") ||
    typeof size !== "number" ||
    !Number.isFinite(size) ||
    !Number.isInteger(size) ||
    size <= 0 ||
    size > maxVoiceArtifactBytes ||
    typeof record.cached !== "boolean"
  ) {
    throw invalidResponse();
  }
  return {
    fileId,
    purpose: "audio",
    contentType,
    size,
    cached: record.cached,
  };
}

function readString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function isUuid(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value,
  );
}

function invalidResponse(): ApiClientError {
  return new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    "Server returned invalid voice artifact metadata.",
    { recoverable: true },
  );
}
