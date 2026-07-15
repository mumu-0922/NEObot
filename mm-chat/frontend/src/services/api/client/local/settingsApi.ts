import type { SettingsApi } from "../types";
import type { PublicServerConfig } from "../../../../lib/defaultConfig/shared";
import { requestLocalJson } from "./http";

export function createLocalSettingsApiShell(): SettingsApi {
  return {
    async getRuntimeConfig() {
      return requestLocalJson<PublicServerConfig>("/api/config", {
        method: "GET",
      });
    },
  };
}
