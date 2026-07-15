import {
  ApiClientError,
  createNeoChatApiClient,
  joinUrl,
  type ApiClientConfig,
  type AppendUserMessageInput,
  type ChatMessageDTO,
  type ConversationDTO,
  type CreateConversationInput,
  type DeleteMessageInput,
  type DuplicateConversationInput,
  type GenerateConversationTitleInput,
  type ModelRef,
  type UpdateConversationInput,
  type UpdateMessageInput,
  type NeoChatApiClient,
  type ServerAttachmentDTO,
} from "./client";
import { normalizeSessionTitle } from "../../lib/chat/entities";
import { SERVER_DEFAULT_PROVIDER_ID } from "../../lib/defaultConfig/shared";

const SERVER_DEFAULT_BACKEND_PROVIDER_ID = "openai_compatible";

export interface ChatCrudSessionConfig {
  useSearch?: boolean;
  useReasoning?: boolean;
  activePlugins?: string[];
  activeSkills?: string[];
}

export interface ChatCrudSession {
  id: string;
  title: string;
  messageCount: number;
  updatedAt: number;
  model: string;
  pinned: boolean;
  systemInstruction?: string;
  config?: ChatCrudSessionConfig;
}

export interface ChatCrudAttachment {
  id: string;
  source: "server";
  fileId: string;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: string;
  url: string;
}

export interface ChatCrudMessage {
  id: string;
  role: "user" | "model";
  content: string;
  timestamp: number;
  attachments?: ChatCrudAttachment[];
  model?: string;
  outputBlocks?: unknown[];
  parentMessageId?: string;
  treeParentMessageId?: string | null;
}

export interface ChatCrudServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export interface ChatCrudService {
  mode: NeoChatApiClient["mode"];
  serverEnabled: boolean;
  createConversation(input: CreateConversationInput): Promise<ChatCrudSession>;
  listConversations(): Promise<ChatCrudSession[]>;
  updateConversation(input: UpdateConversationInput): Promise<ChatCrudSession>;
  deleteConversation(conversationId: string): Promise<void>;
  duplicateConversation(
    input: DuplicateConversationInput,
  ): Promise<ChatCrudSession>;
  generateConversationTitle(
    input: GenerateConversationTitleInput,
  ): Promise<string>;
  updateMessage(input: UpdateMessageInput): Promise<ChatCrudMessage>;
  deleteMessage(input: DeleteMessageInput): Promise<void>;
  appendUserMessage(input: AppendUserMessageInput): Promise<ChatCrudMessage>;
  listMessages(conversationId: string): Promise<ChatCrudMessage[]>;
}

export function createChatCrudService(
  options: ChatCrudServiceOptions = {},
): ChatCrudService {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const baseUrl = client.config.baseUrl;
  const serverEnabled =
    client.mode === "server" && client.capabilities.chatCrud === true;

  function requireServerCrud(): void {
    if (!serverEnabled) {
      throw new ApiClientError(
        "SERVER_CHAT_CRUD_DISABLED",
        "Server chat CRUD is not enabled for the current API mode.",
        { recoverable: true },
      );
    }
  }

  return {
    mode: client.mode,
    serverEnabled,

    async createConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.createConversation(input),
      );
    },

    async listConversations() {
      requireServerCrud();
      const conversations = await client.chat.listConversations();
      return conversations.map(mapConversationDtoToSession);
    },

    async updateConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.updateConversation(input),
      );
    },

    async deleteConversation(conversationId) {
      requireServerCrud();
      await client.chat.deleteConversation(conversationId);
    },

    async duplicateConversation(input) {
      requireServerCrud();
      return mapConversationDtoToSession(
        await client.chat.duplicateConversation(input),
      );
    },

    async generateConversationTitle(input) {
      requireServerCrud();
      const response = await client.chat.generateConversationTitle(input);
      return normalizeSessionTitle(response.title);
    },

    async updateMessage(input) {
      requireServerCrud();
      return mapChatMessageDtoToMessage(
        await client.chat.updateMessage(input),
        {
          baseUrl,
        },
      );
    },

    async deleteMessage(input) {
      requireServerCrud();
      await client.chat.deleteMessage(input);
    },

    async appendUserMessage(input) {
      requireServerCrud();
      return mapChatMessageDtoToMessage(
        await client.chat.appendUserMessage(input),
        { baseUrl },
      );
    },

    async listMessages(conversationId) {
      requireServerCrud();
      const messages = await client.chat.listMessages(conversationId);
      return messages.map((message) =>
        mapChatMessageDtoToMessage(message, { baseUrl }),
      );
    },
  };
}

export function mapConversationDtoToSession(
  conversation: ConversationDTO,
): ChatCrudSession {
  return {
    id: conversation.id,
    title: conversation.title.trim() || "New Chat",
    messageCount: Math.max(
      0,
      Math.floor(Number(conversation.messageCount) || 0),
    ),
    updatedAt: parseServerTimestamp(
      conversation.updatedAt,
      "conversation.updatedAt",
    ),
    model: modelRefToModelString(conversation.modelRef),
    pinned: conversation.pinned === true || conversation.config.pinned === true,
    systemInstruction:
      typeof conversation.systemInstruction === "string"
        ? conversation.systemInstruction
        : undefined,
    config: normalizeConversationConfig(conversation.config),
  };
}

