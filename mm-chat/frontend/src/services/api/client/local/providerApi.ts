import { unsupportedFeature } from "../errors";
import type {
  ProviderApi,
  ProviderModelsInput,
  ProviderModelsResponse,
} from "../types";

export function createLocalProviderApiShell(): ProviderApi {
  return {
    async listModels(
      input: ProviderModelsInput,
    ): Promise<ProviderModelsResponse> {
      void input;
      throw unsupportedFeature(
        "local provider model listing after G9.3 route removal",
      );
    },
  };
}
