import { createLocalChatApiShell } from "./local/chat-api";
import { phase11Capabilities, resolveApiClientConfig } from "./mode";
import { createServerChatApiShell } from "./server/chat-api";
import { createHttpClient } from "./server/http-client";
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
    capabilities: { ...phase11Capabilities },
    chat,
  };
}

export * from "./errors";
export * from "./mode";
export * from "./server/http-client";
export * from "./server/sse";
export type * from "./types";
