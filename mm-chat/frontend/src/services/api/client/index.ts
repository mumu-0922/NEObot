import { createLocalAgentApiShell } from "./local/agentApi";
import { createLocalAuthApiShell } from "./local/authApi";
import { createLocalByokApiShell } from "./local/byokApi";
import { createLocalChatApiShell } from "./local/chatApi";
import { createLocalFileApiShell } from "./local/fileApi";
import { createLocalImageGenerationApiShell } from "./local/imageApi";
import { createLocalImportApiShell } from "./local/importApi";
import { createLocalKnowledgeApiShell } from "./local/knowledgeApi";
import { createLocalPluginApiShell } from "./local/pluginApi";
import { createLocalProviderApiShell } from "./local/providerApi";
import { createLocalSettingsApiShell } from "./local/settingsApi";
import { createLocalTeamApiShell } from "./local/teamApi";
import { phase11Capabilities, resolveApiClientConfig } from "./mode";
import { createServerAgentApiShell } from "./server/agentApi";
import { createServerAuthApiShell } from "./server/authApi";
import { createServerByokApiShell } from "./server/byokApi";
import { createServerChatApiShell } from "./server/chatApi";
import { createServerFileApiShell } from "./server/fileApi";
import { createServerImageGenerationApiShell } from "./server/imageApi";
import { createServerImportApiShell } from "./server/importApi";
import { createServerKnowledgeApiShell } from "./server/knowledgeApi";
import { createServerPluginApiShell } from "./server/pluginApi";
import { createServerProviderApiShell } from "./server/providerApi";
import { createServerSettingsApiShell } from "./server/settingsApi";
import { createServerTeamApiShell } from "./server/teamApi";
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
  const images = serverHttpClient
    ? createServerImageGenerationApiShell(serverHttpClient)
    : createLocalImageGenerationApiShell();
  const imports = serverHttpClient
    ? createServerImportApiShell(serverHttpClient)
    : createLocalImportApiShell();
  const agents = serverHttpClient
    ? createServerAgentApiShell(serverHttpClient)
    : createLocalAgentApiShell();
  const auth = serverHttpClient
    ? createServerAuthApiShell(serverHttpClient)
    : createLocalAuthApiShell();
  const settings = serverHttpClient
    ? createServerSettingsApiShell(serverHttpClient)
    : createLocalSettingsApiShell();
  const providers = serverHttpClient
    ? createServerProviderApiShell(serverHttpClient)
    : createLocalProviderApiShell();
  const byok = serverHttpClient
    ? createServerByokApiShell(serverHttpClient)
    : createLocalByokApiShell();
  const plugins = serverHttpClient
    ? createServerPluginApiShell(serverHttpClient)
    : createLocalPluginApiShell();
  const teams = serverHttpClient
    ? createServerTeamApiShell(serverHttpClient)
    : createLocalTeamApiShell();
  const knowledge = serverHttpClient
    ? createServerKnowledgeApiShell(serverHttpClient)
    : createLocalKnowledgeApiShell();

  return {
    mode: resolved.mode,
    config: resolved,
    capabilities: {
      ...phase11Capabilities,
      chatCrud: serverEnabled,
      chatStream: serverEnabled,
      files: serverEnabled,
      auth: serverEnabled,
      imports: serverEnabled,
      plugins: serverEnabled,
      teams: serverEnabled,
      knowledge: serverEnabled,
      imageGeneration: serverEnabled,
    },
    auth,
    settings,
    providers,
    byok,
    images,
    chat,
    files,
    plugins,
    imports,
    agents,
    teams,
    knowledge,
  };
}

export * from "./errors";
export * from "./mode";
export * from "./server/httpClient";
export * from "./server/sse";
export type * from "./types";
