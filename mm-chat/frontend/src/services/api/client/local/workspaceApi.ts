import { unsupportedFeature } from "../errors";
import type { WorkspaceApi } from "../types";

export function createLocalWorkspaceApiShell(): WorkspaceApi {
  return {
    async list() {
      throw unsupportedFeature("server-owned Workspaces");
    },
    async get() {
      throw unsupportedFeature("server-owned Workspaces");
    },
    async importLegacy() {
      throw unsupportedFeature("server-owned Workspaces");
    },
    async update() {
      throw unsupportedFeature("server-owned Workspaces");
    },
    async delete() {
      throw unsupportedFeature("server-owned Workspaces");
    },
    async bind() {
      throw unsupportedFeature("Host Workspace binding");
    },
    async setConversation() {
      throw unsupportedFeature("server-owned Workspace grouping");
    },
    async clearConversation() {
      throw unsupportedFeature("server-owned Workspace grouping");
    },
    async getHostStatus() {
      throw unsupportedFeature("Host Workspace status");
    },
    async browseDirectories() {
      throw unsupportedFeature("Host directory browsing");
    },
    async pickNativeDirectory() {
      throw unsupportedFeature("Host native directory picker");
    },
    async previewFile() {
      throw unsupportedFeature("Host Workspace file preview");
    },
    async readFile() {
      throw unsupportedFeature("Host Workspace file reading");
    },
  };
}
