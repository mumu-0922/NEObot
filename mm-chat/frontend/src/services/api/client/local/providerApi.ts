import type {
  ProviderApi,
  ProviderModelsInput,
  ProviderModelsResponse,
} from "../types";
import { requestLocalJson } from "./http";

export function createLocalProviderApiShell(): ProviderApi {
  return {
    async listModels(
      input: ProviderModelsInput,
    ): Promise<ProviderModelsResponse> {
      return requestLocalJson<ProviderModelsResponse>("/api/providers/models", {
        method: "POST",
        body: input,
        signal: input.signal,
      });
    },
  };
}
