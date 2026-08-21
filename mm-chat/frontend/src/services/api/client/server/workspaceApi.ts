import { ApiClientError } from "../errors";
import type {
  HostDirectoryBrowseDTO,
  HostWorkspaceStatusDTO,
  NativeDirectoryPickDTO,
  WorkspaceApi,
  WorkspaceDTO,
  WorkspaceFileDTO,
  WorkspaceSettingsDTO,
} from "../types";
import type { HttpClient } from "./httpClient";

const basePath = "/v1/workspaces";

export function createServerWorkspaceApiShell(
  httpClient: HttpClient,
): WorkspaceApi {
  return {
    async list(options = {}) {
      const response = await httpClient.requestJson<unknown>(basePath, {
        signal: options.signal,
      });
      const envelope = asRecord(response, "Workspace list");
      if (!Array.isArray(envelope.workspaces)) invalid("Workspace list");
      return envelope.workspaces.map(normalizeWorkspaceDTO);
    },
    async get(workspaceId, options = {}) {
      return normalizeWorkspaceEnvelope(
        await httpClient.requestJson<unknown>(workspacePath(workspaceId), {
          signal: options.signal,
        }),
      );
    },
    async importLegacy(input) {
      return normalizeWorkspaceEnvelope(
        await httpClient.requestJson<unknown>(
          workspacePath(input.workspaceId),
          {
            method: "PUT",
            body: workspaceSettingsBody(input.settings),
            signal: input.signal,
          },
        ),
      );
    },
    async update(input) {
      return normalizeWorkspaceEnvelope(
        await httpClient.requestJson<unknown>(
          workspacePath(input.workspaceId),
          {
            method: "PATCH",
            body: {
              ...workspaceSettingsBody(input.settings),
              expectedRevision: input.expectedRevision,
            },
            signal: input.signal,
          },
        ),
      );
    },
    async delete(input) {
      await httpClient.requestJson<void>(workspacePath(input.workspaceId), {
        method: "DELETE",
        body: { expectedRevision: input.expectedRevision },
        signal: input.signal,
      });
    },
    async bind(input) {
      return normalizeWorkspaceEnvelope(
        await httpClient.requestJson<unknown>(
          `${workspacePath(input.workspaceId)}/bind`,
          {
            method: "POST",
            body: {
              expectedRevision: input.expectedRevision,
              path: input.path,
            },
            signal: input.signal,
          },
        ),
      );
    },
    async setConversation(input) {
      await httpClient.requestJson<void>(
        `${workspacePath(input.workspaceId)}/conversations/${encodeURIComponent(input.conversationId)}`,
        { method: "PUT", signal: input.signal },
      );
    },
    async clearConversation(input) {
      await httpClient.requestJson<void>(
        `${workspacePath(input.workspaceId)}/conversations/${encodeURIComponent(input.conversationId)}`,
        { method: "DELETE", signal: input.signal },
      );
    },
    async getHostStatus(options = {}) {
      const value = asRecord(
        await httpClient.requestJson<unknown>(`${basePath}/host-status`, {
          signal: options.signal,
        }),
        "Host Workspace status",
      );
      const status = oneOf(value.status, [
        "disabled",
        "unavailable",
        "ready",
      ] as const);
      const features = asRecord(value.features, "Host Workspace features");
      const permissionModes = Array.isArray(features.permissionModes)
        ? features.permissionModes.map((mode) =>
            oneOf(mode, [
              "read-only",
              "workspace-write",
              "danger-full-access",
            ] as const),
          )
        : invalid("Host Workspace permission modes");
      return {
        enabled: booleanValue(value.enabled, "Host Workspace enabled"),
        status,
        ...optionalString("runnerId", value.runnerId),
        ...optionalString("platform", value.platform),
        ...optionalString("architecture", value.architecture),
        features: {
          workspaceResolve: booleanValue(
            features.workspaceResolve,
            "Workspace resolve capability",
          ),
          directoryBrowse: booleanValue(
            features.directoryBrowse,
            "Directory browse capability",
          ),
          nativeDirectoryPicker: booleanValue(
            features.nativeDirectoryPicker,
            "Native directory picker capability",
          ),
          windowsPathInterop: booleanValue(
            features.windowsPathInterop,
            "Windows path capability",
          ),
          execution: booleanValue(features.execution, "Execution capability"),
          permissionModes,
        },
      } satisfies HostWorkspaceStatusDTO;
    },
    async browseDirectories(input) {
      return normalizeDirectoryBrowse(
        await httpClient.requestJson<unknown>(
          `${basePath}/directories/browse`,
          {
            method: "POST",
            body: { path: input.path ?? "" },
            signal: input.signal,
          },
        ),
      );
    },
    async pickNativeDirectory(options = {}) {
      const value = asRecord(
        await httpClient.requestJson<unknown>(
          `${basePath}/directories/pick-native`,
          { method: "POST", body: {}, signal: options.signal },
        ),
        "Native directory selection",
      );
      const cancelled = booleanValue(value.cancelled, "Picker cancellation");
      if (cancelled) return { cancelled: true };
      return {
        cancelled: false,
        path: stringValue(value.path, "Selected directory path"),
        displayPath: stringValue(
          value.displayPath,
          "Selected directory display path",
        ),
        pathKind: oneOf(value.pathKind, ["wsl", "windows-mounted"] as const),
      } satisfies NativeDirectoryPickDTO;
    },
  };
}

function normalizeWorkspaceEnvelope(value: unknown): WorkspaceDTO {
  return normalizeWorkspaceDTO(asRecord(value, "Workspace response").workspace);
}

