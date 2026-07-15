import type { Plugin } from "@/types";
import { ApiClientError } from "../errors";
import type { PluginApi, PluginListAvailableResponse } from "../types";
import type { HttpClient } from "./httpClient";

const pluginListPath = "/v1/plugins";

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

function isUnavailablePluginRegistryError(error: unknown): boolean {
  return (
    error instanceof ApiClientError &&
    (error.status === 404 ||
      error.code === "NOT_FOUND" ||
      error.code === "PLUGIN_REGISTRY_UNAVAILABLE")
  );
}
