import { createLocalChatApiShell } from "./local/chatApi";
import { createLocalFileApiShell } from "./local/fileApi";
import { createLocalImportApiShell } from "./local/importApi";
import { phase11Capabilities, resolveApiClientConfig } from "./mode";
import { createServerChatApiShell } from "./server/chatApi";
import { createServerFileApiShell } from "./server/fileApi";
import { createServerImportApiShell } from "./server/importApi";
import { createHttpClient } from "./server/httpClient";
import type { ApiClientConfig, NeoChatApiClient } from "./types";

export function createNeoChatApiClient(
  config: ApiClientConfig = {},
): NeoChatApiClient {
  const resolved = resolveApiClientConfig(config);
  const serverEnabled = resolved.mode === "server" && resolved.serverConfigured;
  const serverHttpClient = serverEnabled
    ? createHttpClient({ baseUrl: resolved.baseUrl })
    : null;
  const chat = serverHttpClient
    ? createServerChatApiShell(serverHttpClient)
    : createLocalChatApiShell();
  const files = serverHttpClient
    ? createServerFileApiShell(serverHttpClient)
    : createLocalFileApiShell();
  const imports = serverHttpClient
    ? createServerImportApiShell(serverHttpClient)
    : createLocalImportApiShell();

  return {
    mode: resolved.mode,
    config: resolved,
    capabilities: {
      ...phase11Capabilities,
      chatCrud: serverEnabled,
      chatStream: serverEnabled,
      files: serverEnabled,
      imports: serverEnabled,
    },
    chat,
    files,
    imports,
  };
}

export * from "./errors";
export * from "./mode";
export * from "./server/httpClient";
export * from "./server/sse";
export type * from "./types";