export function mapChatMessageDtoToMessage(
  message: ChatMessageDTO,
  options: { baseUrl?: string } = {},
): ChatCrudMessage {
  const timestamp = parseServerTimestamp(
    message.createdAt,
    "message.createdAt",
  );
  const role = mapServerRoleToLegacyRole(message.role);
  const model = modelRefToModelString(message.modelRef);

  return {
    id: message.id,
    role,
    content: message.content,
    timestamp,
    ...(role === "model" && model ? { model } : {}),
    ...(message.attachments.length > 0
      ? {
          attachments: message.attachments.map((attachment) =>
            mapServerAttachmentToAttachment(attachment, options),
          ),
        }
      : {}),
    ...(message.outputBlocks.length > 0
      ? { outputBlocks: message.outputBlocks }
      : {}),
    ...(message.parentMessageId
      ? { parentMessageId: message.parentMessageId }
      : {}),
    ...treeParentFromMetadata(message.metadata),
  };
}

function treeParentFromMetadata(
  metadata: Record<string, unknown>,
): Pick<ChatCrudMessage, "treeParentMessageId"> {
  if (!Object.prototype.hasOwnProperty.call(metadata, "treeParentMessageId")) {
    return {};
  }
  const value = metadata.treeParentMessageId;
  if (typeof value === "string" && value.trim()) {
    return { treeParentMessageId: value.trim() };
  }
  if (value === null) {
    return { treeParentMessageId: null };
  }
  return {};
}

export function modelRefToModelString(modelRef?: ModelRef): string {
  const providerId = modelRef?.providerId?.trim() ?? "";
  const modelId = modelRef?.modelId?.trim() ?? "";
  if (providerId && modelId) return `${providerId}:${modelId}`;
  return modelId || providerId;
}

export function modelStringToModelRef(model: string): ModelRef | undefined {
  const trimmed = model.trim();
  if (!trimmed) return undefined;

  const parsed = parseLegacyModelString(trimmed);
  const providerId =
    parsed.providerId === SERVER_DEFAULT_PROVIDER_ID
      ? SERVER_DEFAULT_BACKEND_PROVIDER_ID
      : (parsed.providerId ?? "");

  return {
    providerId,
    modelId: parsed.modelName,
  };
}

export function parseServerTimestamp(value: string, fieldName: string): number {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      `Server returned invalid ${fieldName}.`,
    );
  }
  return timestamp;
}

function normalizeConversationConfig(
  config: Record<string, unknown>,
): ChatCrudSessionConfig | undefined {
  const normalized: ChatCrudSessionConfig = {};

  if (typeof config.useSearch === "boolean") {
    normalized.useSearch = config.useSearch;
  }
  if (typeof config.useReasoning === "boolean") {
    normalized.useReasoning = config.useReasoning;
  }
  if (Array.isArray(config.activePlugins)) {
    const activePlugins = config.activePlugins.filter(
      (value): value is string => typeof value === "string" && value !== "",
    );
    if (activePlugins.length > 0) normalized.activePlugins = activePlugins;
  }
  if (Array.isArray(config.activeSkills)) {
    const activeSkills = config.activeSkills.filter(
      (value): value is string => typeof value === "string" && value !== "",
    );
    if (activeSkills.length > 0) normalized.activeSkills = activeSkills;
  }

  return Object.keys(normalized).length > 0 ? normalized : undefined;
}

function parseLegacyModelString(model: string): {
  providerId?: string;
  modelName: string;
} {
  const separatorIndex = model.indexOf(":");
  if (separatorIndex > 0) {
    const providerId = model.slice(0, separatorIndex);
    const modelName = model.slice(separatorIndex + 1);
    if (modelName) {
      return { providerId, modelName };
    }
  }

  return { modelName: model };
}

function mapServerRoleToLegacyRole(
  role: ChatMessageDTO["role"],
): ChatCrudMessage["role"] {
  if (role === "user") return "user";
  if (role === "assistant") return "model";

  throw new ApiClientError(
    "UNSUPPORTED_MESSAGE_ROLE",
    `Server message role "${role}" cannot be rendered by the legacy chat UI.`,
  );
}

function mapServerAttachmentToAttachment(
  attachment: ServerAttachmentDTO,
  options: { baseUrl?: string },
): ChatCrudAttachment {
  return {
    id: attachment.id || attachment.fileId,
    source: "server",
    fileId: attachment.fileId,
    fileName: attachment.fileName || "download",
    mimeType: attachment.mimeType || "application/octet-stream",
    size: attachment.size,
    sha256: attachment.sha256,
    purpose: attachment.purpose || "input",
    url: joinUrl(
      options.baseUrl ?? "",
      `/v1/files/${encodeURIComponent(attachment.fileId)}/content`,
    ),
  };
}
