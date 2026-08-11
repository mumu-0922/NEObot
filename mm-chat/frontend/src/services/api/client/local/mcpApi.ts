import { unsupportedFeature } from "../errors";
import type { McpApi } from "../types";

const unsupported = () => unsupportedFeature("MCP Tools in local API mode");

export function createLocalMcpApiShell(): McpApi {
  return {
    async listServers() {
      throw unsupported();
    },
    async createPrivateServer() {
      throw unsupported();
    },
    async deletePrivateServer() {
      throw unsupported();
    },
    async validatePrivateServer() {
      throw unsupported();
    },
    async setCredential() {
      throw unsupported();
    },
    async deleteCredential() {
      throw unsupported();
    },
    async getConversationSelection() {
      throw unsupported();
    },
    async replaceConversationSelection() {
      throw unsupported();
    },
    async getWorkspaceSelection() {
      throw unsupported();
    },
    async replaceWorkspaceSelection() {
      throw unsupported();
    },
    async startOAuth() {
      throw unsupported();
    },
    async revokeOAuth() {
      throw unsupported();
    },
    async listCalls() {
      throw unsupported();
    },
  };
}
