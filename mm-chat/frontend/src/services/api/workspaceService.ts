import type { Attachment, Workspace } from "@/types";
import { normalizeWorkspace } from "@/lib/chat/entities";
import {
  ApiClientError,
  createNeoChatApiClient,
  type ApiClientConfig,
  type HostDirectoryBrowseDTO,
  type HostWorkspaceStatusDTO,
  type NativeDirectoryPickDTO,
  type NeoChatApiClient,
  type WorkspaceDTO,
  type WorkspaceSettingsDTO,
} from "./client";

export interface WorkspaceServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export function createWorkspaceService(options: WorkspaceServiceOptions = {}) {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const workspaceApi = client.workspaces;
  const serverEnabled =
    client.mode === "server" &&
    client.capabilities.workspaces === true &&
    workspaceApi !== undefined;

  function requireServer(): void {
    if (!serverEnabled) {
      throw new ApiClientError(
        "SERVER_WORKSPACES_DISABLED",
        "Server Workspaces are not enabled for the current API mode.",
        { recoverable: true },
      );
    }
  }

  function api() {
    requireServer();
    if (!workspaceApi) {
      throw new ApiClientError(
        "SERVER_WORKSPACES_DISABLED",
        "Server Workspaces are unavailable.",
      );
    }
    return workspaceApi;
  }

  return {
    serverEnabled,
    async list(signal?: AbortSignal): Promise<Workspace[]> {
      requireServer();
      return (await api().list({ signal })).map(toWorkspace);
    },
    async importLegacy(
      workspace: Workspace,
      signal?: AbortSignal,
    ): Promise<Workspace> {
      requireServer();
      return toWorkspace(
        await api().importLegacy({
          workspaceId: workspace.id,
          settings: toSettings(workspace),
          signal,
        }),
      );
    },
    async update(
      workspace: Workspace,
      signal?: AbortSignal,
    ): Promise<Workspace> {
      requireServer();
      if (!workspace.revision) {
        throw new ApiClientError(
          "WORKSPACE_REVISION_MISSING",
          "Workspace revision is missing.",
        );
      }
      return toWorkspace(
        await api().update({
          workspaceId: workspace.id,
          expectedRevision: workspace.revision,
          settings: toSettings(workspace),
          signal,
        }),
      );
    },
    async delete(workspace: Workspace, signal?: AbortSignal): Promise<void> {
      requireServer();
      if (!workspace.revision) {
        throw new ApiClientError(
          "WORKSPACE_REVISION_MISSING",
          "Workspace revision is missing.",
        );
      }
      await api().delete({
        workspaceId: workspace.id,
        expectedRevision: workspace.revision,
        signal,
      });
    },
    async bind(
      workspace: Workspace,
      path: string,
      signal?: AbortSignal,
    ): Promise<Workspace> {
      requireServer();
      if (!workspace.revision) {
        throw new ApiClientError(
          "WORKSPACE_REVISION_MISSING",
          "Workspace revision is missing.",
        );
      }
      return toWorkspace(
        await api().bind({
          workspaceId: workspace.id,
          expectedRevision: workspace.revision,
          path,
          signal,
        }),
      );
    },
    async setConversation(
      workspaceId: string,
      conversationId: string,
      signal?: AbortSignal,
    ): Promise<void> {
      requireServer();
      await api().setConversation({
        workspaceId,
        conversationId,
        signal,
      });
    },
    async clearConversation(
      workspaceId: string,
      conversationId: string,
      signal?: AbortSignal,
    ): Promise<void> {
      requireServer();
      await api().clearConversation({
        workspaceId,
        conversationId,
        signal,
      });
    },
    async getHostStatus(signal?: AbortSignal): Promise<HostWorkspaceStatusDTO> {
      requireServer();
      return api().getHostStatus({ signal });
    },
    async browseDirectories(
      path?: string,
      signal?: AbortSignal,
    ): Promise<HostDirectoryBrowseDTO> {
      requireServer();
      return api().browseDirectories({ path, signal });
    },
    async pickNativeDirectory(
      signal?: AbortSignal,
    ): Promise<NativeDirectoryPickDTO> {
      requireServer();
      return api().pickNativeDirectory({ signal });
    },
  };
}

function toSettings(workspace: Workspace): WorkspaceSettingsDTO {
  return {
    name: workspace.name,
    systemPrompt: workspace.systemPrompt ?? "",
    files: workspace.files.map((file) => ({ ...file })),
    color: workspace.color ?? "blue",
    enableSearch: workspace.enableSearch ?? false,
    enableReasoning: workspace.enableReasoning ?? false,
  };
}

export function toWorkspace(dto: WorkspaceDTO): Workspace {
  return normalizeWorkspace({
    id: dto.id,
    name: dto.name,
    systemPrompt: dto.systemPrompt || undefined,
    files: dto.files.map((file) => ({ ...file }) as Attachment),
    color: dto.color || "blue",
    enableSearch: dto.enableSearch ?? false,
    enableReasoning: dto.enableReasoning ?? false,
    createdAt: Date.parse(dto.createdAt),
    updatedAt: Date.parse(dto.updatedAt),
    revision: dto.revision,
    bindingStatus: dto.bindingStatus,
    runnerId: dto.runnerId,
    canonicalPath: dto.canonicalPath,
    displayPath: dto.displayPath,
    pathKind: dto.pathKind,
    directoryFingerprint: dto.directoryFingerprint,
    boundAt: dto.boundAt ? Date.parse(dto.boundAt) : undefined,
  });
}
