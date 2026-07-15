import type { Plugin } from "@/types";
import { ApiClientError } from "../errors";
import type {
  PluginApi,
  PluginExecuteInput,
  PluginInstallInput,
  PluginInstallResponse,
  PluginListAvailableResponse,
} from "../types";
import { postTransitionalPluginExecution } from "../pluginExecutionHttp";
import type { HttpClient } from "./httpClient";

const pluginListPath = "/v1/plugins";
const pluginInstallPath = "/v1/plugins/install";

interface ServerPluginListResponse {
  plugins?: unknown;
  unavailable?: unknown;
}

export function createServerPluginApiShell(httpClient: HttpClient): PluginApi {
  return {
    async listAvailable(input = {}): Promise<PluginListAvailableResponse> {
      try {
        const response = await httpClient.requestJson<ServerPluginListResponse>(
          pluginListPath,
          { method: "GET", signal: input.signal },
        );
        return normalizePluginListResponse(response);
      } catch (error) {
        if (isUnavailablePluginRegistryError(error)) {
          return { plugins: [], unavailable: true };
        }
        throw error;
      }
    },
    async install(input: PluginInstallInput): Promise<PluginInstallResponse> {
      try {
        const response =
          await httpClient.requestJson<ServerPluginInstallResponse>(
            pluginInstallPath,
            {
              method: "POST",
              body: toPluginInstallBody(input),
              signal: input.signal,
            },
          );
        return normalizePluginInstallResponse(response);
      } catch (error) {
        if (isUnavailablePluginRegistryError(error)) {
          throw new ApiClientError(
            "PLUGIN_INSTALL_UNAVAILABLE",
            "Plugin install is not available in server mode yet.",
            { recoverable: true },
          );
        }
        throw error;
      }
    },
    async execute(input: PluginExecuteInput): Promise<Response> {
      return postTransitionalPluginExecution(input);
    },
  };
}

function normalizePluginListResponse(
  response: ServerPluginListResponse,
): PluginListAvailableResponse {
  if (!response || !Array.isArray(response.plugins)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      "Server returned an invalid plugin registry response.",
    );
  }

  return {
    plugins: response.plugins as Plugin[],
    unavailable: response.unavailable === true || undefined,
  };
}

interface ServerPluginInstallResponse {
  plugin?: unknown;
}

function normalizePluginInstallResponse(
  response: ServerPluginInstallResponse,
): PluginInstallResponse {
  if (!response || !isRecord(response.plugin)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      "Server returned an invalid plugin install response.",
    );
  }

  return { plugin: response.plugin as unknown as Plugin };
}

function toPluginInstallBody(
  input: PluginInstallInput,
): Record<string, unknown> {
  if (input.customInput !== undefined) {
    return { customInput: input.customInput };
  }
  return { plugin: input.plugin };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function isUnavailablePluginRegistryError(error: unknown): boolean {
  return (
    error instanceof ApiClientError &&
    (error.status === 404 ||
      error.code === "NOT_FOUND" ||
      error.code === "PLUGIN_REGISTRY_UNAVAILABLE")
  );
}
