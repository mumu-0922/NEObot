import type { SettingsApi } from "../types";
import type { PublicServerConfig } from "../../../../lib/defaultConfig/shared";
import type { HttpClient } from "./httpClient";

export function createServerSettingsApiShell(
  httpClient: HttpClient,
): SettingsApi {
  return {
    async getRuntimeConfig() {
      return httpClient.requestJson<PublicServerConfig>("/v1/config");
    },
  };
}
