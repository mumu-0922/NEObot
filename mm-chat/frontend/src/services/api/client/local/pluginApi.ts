import type { PluginApi, PluginListAvailableResponse } from "../types";
import { requestLocalJson } from "./http";

export function createLocalPluginApiShell(): PluginApi {
  return {
    async listAvailable(input = {}): Promise<PluginListAvailableResponse> {
      return requestLocalJson<PluginListAvailableResponse>(
        "/api/plugins/list",
        {
          method: "GET",
          signal: input.signal,
        },
      );
    },
  };
}
