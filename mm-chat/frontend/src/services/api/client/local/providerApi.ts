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
    async getServerDefaultConfig() {
      throw unsupportedFeature("local server-default provider config");
    },
    async updateServerDefaultConfig(input) {
      void input;
      throw unsupportedFeature("local server-default provider config update");
    },
    async listAdminProviderConfigs() {
      throw unsupportedFeature("local admin provider config list");
    },
    async updateAdminProviderConfig(providerId, input) {
      void providerId;
      void input;
      throw unsupportedFeature("local admin provider config update");
    },
    async deleteAdminProviderConfig(providerId) {
      void providerId;
      throw unsupportedFeature("local admin provider config delete");
    },
  };
}
