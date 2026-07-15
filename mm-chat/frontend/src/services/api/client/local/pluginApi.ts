import type {
  PluginApi,
  PluginExecuteInput,
  PluginInstallInput,
  PluginInstallResponse,
  PluginListAvailableResponse,
} from "../types";
import { postTransitionalPluginExecution } from "../pluginExecutionHttp";
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
    async execute(input: PluginExecuteInput): Promise<Response> {
      return postTransitionalPluginExecution(input);
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
