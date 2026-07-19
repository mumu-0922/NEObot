import type {
  AdminRAGProviderConfigDTO,
  AdminRAGProviderConfigsDTO,
  AdminRAGProviderConnectionDTO,
  RAGProviderApi,
  RAGProviderId,
  UpdateAdminRAGProviderConfigInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const adminRAGProviderPath = (providerId: RAGProviderId) =>
  `/v1/admin/rag/providers/${encodeURIComponent(providerId)}`;

export function createServerRAGProviderApiShell(
  httpClient: HttpClient,
): RAGProviderApi {
  return {
    async listAdminRAGProviderConfigs(): Promise<AdminRAGProviderConfigsDTO> {
      return httpClient.requestJson<AdminRAGProviderConfigsDTO>(
        "/v1/admin/rag/providers",
      );
    },
    async updateAdminRAGProviderConfig(
      providerId: RAGProviderId,
      input: UpdateAdminRAGProviderConfigInput,
    ): Promise<AdminRAGProviderConfigDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<AdminRAGProviderConfigDTO>(
        adminRAGProviderPath(providerId),
        { method: "PUT", body, signal },
      );
    },
    async testAdminRAGProviderConnection(
      providerId: RAGProviderId,
      signal?: AbortSignal,
    ): Promise<AdminRAGProviderConnectionDTO> {
      return httpClient.requestJson<AdminRAGProviderConnectionDTO>(
        `${adminRAGProviderPath(providerId)}/test`,
        { method: "POST", signal },
      );
    },
    async activateAdminRAGProvider(
      providerId: RAGProviderId,
      signal?: AbortSignal,
    ): Promise<AdminRAGProviderConnectionDTO> {
      return httpClient.requestJson<AdminRAGProviderConnectionDTO>(
        `${adminRAGProviderPath(providerId)}/activate`,
        { method: "POST", signal },
      );
    },
    async deleteAdminRAGProviderConfig(
      providerId: RAGProviderId,
    ): Promise<void> {
      await httpClient.requestJson<void>(adminRAGProviderPath(providerId), {
        method: "DELETE",
      });
    },
  };
}
