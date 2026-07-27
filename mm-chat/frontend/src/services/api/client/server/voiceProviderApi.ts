import { ApiClientError } from "../errors";
import type {
  AdminVoiceProviderConfigDTO,
  AdminVoiceProviderConfigsDTO,
  AdminVoiceProviderConnectionDTO,
  VoiceProviderApi,
  VoiceProviderId,
} from "../types";
import type { HttpClient } from "./httpClient";

const adminVoiceProviderPath = (providerId: VoiceProviderId) =>
  `/v1/admin/voice/providers/${encodeURIComponent(providerId)}`;

export function createServerVoiceProviderApiShell(
  httpClient: HttpClient,
): VoiceProviderApi {
  return {
    async listAdminVoiceProviderConfigs(): Promise<AdminVoiceProviderConfigsDTO> {
      return normalizeVoiceProviderConfigs(
        await httpClient.requestJson<unknown>("/v1/admin/voice/providers"),
      );
    },
    async updateAdminVoiceProviderConfig(
      providerId,
      input,
    ): Promise<AdminVoiceProviderConfigDTO> {
      const { signal, ...body } = input;
      return normalizeVoiceProviderConfig(
        await httpClient.requestJson<unknown>(
          adminVoiceProviderPath(providerId),
          { method: "PUT", body, signal },
        ),
      );
    },
    async testAdminVoiceProviderConnection(
      providerId,
      signal,
    ): Promise<AdminVoiceProviderConnectionDTO> {
      return normalizeVoiceProviderConnection(
        await httpClient.requestJson<unknown>(
          `${adminVoiceProviderPath(providerId)}/test`,
          { method: "POST", signal },
        ),
      );
    },
    async activateAdminVoiceProvider(
      providerId,
      signal,
    ): Promise<AdminVoiceProviderConnectionDTO> {
      return normalizeVoiceProviderConnection(
        await httpClient.requestJson<unknown>(
          `${adminVoiceProviderPath(providerId)}/activate`,
          { method: "POST", signal },
        ),
      );
    },
    async deleteAdminVoiceProviderConfig(providerId): Promise<void> {
      await httpClient.requestJson<void>(adminVoiceProviderPath(providerId), {
        method: "DELETE",
      });
    },
  };
}

function normalizeVoiceProviderConfigs(
  value: unknown,
): AdminVoiceProviderConfigsDTO {
  if (!isRecord(value) || !Array.isArray(value.providers)) {
    throw invalidResponse();
  }
  const providers = value.providers.map(normalizeVoiceProviderConfig);
  if (providers.length > 1) throw invalidResponse();
  const activeProviderId = value.activeProviderId;
  if (activeProviderId !== undefined && activeProviderId !== "siliconflow") {
    throw invalidResponse();
  }
  return {
    providers,
    ...(activeProviderId ? { activeProviderId } : {}),
  };
}

function normalizeVoiceProviderConfig(
  value: unknown,
): AdminVoiceProviderConfigDTO {
  if (!isRecord(value)) throw invalidResponse();
  const connectionTestedAt = value.connectionTestedAt;
  if (
    value.id !== "VOICE:SILICONFLOW" ||
    typeof value.name !== "string" ||
    !value.name.trim() ||
    value.provider !== "siliconflow" ||
    value.baseUrl !== "https://api.siliconflow.cn/v1" ||
    value.model !== "FunAudioLLM/CosyVoice2-0.5B" ||
    value.voice !== "FunAudioLLM/CosyVoice2-0.5B:claire" ||
    typeof value.enabled !== "boolean" ||
    typeof value.hasApiKey !== "boolean" ||
    typeof value.connectionTestValid !== "boolean" ||
    (connectionTestedAt !== undefined &&
      (typeof connectionTestedAt !== "string" ||
        !Number.isFinite(Date.parse(connectionTestedAt))))
  ) {
    throw invalidResponse();
  }
  return {
    id: value.id,
    name: value.name,
    provider: value.provider,
    baseUrl: value.baseUrl,
    model: value.model,
    voice: value.voice,
    enabled: value.enabled,
    hasApiKey: value.hasApiKey,
    connectionTestValid: value.connectionTestValid,
    ...(typeof connectionTestedAt === "string" ? { connectionTestedAt } : {}),
  };
}

function normalizeVoiceProviderConnection(
  value: unknown,
): AdminVoiceProviderConnectionDTO {
  if (!isRecord(value)) throw invalidResponse();
  const contentType = value.contentType;
  const size = value.size;
  if (
    typeof contentType !== "string" ||
    !contentType.toLowerCase().startsWith("audio/") ||
    typeof size !== "number" ||
    !Number.isInteger(size) ||
    size <= 0 ||
    size > 10 << 20
  ) {
    throw invalidResponse();
  }
  return {
    provider: normalizeVoiceProviderConfig(value.provider),
    contentType: contentType.toLowerCase(),
    size,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function invalidResponse(): ApiClientError {
  return new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    "Server returned invalid Voice provider metadata.",
    { recoverable: true },
  );
}
