import type {
  ProviderApi,
  ProviderModelsInput,
  ProviderModelsResponse,
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
  };
}
