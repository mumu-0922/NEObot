import type {
  PluginApi,
  PluginInstallInput,
  PluginInstallResponse,
  PluginListAvailableResponse,
} from "../types";
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
    async install(input: PluginInstallInput): Promise<PluginInstallResponse> {
      return requestLocalJson<PluginInstallResponse>("/api/plugins/install", {
        method: "POST",
        body: toPluginInstallBody(input),
        signal: input.signal,
      });
    },
  };
}

function toPluginInstallBody(
  input: PluginInstallInput,
): Record<string, unknown> {
  if (input.customInput !== undefined) {
    return { customInput: input.customInput };
  }
  return { plugin: input.plugin };
}