function normalizeWorkspaceDTO(value: unknown): WorkspaceDTO {
  const item = asRecord(value, "Workspace");
  const files = Array.isArray(item.files)
    ? item.files.map(normalizeWorkspaceFile)
    : invalid("Workspace files");
  const revision = Number(item.revision);
  if (!Number.isSafeInteger(revision) || revision < 1)
    invalid("Workspace revision");
  return {
    id: stringValue(item.id, "Workspace id"),
    name: stringValue(item.name, "Workspace name"),
    systemPrompt: optionalStringValue(item.systemPrompt),
    files,
    color: optionalStringValue(item.color),
    ...optionalBoolean("enableSearch", item.enableSearch),
    ...optionalBoolean("enableReasoning", item.enableReasoning),
    revision,
    bindingStatus: oneOf(item.bindingStatus, ["unbound", "bound"] as const),
    ...optionalString("runnerId", item.runnerId),
    ...optionalString("canonicalPath", item.canonicalPath),
    ...optionalString("displayPath", item.displayPath),
    ...(item.pathKind === undefined
      ? {}
      : {
          pathKind: oneOf(item.pathKind, ["wsl", "windows-mounted"] as const),
        }),
    ...optionalString("directoryFingerprint", item.directoryFingerprint),
    ...optionalString("boundAt", item.boundAt),
    ...optionalString("legacyImportedAt", item.legacyImportedAt),
    createdAt: timestampValue(item.createdAt, "Workspace createdAt"),
    updatedAt: timestampValue(item.updatedAt, "Workspace updatedAt"),
  };
}

function normalizeWorkspaceFile(value: unknown): WorkspaceFileDTO {
  const file = asRecord(value, "Workspace file");
  return {
    id: stringValue(file.id, "Workspace file id"),
    mimeType: stringValue(file.mimeType, "Workspace file mimeType"),
    fileName: stringValue(file.fileName, "Workspace file name"),
    ...optionalString("data", file.data),
    ...optionalString("url", file.url),
    ...optionalString("source", file.source),
    ...optionalString("fileId", file.fileId),
    ...optionalNumber("size", file.size),
    ...optionalString("sha256", file.sha256),
    ...optionalString("purpose", file.purpose),
  };
}

function normalizeDirectoryBrowse(value: unknown): HostDirectoryBrowseDTO {
  const listing = asRecord(value, "Directory listing");
  if (!Array.isArray(listing.entries)) invalid("Directory entries");
  return {
    path: stringValue(listing.path, "Directory path"),
    displayPath: stringValue(listing.displayPath, "Directory display path"),
    pathKind: oneOf(listing.pathKind, ["wsl", "windows-mounted"] as const),
    ...optionalString("parentPath", listing.parentPath),
    entries: listing.entries.map((entryValue) => {
      const entry = asRecord(entryValue, "Directory entry");
      return {
        name: stringValue(entry.name, "Directory entry name"),
        path: stringValue(entry.path, "Directory entry path"),
        displayPath: stringValue(
          entry.displayPath,
          "Directory entry display path",
        ),
        pathKind: oneOf(entry.pathKind, ["wsl", "windows-mounted"] as const),
      };
    }),
  };
}

export function workspaceSettingsBody(
  settings: WorkspaceSettingsDTO,
): WorkspaceSettingsDTO {
  return {
    name: settings.name,
    systemPrompt: settings.systemPrompt,
    files: settings.files,
    color: settings.color,
    ...(settings.enableSearch === undefined
      ? {}
      : { enableSearch: settings.enableSearch }),
    ...(settings.enableReasoning === undefined
      ? {}
      : { enableReasoning: settings.enableReasoning }),
  };
}

function workspacePath(workspaceId: string): string {
  return `${basePath}/${encodeURIComponent(workspaceId)}`;
}

function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    invalid(label);
  return value as Record<string, unknown>;
}

function stringValue(value: unknown, label: string): string {
  if (typeof value !== "string" || !value) invalid(label);
  return value;
}

function optionalStringValue(value: unknown): string {
  if (value === undefined || value === null) return "";
  if (typeof value !== "string") invalid("Optional string");
  return value;
}

function booleanValue(value: unknown, label: string): boolean {
  if (typeof value !== "boolean") invalid(label);
  return value;
}

function timestampValue(value: unknown, label: string): string {
  const result = stringValue(value, label);
  if (!Number.isFinite(Date.parse(result))) invalid(label);
  return result;
}

function optionalString<K extends string>(
  key: K,
  value: unknown,
): Partial<Record<K, string>> {
  if (value === undefined || value === null || value === "") return {};
  if (typeof value !== "string") invalid(key);
  return { [key]: value } as Record<K, string>;
}

function optionalBoolean<K extends string>(
  key: K,
  value: unknown,
): Partial<Record<K, boolean>> {
  if (value === undefined || value === null) return {};
  if (typeof value !== "boolean") invalid(key);
  return { [key]: value } as Record<K, boolean>;
}

function optionalNumber<K extends string>(
  key: K,
  value: unknown,
): Partial<Record<K, number>> {
  if (value === undefined || value === null) return {};
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0)
    invalid(key);
  return { [key]: value } as Record<K, number>;
}

function oneOf<const T extends readonly string[]>(
  value: unknown,
  values: T,
): T[number] {
  if (typeof value !== "string" || !values.includes(value))
    invalid("Enum value");
  return value as T[number];
}

function invalid(label: string): never {
  throw new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    `Server returned an invalid ${label}.`,
  );
}
