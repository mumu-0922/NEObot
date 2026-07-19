import { unsupportedFeature } from "../errors";
import type { SearchProviderApi } from "../types";

export function createLocalSearchProviderApiShell(): SearchProviderApi {
  return {
    async listAdminSearchProviderConfigs() {
      throw unsupportedFeature("local admin search provider config list");
    },
    async updateAdminSearchProviderConfig(providerId, input) {
      void providerId;
      void input;
      throw unsupportedFeature("local admin search provider config update");
    },
    async testAdminSearchProviderConnection(providerId, signal) {
      void providerId;
      void signal;
      throw unsupportedFeature("local search provider connection testing");
    },
    async activateAdminSearchProvider(providerId, signal) {
      void providerId;
      void signal;
      throw unsupportedFeature("local search provider activation");
    },
    async deleteAdminSearchProviderConfig(providerId) {
      void providerId;
      throw unsupportedFeature("local admin search provider config delete");
    },
  };
}
