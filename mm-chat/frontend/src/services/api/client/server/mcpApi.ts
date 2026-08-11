import {
  normalizeMcpCalls,
  normalizeMcpConversationSelectionEnvelope,
  normalizeMcpServerEnvelope,
  normalizeMcpServers,
  normalizeMcpWorkspaceSelectionEnvelope,
} from "@/lib/mcp/types";
import { ApiClientError } from "../errors";
import type {
  McpApi,
  McpCreatePrivateServerInput,
  McpListCallsInput,
  McpListServersInput,
  McpOAuthStartInput,
  McpOAuthStartResult,
  McpReplaceConversationSelectionInput,
  McpReplaceWorkspaceSelectionInput,
  McpSetCredentialInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const MCP_BASE_PATH = "/v1/mcp";

export function createServerMcpApiShell(httpClient: HttpClient): McpApi {
  return {
    async listServers(input: McpListServersInput = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${MCP_BASE_PATH}/servers${query({ conversationId: input.conversationId })}`,
        { signal: input.signal },
      );
      return requireNormalized(normalizeMcpServers(response), "server list");
    },

    async createPrivateServer(input: McpCreatePrivateServerInput) {
      const response = await httpClient.requestJson<unknown>(
        `${MCP_BASE_PATH}/servers`,
        {
          method: "POST",
          body: {
            name: input.name,
            endpointUrl: input.endpointUrl,
            authType: input.authType,
            ...(input.headerName ? { headerName: input.headerName } : {}),
            ...(input.clientId ? { clientId: input.clientId } : {}),
            ...(input.scopes ? { scopes: input.scopes } : {}),
          },
          signal: input.signal,
        },
      );
      return requireNormalized(normalizeMcpServerEnvelope(response), "server");
    },

    async deletePrivateServer(serverId, options = {}) {
      await httpClient.requestJson<void>(
        `${MCP_BASE_PATH}/servers/private/${encodeURIComponent(serverId)}`,
        { method: "DELETE", signal: options.signal },
      );
    },

    async validatePrivateServer(serverId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${MCP_BASE_PATH}/servers/private/${encodeURIComponent(serverId)}/validate`,
        { method: "POST", body: {}, signal: options.signal },
      );
      return requireNormalized(normalizeMcpServerEnvelope(response), "server");
    },

    async setCredential(input: McpSetCredentialInput) {
      const response = await httpClient.requestJson<unknown>(
        `${serverPath(input.serverRef)}/credential`,
        {
          method: "PUT",
          body: {
            value: input.value,
            ...(input.conversationId
              ? { conversationId: input.conversationId }
              : {}),
          },
          signal: input.signal,
        },
      );
      return requireNormalized(normalizeMcpServerEnvelope(response), "server");
    },

    async deleteCredential(input) {
      await httpClient.requestJson<void>(
        `${serverPath(input.serverRef)}/credential${query({ conversationId: input.conversationId })}`,
        { method: "DELETE", signal: input.signal },
      );
    },

    async getConversationSelection(conversationId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${conversationPath(conversationId)}/selection`,
        { signal: options.signal },
      );
      return requireNormalized(
        normalizeMcpConversationSelectionEnvelope(response),
        "conversation selection",
      );
    },

    async replaceConversationSelection(
      input: McpReplaceConversationSelectionInput,
    ) {
      const response = await httpClient.requestJson<unknown>(
        `${conversationPath(input.conversationId)}/selection`,
        {
          method: "PUT",
          body: {
            mode: input.mode,
            revision: input.revision,
            servers: input.servers,
          },
          signal: input.signal,
        },
      );
      return requireNormalized(
        normalizeMcpConversationSelectionEnvelope(response),
        "conversation selection",
      );
    },

    async getWorkspaceSelection(workspaceId, options = {}) {
      const response = await httpClient.requestJson<unknown>(
        `${workspacePath(workspaceId)}/selection`,
        { signal: options.signal },
      );
      return requireNormalized(
        normalizeMcpWorkspaceSelectionEnvelope(response),
        "workspace selection",
      );
    },

    async replaceWorkspaceSelection(input: McpReplaceWorkspaceSelectionInput) {
      const response = await httpClient.requestJson<unknown>(
        `${workspacePath(input.workspaceId)}/selection`,
        {
          method: "PUT",
          body: { revision: input.revision, servers: input.servers },
          signal: input.signal,
        },
      );
      return requireNormalized(
        normalizeMcpWorkspaceSelectionEnvelope(response),
        "workspace selection",
      );
    },

    async startOAuth(input: McpOAuthStartInput) {
      const response = await httpClient.requestJson<unknown>(
        `${MCP_BASE_PATH}/oauth/start`,
        {
          method: "POST",
          body: {
            serverRef: input.serverRef,
            ...(input.conversationId
              ? { conversationId: input.conversationId }
              : {}),
            returnUrl: input.returnUrl,
          },
          signal: input.signal,
        },
      );
      return normalizeOAuthStart(response);
    },

    async revokeOAuth(serverRef, options = {}) {
      await httpClient.requestJson<void>(`${MCP_BASE_PATH}/oauth/revoke`, {
        method: "POST",
        body: { serverRef },
        signal: options.signal,
      });
    },

    async listCalls(input: McpListCallsInput) {
      const response = await httpClient.requestJson<unknown>(
        `${conversationPath(input.conversationId)}/calls${query({ runId: input.runId })}`,
        { signal: input.signal },
      );
      return requireNormalized(normalizeMcpCalls(response), "call timeline");
    },
  };
}

function serverPath(ref: { source: string; id: string }): string {
  return `${MCP_BASE_PATH}/servers/${encodeURIComponent(ref.source)}/${encodeURIComponent(ref.id)}`;
}

function conversationPath(conversationId: string): string {
  return `${MCP_BASE_PATH}/conversations/${encodeURIComponent(conversationId)}`;
}

function workspacePath(workspaceId: string): string {
  return `${MCP_BASE_PATH}/workspaces/${encodeURIComponent(workspaceId)}`;
}

function query(values: Record<string, string | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) {
    if (value?.trim()) params.set(key, value.trim());
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
}

function requireNormalized<T>(value: T | null, name: string): T {
  if (value !== null) return value;
  throw new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    `Server returned an invalid MCP ${name} response.`,
  );
}

function normalizeOAuthStart(value: unknown): McpOAuthStartResult {
  if (!isRecord(value)) return invalidOAuthStart();
  const authorizationUrl =
    typeof value.authorizationUrl === "string"
      ? value.authorizationUrl.trim()
      : "";
  const expiresAt =
    typeof value.expiresAt === "string" &&
    Number.isFinite(Date.parse(value.expiresAt))
      ? value.expiresAt
      : "";
  if (!authorizationUrl || !expiresAt) return invalidOAuthStart();
  return { authorizationUrl, expiresAt };
}

function invalidOAuthStart(): never {
  throw new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    "Server returned an invalid MCP OAuth response.",
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
