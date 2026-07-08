import { createLocalChatApiShell } from "./local/chatApi";
import { phase11Capabilities, resolveApiClientConfig } from "./mode";
import { createServerChatApiShell } from "./server/chatApi";
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

  return {
    mode: resolved.mode,
    config: resolved,
    capabilities: {
      ...phase11Capabilities,
      chatCrud: resolved.mode === "server" && resolved.serverConfigured,
    },
    chat,
  };
}

export * from "./errors";
export * from "./mode";
export * from "./server/httpClient";
export * from "./server/sse";
export type * from "./types";
