import type {
  AdminSearchProviderConfigDTO,
  AdminSearchProviderConfigsDTO,
  AdminSearchProviderConnectionDTO,
  SearchProviderApi,
  SearchProviderId,
  UpdateAdminSearchProviderConfigInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const adminSearchProviderPath = (providerId: SearchProviderId) =>
  `/v1/admin/search/providers/${encodeURIComponent(providerId)}`;

export function createServerSearchProviderApiShell(
  httpClient: HttpClient,
): SearchProviderApi {
  return {
    async listAdminSearchProviderConfigs(): Promise<AdminSearchProviderConfigsDTO> {
      return httpClient.requestJson<AdminSearchProviderConfigsDTO>(
        "/v1/admin/search/providers",
      );
    },
    async updateAdminSearchProviderConfig(
      providerId: SearchProviderId,
      input: UpdateAdminSearchProviderConfigInput,
    ): Promise<AdminSearchProviderConfigDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<AdminSearchProviderConfigDTO>(
        adminSearchProviderPath(providerId),
        { method: "PUT", body, signal },
      );
    },
    async testAdminSearchProviderConnection(
      providerId: SearchProviderId,
      signal?: AbortSignal,
    ): Promise<AdminSearchProviderConnectionDTO> {
      return httpClient.requestJson<AdminSearchProviderConnectionDTO>(
        `${adminSearchProviderPath(providerId)}/test`,
        { method: "POST", signal },
      );
    },
    async activateAdminSearchProvider(
      providerId: SearchProviderId,
      signal?: AbortSignal,
    ): Promise<AdminSearchProviderConnectionDTO> {
      return httpClient.requestJson<AdminSearchProviderConnectionDTO>(
        `${adminSearchProviderPath(providerId)}/activate`,
        { method: "POST", signal },
      );
    },
    async deleteAdminSearchProviderConfig(
      providerId: SearchProviderId,
    ): Promise<void> {
      await httpClient.requestJson<void>(adminSearchProviderPath(providerId), {
        method: "DELETE",
      });
    },
  };
}
