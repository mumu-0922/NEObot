import type {
  AdminRAGProviderConfigsDTO,
  AdminRAGProviderConnectionDTO,
  ConfigureAdminRAGProviderInput,
  RAGProviderApi,
  RAGProviderId,
  RAGProviderStatusDTO,
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
    async getRAGProviderStatus(
      signal?: AbortSignal,
    ): Promise<RAGProviderStatusDTO> {
      return httpClient.requestJson<RAGProviderStatusDTO>(
        "/v1/rag/provider-status",
        { signal },
      );
    },
    async configureAdminRAGProvider(
      providerId: RAGProviderId,
      input: ConfigureAdminRAGProviderInput,
    ): Promise<AdminRAGProviderConnectionDTO> {
      const { signal, ...body } = input;
      return httpClient.requestJson<AdminRAGProviderConnectionDTO>(
        `${adminRAGProviderPath(providerId)}/configure`,
        { method: "POST", body, signal },
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
