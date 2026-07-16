import { unsupportedFeature } from "../errors";
import type { SettingsApi } from "../types";

export function createLocalSettingsApiShell(): SettingsApi {
  return {
    async getRuntimeConfig() {
      throw unsupportedFeature("local runtime config after G9.3 route removal");
    },
  };
}
