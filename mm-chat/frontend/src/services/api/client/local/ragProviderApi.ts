import { unsupportedFeature } from "../errors";
import type { RAGProviderApi } from "../types";

export function createLocalRAGProviderApiShell(): RAGProviderApi {
  return {
    async listAdminRAGProviderConfigs() {
      throw unsupportedFeature("local admin RAG provider config list");
    },
    async getRAGProviderStatus(signal) {
      void signal;
      throw unsupportedFeature("local RAG provider status");
    },
    async configureAdminRAGProvider(providerId, input) {
      void providerId;
      void input;
      throw unsupportedFeature("local atomic RAG provider configuration");
    },
    async deleteAdminRAGProviderConfig(providerId) {
      void providerId;
      throw unsupportedFeature("local admin RAG provider config delete");
    },
  };
}
