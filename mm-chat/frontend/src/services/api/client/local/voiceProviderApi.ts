import { unsupportedFeature } from "../errors";
import type { VoiceProviderApi } from "../types";

export function createLocalVoiceProviderApiShell(): VoiceProviderApi {
  return {
    async listAdminVoiceProviderConfigs() {
      throw unsupportedFeature("local admin voice provider config list");
    },
    async updateAdminVoiceProviderConfig() {
      throw unsupportedFeature("local admin voice provider config update");
    },
    async testAdminVoiceProviderConnection() {
      throw unsupportedFeature("local voice provider connection testing");
    },
    async activateAdminVoiceProvider() {
      throw unsupportedFeature("local voice provider activation");
    },
    async deleteAdminVoiceProviderConfig() {
      throw unsupportedFeature("local admin voice provider config delete");
    },
  };
}
