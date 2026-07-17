import type {
  AdminProviderConfigDTO,
  AdminProviderConfigsDTO,
  ProviderApi,
  ProviderModelsInput,
  ProviderModelsResponse,
  UpdateAdminProviderConfigInput,
} from "../types";
import type { HttpClient } from "./httpClient";

export function createServerProviderApiShell(
  httpClient: HttpClient,
): ProviderApi {
  return {
    async listModels(
      input: ProviderModelsInput,
    ): Promise<ProviderModelsResponse> {
      const { signal, ...body } = input;
      return httpClient.requestJson<ProviderModelsResponse>(
        "/v1/providers/models",
        {
          method: "POST",
          body,
          signal,
        },
      );
    },
    async getServerDefaultConfig(): Promise<AdminProviderConfigDTO> {
      return httpClient.requestJson<AdminProviderConfigDTO>(
        "/v1/admin/provider-config",
      );
    },
    async updateServerDefaultConfig(
      input: UpdateAdminProviderConfigInput,
    ): Promise<AdminProviderConfigDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<AdminProviderConfigDTO>(
        "/v1/admin/provider-config",
        {
          method: "PUT",
          body,
          signal,
        },
      );
    },
    async listAdminProviderConfigs(): Promise<AdminProviderConfigsDTO> {
      return httpClient.requestJson<AdminProviderConfigsDTO>(
        "/v1/admin/providers",
      );
    },
    async updateAdminProviderConfig(
      providerId: string,
      input: UpdateAdminProviderConfigInput,
    ): Promise<AdminProviderConfigDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<AdminProviderConfigDTO>(
        `/v1/admin/providers/${encodeURIComponent(providerId)}`,
        {
          method: "PUT",
          body,
          signal,
        },
      );
    },
    async deleteAdminProviderConfig(providerId: string): Promise<void> {
      await httpClient.requestJson<void>(
        `/v1/admin/providers/${encodeURIComponent(providerId)}`,
        { method: "DELETE" },
      );
    },
  };
}
