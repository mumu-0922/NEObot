import { createLocalChatApiShell } from "./local/chatApi";
import { createLocalFileApiShell } from "./local/fileApi";
import { phase11Capabilities, resolveApiClientConfig } from "./mode";
import { createServerChatApiShell } from "./server/chatApi";
import { createServerFileApiShell } from "./server/fileApi";
import { createHttpClient } from "./server/httpClient";
import type { ApiClientConfig, NeoChatApiClient } from "./types";

export function createNeoChatApiClient(
  config: ApiClientConfig = {},
): NeoChatApiClient {
  const resolved = resolveApiClientConfig(config);
  const chat =
    resolved.mode === "server" && resolved.serverConfigured
      ? createServerChatApiShell(
          createHttpClient({ baseUrl: resolved.baseUrl }),
        )
      : createLocalChatApiShell();
  const files =
    resolved.mode === "server" && resolved.serverConfigured
      ? createServerFileApiShell(
          createHttpClient({ baseUrl: resolved.baseUrl }),
        )
      : createLocalFileApiShell();

  return {
    mode: resolved.mode,
    config: resolved,
    capabilities: {
      ...phase11Capabilities,
      chatCrud: resolved.mode === "server" && resolved.serverConfigured,
      chatStream: resolved.mode === "server" && resolved.serverConfigured,
      files: resolved.mode === "server" && resolved.serverConfigured,
    },
    chat,
    files,
  };
}

export * from "./errors";
export * from "./mode";
export * from "./server/httpClient";
export * from "./server/sse";
export type * from "./types";
